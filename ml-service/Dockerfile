# Dockerfile for the Olds ML service (Python + FastAPI).
#
# Unlike the Go Dockerfile, we do NOT use a multi-stage build here.
# Go produces a self-contained static binary you can copy to a scratch image.
# Python cannot be separated from its runtime, site-packages, and model weights —
# everything must live in the same image layer.
#
# The main strategy here is LAYER ORDER for caching:
#   1. Install system deps
#   2. Install Python deps (slow — PyTorch is ~700MB)
#   3. Download and cache ML models (slow — ~200MB total)
#   4. Copy source code (fast — changes often)
#
# Steps 1-3 only re-run when requirements.txt or the Dockerfile itself changes.
# Source code changes only rebuild step 4, which takes seconds.

FROM python:3.11-slim

WORKDIR /app

# ── System dependencies ───────────────────────────────────────────────────────
# libgomp1 (GNU OpenMP) is required by PyTorch's native C extensions.
# Without it you get: "libgomp.so.1: cannot open shared object file" at runtime.
# --no-install-recommends keeps the install small; we only need the one library.
# Cleaning apt lists at the end avoids bloating the image with package metadata.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 \
    curl \
    && rm -rf /var/lib/apt/lists/*

# ── Python dependencies ───────────────────────────────────────────────────────
# Copy requirements.txt before any source code.
# Docker caches this layer; if requirements.txt hasn't changed, pip is skipped
# on the next build — even if source files changed. Same principle as copying
# package.json before npm install in a Node Dockerfile.
#
# sentence-transformers pulls in torch automatically. On a non-GPU machine,
# pip installs the CPU-only wheel (~700MB). If you want to be explicit:
#   pip install torch --index-url https://download.pytorch.org/whl/cpu
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# ── Bake ML models into the image ─────────────────────────────────────────────
# Models are downloaded at build time so the first /analyze request does not
# have to wait for a ~200MB download. This makes the image larger but the
# service startup fast and predictable.
#
# spaCy model (~12MB): python -m spacy download installs it as a pip package
# into site-packages, where spacy.load("en_core_web_sm") will find it.
RUN pip install --no-cache-dir \
    https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.7.1/en_core_web_sm-3.7.1-py3-none-any.whl

# sentence-transformers model (~90MB): HuggingFace caches weights in the
# directory pointed to by TRANSFORMERS_CACHE. We pin it to /app/.cache/huggingface
# so the cache is under our working directory and its location is predictable.
# The RUN command triggers a download by instantiating the model once.
ENV TRANSFORMERS_CACHE=/app/.cache/huggingface
RUN python -c "\
from sentence_transformers import SentenceTransformer; \
SentenceTransformer('all-MiniLM-L6-v2'); \
print('all-MiniLM-L6-v2 cached successfully')"

# ── Application source ────────────────────────────────────────────────────────
# Copied last so that source code edits don't invalidate the expensive
# dependency and model layers above.
COPY app/ ./app/

EXPOSE 8001

# Single worker — intentional.
# sentence-transformers is not fork-safe for multi-process Uvicorn workers
# without explicit multiprocessing setup. Multiple workers × ~1GB/process
# would exhaust memory on a development machine. A single worker handles
# article analysis fine: NER + embedding takes 50-200ms, and ingestion
# is not a concurrent hot path.
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001"]
