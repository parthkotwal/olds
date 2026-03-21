-- Phase 11: initial schema for Olds persistence layer.
--
-- Extension must come first — the vector type used in article_embeddings
-- is provided by pgvector. The IF NOT EXISTS guard makes this idempotent.
CREATE EXTENSION IF NOT EXISTS vector;

-- ── articles ──────────────────────────────────────────────────────────────────
-- One row per article. Scalar fields only — entities and embeddings live in
-- their own tables so that list queries never touch wide vector columns.
CREATE TABLE articles (
    id           TEXT        PRIMARY KEY,
    title        TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    url          TEXT        NOT NULL,
    source       TEXT        NOT NULL,
    category     TEXT        NOT NULL,
    published_at TIMESTAMPTZ NOT NULL,
    image_url    TEXT        NOT NULL DEFAULT '',
    raw_text     TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Supports GET /articles?category=X without a full table scan.
CREATE INDEX idx_articles_category     ON articles (category);
-- Supports default feed ordering (newest first).
CREATE INDEX idx_articles_published_at ON articles (published_at DESC);

-- ── article_entities ──────────────────────────────────────────────────────────
-- Named entities extracted by the ML service (spaCy). Many per article.
-- Kept separate from articles so Phase 15 (connection provenance labels) can
-- query "which entities do articles A and B share?" with a plain JOIN.
CREATE TABLE article_entities (
    id         BIGSERIAL   PRIMARY KEY,
    article_id TEXT        NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    text       TEXT        NOT NULL,   -- e.g. "South China Sea"
    label      TEXT        NOT NULL,   -- e.g. "GPE", "PERSON", "ORG"
    start_pos  INT         NOT NULL,   -- character offset in raw_text
    end_pos    INT         NOT NULL
);

CREATE INDEX idx_article_entities_article_id ON article_entities (article_id);

-- ── article_embeddings ────────────────────────────────────────────────────────
-- One 384-dimensional sentence-transformer vector per article.
-- Stored in a separate table so SELECT on articles never reads 3KB vectors.
-- Joined only during startup graph hydration.
--
-- vector(384): pgvector type. all-MiniLM-L6-v2 always produces exactly 384 dims.
CREATE TABLE article_embeddings (
    article_id TEXT        PRIMARY KEY REFERENCES articles(id) ON DELETE CASCADE,
    embedding  vector(384) NOT NULL
);

-- ── behavior_events ───────────────────────────────────────────────────────────
-- One row per raw behavioral event from POST /behavior.
-- Events are immutable facts — never updated, only inserted.
-- Aggregated at startup to rebuild the in-memory behavior.Store.
--
-- user_id is NULL for now (anonymous sessions in Phase 1). Phase 12 (OAuth)
-- will link events to user records. The column exists now so Phase 12 needs
-- no schema migration here — only an ALTER TABLE ... ADD FOREIGN KEY.
CREATE TABLE behavior_events (
    id          BIGSERIAL        PRIMARY KEY,
    article_id  TEXT             NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    event_type  TEXT             NOT NULL CHECK (event_type IN ('dwell', 'scroll_depth', 'reopen')),
    value       DOUBLE PRECISION NOT NULL,
    user_id     TEXT,
    occurred_at TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_behavior_events_article_id ON behavior_events (article_id);
