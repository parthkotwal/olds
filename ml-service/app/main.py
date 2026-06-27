"""FastAPI application for the Olds ML service.

Exposes two endpoints:
  GET  /health   — liveness check; reports whether the spaCy model is loaded
  POST /analyze  — accepts article text, returns extracted entities
"""

import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException

from app.analyzer import Analyzer, SPACY_MODEL_NAME
from app.schemas import AnalyzeRequest, AnalyzeResponse, ModelMeta

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

_analyzer: Analyzer | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Load spaCy model at startup."""
    global _analyzer
    logger.info("loading spaCy model...")
    _analyzer = Analyzer()
    logger.info("spaCy model loaded — service ready")
    yield
    _analyzer = None


app = FastAPI(
    title="Olds ML Service",
    description="Named entity extraction for the Olds news reader.",
    version="0.2.0",
    lifespan=lifespan,
)


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "models_loaded": _analyzer is not None}


@app.post("/analyze", response_model=AnalyzeResponse)
def analyze(request: AnalyzeRequest) -> AnalyzeResponse:
    """Extract named entities from the given text.

    Embeddings are handled by the Go backend via OpenAI's API.
    """
    if _analyzer is None:
        raise HTTPException(status_code=503, detail="model not loaded")

    entities = _analyzer.extract_entities(request.text)

    return AnalyzeResponse(
        article_id=request.article_id,
        entities=entities,
        model_meta=ModelMeta(
            spacy_model=SPACY_MODEL_NAME,
        ),
    )
