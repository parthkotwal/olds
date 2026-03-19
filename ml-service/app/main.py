"""FastAPI application for the Olds ML service.

Exposes two endpoints:
  GET  /health   — liveness check; reports whether models are loaded
  POST /analyze  — accepts article text, returns entities + embedding
"""

import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException

from app.analyzer import Analyzer, EMBEDDING_DIM, EMBEDDING_MODEL_NAME, SPACY_MODEL_NAME
from app.schemas import AnalyzeRequest, AnalyzeResponse, ModelMeta

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Module-level reference to the shared Analyzer instance.
# Set during lifespan startup, read by route handlers.
# Python's GIL makes concurrent reads safe in a single-process deployment.
_analyzer: Analyzer | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """FastAPI lifespan context manager — runs startup/shutdown logic.

    This is the modern replacement for @app.on_event("startup"), which is
    deprecated in FastAPI 0.93+. Code before `yield` runs at startup;
    code after runs at shutdown.

    The server does not accept requests until this function yields, so
    model loading here guarantees models are ready before any /analyze
    call can arrive. This is why the healthcheck's `start_period` in
    docker-compose.yml exists — models take ~10s to load on first start.
    """
    global _analyzer
    logger.info("loading spaCy and sentence-transformer models...")
    _analyzer = Analyzer()
    logger.info("models loaded — service ready")
    yield
    # Shutdown: nothing to explicitly release for these models.
    # The yield boundary is preserved for future resource cleanup
    # (e.g. database connections, background tasks).
    _analyzer = None


app = FastAPI(
    title="Olds ML Service",
    description="Named entity extraction and text embedding for the Olds news reader.",
    version="0.1.0",
    lifespan=lifespan,
)


@app.get("/health")
def health() -> dict:
    """Liveness check. Returns 200 once models are loaded.

    The `models_loaded` field lets docker-compose's healthcheck distinguish
    between "container started but models still loading" and "fully ready".
    The Go backend's `depends_on: condition: service_healthy` waits for this.
    """
    return {"status": "ok", "models_loaded": _analyzer is not None}


@app.post("/analyze", response_model=AnalyzeResponse)
def analyze(request: AnalyzeRequest) -> AnalyzeResponse:
    """Extract named entities and generate an embedding for the given text.

    Why `def` and not `async def`?
    This function is CPU-bound (NER + embedding are synchronous operations
    on in-process models). FastAPI automatically runs plain `def` route
    handlers in a thread pool executor, keeping the async event loop free
    for I/O. If this were `async def`, the CPU work would block the event
    loop and degrade all concurrent requests. Use `def` for CPU-bound work,
    `async def` for I/O-bound work — this is the key FastAPI concurrency rule.
    """
    if _analyzer is None:
        # Guards against edge cases in testing or misconfigured deployments
        # where lifespan did not complete. Should never happen in normal operation.
        raise HTTPException(status_code=503, detail="models not loaded")

    entities, embedding = _analyzer.analyze(request.text)

    return AnalyzeResponse(
        article_id=request.article_id,
        entities=entities,
        embedding=embedding,
        model_meta=ModelMeta(
            spacy_model=SPACY_MODEL_NAME,
            embedding_model=EMBEDDING_MODEL_NAME,
            embedding_dim=EMBEDDING_DIM,
        ),
    )
