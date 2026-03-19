"""Core ML logic: named entity recognition and text embedding.

This module owns all interaction with spaCy and sentence-transformers.
Nothing else in the service imports these libraries directly — all ML
work flows through the Analyzer class.

Design principle: models are loaded once at Analyzer instantiation and
reused for the lifetime of the process. Loading all-MiniLM-L6-v2 takes
~2-4 seconds and ~500MB of RAM. Loading per-request would make the
service completely unusable.
"""

import spacy
from sentence_transformers import SentenceTransformer

from app.schemas import Entity

# Model name constants defined here so main.py can include them in ModelMeta
# without importing the heavy libraries itself.
SPACY_MODEL_NAME = "en_core_web_sm"
EMBEDDING_MODEL_NAME = "all-MiniLM-L6-v2"
EMBEDDING_DIM = 384


class Analyzer:
    """Wraps spaCy NER and a sentence-transformer embedding model.

    Instantiate once at application startup (via FastAPI's lifespan hook).
    All public methods are safe to call concurrently from multiple threads —
    spaCy's nlp() pipeline and SentenceTransformer.encode() are both
    stateless with respect to their inputs.
    """

    def __init__(self) -> None:
        # spacy.load() reads the model from the site-packages directory
        # where it was installed during the Docker build step:
        #   RUN python -m spacy download en_core_web_sm
        # If the model is not installed, this raises OSError immediately,
        # which surfaces as a clean startup failure rather than a 500 at
        # request time.
        self._nlp = spacy.load(SPACY_MODEL_NAME)

        # SentenceTransformer reads the model weights from the cache baked
        # into the image at build time (TRANSFORMERS_CACHE env var in Dockerfile).
        # If the cache is missing (e.g. running outside Docker without pre-download),
        # it falls back to downloading from HuggingFace Hub — slow but functional.
        self._embedder = SentenceTransformer(EMBEDDING_MODEL_NAME)

    def extract_entities(self, text: str) -> list[Entity]:
        """Run spaCy NER on the text and return a list of Entity objects.

        spaCy's nlp() call tokenizes the text, runs the full pipeline
        (tagger → parser → NER), and returns a Doc object. doc.ents is
        a tuple of Span objects representing recognized entities.

        Empty or whitespace-only text is handled gracefully — spaCy returns
        an empty ents tuple, so this returns [].
        """
        doc = self._nlp(text)
        return [
            Entity(
                text=ent.text,
                label=ent.label_,  # note: label_ (with underscore) is the string label;
                # ent.label (without) is an integer hash — always use label_
                start=ent.start_char,
                end=ent.end_char,
            )
            for ent in doc.ents
        ]

    def embed(self, text: str) -> list[float]:
        """Generate a dense embedding vector for the text.

        SentenceTransformer.encode() returns a numpy ndarray of shape (384,)
        for a single input string. We call .tolist() to convert it to a plain
        Python list[float] that Pydantic can serialize to JSON.

        Note: encode() also accepts a list of strings for batch processing,
        returning shape (N, 384). We use single-string form here to keep the
        interface simple — batching is a future optimization.

        Empty text returns a zero vector of length EMBEDDING_DIM. The model
        technically handles empty strings, but the resulting vector is not
        meaningful — callers should avoid sending empty text.
        """
        # show_progress_bar=False suppresses the tqdm progress bar that
        # sentence-transformers prints by default, which would pollute logs.
        vector = self._embedder.encode(text, show_progress_bar=False)
        return vector.tolist()

    def analyze(self, text: str) -> tuple[list[Entity], list[float]]:
        """Run both NER and embedding on the text, returning both results.

        This is the primary entry point called by the /analyze route handler.
        Splitting into extract_entities + embed as separate methods keeps
        each concern testable in isolation.
        """
        entities = self.extract_entities(text)
        embedding = self.embed(text)
        return entities, embedding
