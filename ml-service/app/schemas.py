"""Pydantic models defining the request/response contract for the ML service.

These schemas are the single source of truth for what the Go backend sends
and receives. Any change here must be mirrored in the Go structs in
backend/internal/mlclient/client.go.
"""

from pydantic import BaseModel


class AnalyzeRequest(BaseModel):
    """Payload sent by the Go backend to POST /analyze."""

    article_id: str
    text: str


class Entity(BaseModel):
    """A single named entity extracted from the article text."""

    text: str
    label: str
    start: int
    end: int


class ModelMeta(BaseModel):
    """Metadata about which model version produced this response."""

    spacy_model: str


class AnalyzeResponse(BaseModel):
    """Full response from POST /analyze."""

    article_id: str
    entities: list[Entity]
    model_meta: ModelMeta
