-- Reverse of 000001_init_schema.up.sql.
-- Drop in reverse dependency order (tables that reference others must go first).
DROP TABLE IF EXISTS behavior_events;
DROP TABLE IF EXISTS article_embeddings;
DROP TABLE IF EXISTS article_entities;
DROP TABLE IF EXISTS articles;
DROP EXTENSION IF EXISTS vector;
