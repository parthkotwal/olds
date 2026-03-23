-- graph_snapshots records a full system metrics snapshot after every ingestion
-- run. This is the primary data source for Phase 14 stress-test analysis:
-- graph growth curves, decay distribution over time, cross-topic ratio trends.
--
-- One row per ingestion run. With a 30-minute interval and 3 days of testing,
-- expect ~144 rows — trivially small, no indexing needed beyond the PK.
CREATE TABLE IF NOT EXISTS graph_snapshots (
    id          BIGSERIAL    PRIMARY KEY,
    captured_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Graph topology at snapshot time
    node_count            INT              NOT NULL,
    unique_edges          INT              NOT NULL,
    density_pct           DOUBLE PRECISION NOT NULL,
    avg_edges_per_node    DOUBLE PRECISION NOT NULL,
    isolated_nodes        INT              NOT NULL,
    max_edges_per_node    INT              NOT NULL,
    -- cross_topic_ratio_pct: percentage of unique edges that bridge different
    -- categories. This is the core product metric — if it's high, the engine
    -- is surfacing non-obvious connections, not just clustering same-topic articles.
    cross_topic_ratio_pct DOUBLE PRECISION NOT NULL,

    -- Article decay distribution at snapshot time
    articles_fresh  INT NOT NULL,
    articles_recent INT NOT NULL,
    articles_aging  INT NOT NULL,
    articles_stale  INT NOT NULL,

    -- Ingestion run metadata
    ingest_run_count     INT NOT NULL,
    last_ingest_articles INT NOT NULL
);
