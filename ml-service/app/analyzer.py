"""Core ML logic: named entity recognition.

This module owns all interaction with spaCy. Nothing else in the service
imports spaCy directly — all NER work flows through the Analyzer class.

Embeddings are now handled by the Go backend via OpenAI's
text-embedding-3-small API, so sentence-transformers has been removed.
"""

import spacy

from app.schemas import Entity

SPACY_MODEL_NAME = "en_core_web_sm"

_SOURCE_BLOCKLIST: frozenset[str] = frozenset(
    {
        "guardian",
        "the guardian",
        "associated press",
        "ap",
        "reuters",
        "bloomberg",
        "bbc",
        "cnn",
        "new york times",
        "nyt",
        "washington post",
        "fox news",
        "the independent",
        "the telegraph",
        "sky news",
    }
)


class Analyzer:
    """Wraps spaCy NER for named entity extraction.

    Instantiate once at application startup (via FastAPI's lifespan hook).
    """

    def __init__(self) -> None:
        self._nlp = spacy.load(SPACY_MODEL_NAME)

    def extract_entities(self, text: str) -> list[Entity]:
        """Run spaCy NER on the text and return a list of Entity objects."""
        doc = self._nlp(text)
        entities = []
        for ent in doc.ents:
            if ent.text.lower() in _SOURCE_BLOCKLIST:
                continue
            entities.append(
                Entity(
                    text=ent.text,
                    label=ent.label_,
                    start=ent.start_char,
                    end=ent.end_char,
                )
            )
        return entities
