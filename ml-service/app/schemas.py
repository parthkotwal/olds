"""Pydantic models defining the request/response contract for the ML service.

These schemas are the single source of truth for what the Go backend sends
and receives. Any change here must be mirrored in the Go structs in Phase 4.

Pydantic v2 (used here) validates types at instantiation time and raises
clear errors on mismatches — similar to a TypeScript interface at runtime.
"""

from pydantic import BaseModel


class AnalyzeRequest(BaseModel):
    """Payload sent by the Go backend to POST /analyze."""

    article_id: str  # Echo'd back in the response so Go can correlate results
    text: str  # Raw article body text to analyze


class Entity(BaseModel):
    """A single named entity extracted from the article text.

    spaCy entity labels relevant to news articles:
      PERSON  — people, including fictional
      ORG     — companies, agencies, institutions
      GPE     — geopolitical entities: countries, cities, states
      LOC     — non-GPE locations: mountain ranges, bodies of water
      EVENT   — named events: hurricanes, wars, sports events
      NORP    — nationalities, religious/political groups
      DATE    — dates and time periods (lower signal for graph edges)
    """

    text: str  # Surface form: "South China Sea", "Elon Musk"
    label: str  # spaCy label: "GPE", "ORG", "PERSON", "LOC", "EVENT", etc.
    start: int  # Character offset in original text — useful for frontend highlighting
    end: int  # Character offset end


class ModelMeta(BaseModel):
    """Metadata about which model versions produced this response.

    Including this in every response lets the Go backend detect model version
    mismatches without reading environment variables — useful in Phase 5 when
    we start storing embeddings that must be compared by cosine similarity
    (comparing embeddings from different models is meaningless).
    """

    spacy_model: str
    embedding_model: str
    embedding_dim: int


class AnalyzeResponse(BaseModel):
    """Full response from POST /analyze."""

    article_id: str  # Same ID from the request
    entities: list[Entity]
    embedding: list[float]  # 384 floats for all-MiniLM-L6-v2
    model_meta: ModelMeta
