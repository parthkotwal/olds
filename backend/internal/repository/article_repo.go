// Package repository provides the database persistence layer for the Olds backend.
//
// The repository pattern separates the "how to store/load data" concern from
// the "what the data means" concern. The article, graph, and behavior packages
// are domain packages — they define types and in-memory operations. This package
// owns the SQL queries that persist and hydrate those types.
//
// Each repository is defined as an interface + a concrete pgx implementation.
// The interface enables tests to inject a fake implementation without a real
// database, exactly like how the ML client is injected as an optional *mlclient.Client.
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/olds/backend/internal/article"
)

// ArticleRepository defines the persistence operations for articles.
// The interface is the contract; pgxArticleRepo is the implementation.
type ArticleRepository interface {
	// UpsertBatch inserts or updates a slice of articles in a single
	// transaction. For each article, it writes to articles, article_entities,
	// and article_embeddings. Called after every ingestion batch.
	UpsertBatch(ctx context.Context, articles []article.Article) error

	// LoadAll returns every article with its entities and embedding vector.
	// Called once at startup to hydrate the in-memory stores and graph.
	LoadAll(ctx context.Context) ([]article.Article, error)
}

// pgxArticleRepo is the pgx-backed implementation of ArticleRepository.
// The pool field is unexported — callers use the interface, not the concrete type.
type pgxArticleRepo struct {
	pool *pgxpool.Pool
}

// NewArticleRepository returns an ArticleRepository backed by the given connection pool.
// The return type is the interface, so callers depend on the abstraction, not the concrete type.
func NewArticleRepository(pool *pgxpool.Pool) ArticleRepository {
	return &pgxArticleRepo{pool: pool}
}

// UpsertBatch writes a slice of articles to Postgres in a single transaction.
//
// For each article:
//  1. Upsert the articles row (ON CONFLICT DO UPDATE — safe to re-ingest)
//  2. Delete + reinsert article_entities — ML results may change on re-analysis
//  3. Upsert article_embeddings — replaces the vector if re-enriched
//
// In Go, a transaction is started with pool.Begin(ctx) and must be either
// committed (tx.Commit) or rolled back (tx.Rollback). The deferred Rollback
// is a no-op after Commit, so this pattern guarantees cleanup on any return
// path — including early returns from errors. Python analogy: "async with
// conn.transaction():" but explicit rather than a context manager.
func (r *pgxArticleRepo) UpsertBatch(ctx context.Context, articles []article.Article) error {
	if len(articles) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // no-op if Commit was called; rolls back on any error path

	for _, a := range articles {
		// ── 1. Upsert articles row ────────────────────────────────────────────
		// ON CONFLICT (id) DO UPDATE sets every column to the new value.
		// EXCLUDED refers to the row that was rejected due to conflict —
		// i.e., the new values we tried to insert.
		_, err := tx.Exec(ctx, `
			INSERT INTO articles (id, title, description, url, source, category, published_at, image_url, raw_text)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				title        = EXCLUDED.title,
				description  = EXCLUDED.description,
				url          = EXCLUDED.url,
				source       = EXCLUDED.source,
				category     = EXCLUDED.category,
				published_at = EXCLUDED.published_at,
				image_url    = EXCLUDED.image_url,
				raw_text     = EXCLUDED.raw_text
		`, a.ID, a.Title, a.Description, a.URL, a.Source, a.Category,
			a.PublishedAt, a.ImageURL, a.RawText)
		if err != nil {
			return fmt.Errorf("upsert article %s: %w", a.ID, err)
		}

		// ── 2. Replace entities ───────────────────────────────────────────────
		// Delete all existing entities for this article, then insert fresh ones.
		// This is simpler and safer than diffing the old and new entity lists.
		// ON DELETE CASCADE on the FK means we could also delete the article
		// row itself, but explicit delete on article_entities is clearer.
		if _, err := tx.Exec(ctx,
			`DELETE FROM article_entities WHERE article_id = $1`, a.ID,
		); err != nil {
			return fmt.Errorf("delete entities for article %s: %w", a.ID, err)
		}

		for _, ent := range a.Entities {
			if _, err := tx.Exec(ctx, `
				INSERT INTO article_entities (article_id, text, label, start_pos, end_pos)
				VALUES ($1, $2, $3, $4, $5)
			`, a.ID, ent.Text, ent.Label, ent.Start, ent.End); err != nil {
				return fmt.Errorf("insert entity for article %s: %w", a.ID, err)
			}
		}

		// ── 3. Upsert embedding ───────────────────────────────────────────────
		// Only persist if the article has a non-empty embedding.
		// Articles without an embedding (ML service unavailable) are stored
		// in the articles table but skipped here — graph edges won't be
		// computed for them, which matches the in-memory behavior.
		if len(a.Embedding) > 0 {
			vec := pgvector.NewVector(float64ToFloat32(a.Embedding))
			if _, err := tx.Exec(ctx, `
				INSERT INTO article_embeddings (article_id, embedding)
				VALUES ($1, $2)
				ON CONFLICT (article_id) DO UPDATE SET embedding = EXCLUDED.embedding
			`, a.ID, vec); err != nil {
				return fmt.Errorf("upsert embedding for article %s: %w", a.ID, err)
			}
		}
	}

	// Commit atomically publishes all the inserts/updates above.
	// If Commit returns an error, the deferred Rollback cleans up.
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// LoadAll returns every article from Postgres, with entities and embedding vectors
// populated. Called once at startup for graph hydration.
//
// Strategy: two queries instead of one large JOIN.
//  1. SELECT articles JOIN article_embeddings — one row per article
//  2. SELECT article_entities WHERE article_id = ANY($1) — all entities in one query
//
// The alternative (one big JOIN) produces duplicate article rows when an article
// has multiple entities, requiring deduplication in Go. Two queries + in-memory
// stitching is cleaner and produces the same result.
func (r *pgxArticleRepo) LoadAll(ctx context.Context) ([]article.Article, error) {
	// ── Query 1: articles + embeddings ───────────────────────────────────────
	rows, err := r.pool.Query(ctx, `
		SELECT
			a.id, a.title, a.description, a.url, a.source, a.category,
			a.published_at, a.image_url, a.raw_text,
			ae.embedding
		FROM articles a
		LEFT JOIN article_embeddings ae ON ae.article_id = a.id
		ORDER BY a.published_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query articles: %w", err)
	}
	defer rows.Close()

	// articles holds the loaded articles; ids collects IDs for the entity query.
	var articles []article.Article
	var ids []string

	for rows.Next() {
		var a article.Article
		var vec *pgvector.Vector // nullable — articles without an embedding have NULL

		if err := rows.Scan(
			&a.ID, &a.Title, &a.Description, &a.URL, &a.Source, &a.Category,
			&a.PublishedAt, &a.ImageURL, &a.RawText,
			&vec,
		); err != nil {
			return nil, fmt.Errorf("scan article row: %w", err)
		}

		// Convert []float32 → []float64. The ML service uses float64; pgvector
		// stores float32. We convert at the persistence boundary so the domain
		// type (Article.Embedding []float64) stays clean.
		if vec != nil {
			a.Embedding = float32ToFloat64(vec.Slice())
		}

		articles = append(articles, a)
		ids = append(ids, a.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article rows: %w", err)
	}

	if len(articles) == 0 {
		return articles, nil
	}

	// ── Query 2: all entities for all loaded articles ─────────────────────────
	// ANY($1) with a []string argument is pgx's way of writing "WHERE id IN (...)"
	// without constructing a dynamic SQL string. One query for all articles avoids
	// the N+1 problem (one query per article would be very slow with many articles).
	entityRows, err := r.pool.Query(ctx, `
		SELECT article_id, text, label, start_pos, end_pos
		FROM article_entities
		WHERE article_id = ANY($1)
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("query entities: %w", err)
	}
	defer entityRows.Close()

	// Build a map from article ID to its entities so we can stitch them in O(N).
	entityMap := make(map[string][]article.Entity)
	for entityRows.Next() {
		var articleID string
		var ent article.Entity
		if err := entityRows.Scan(&articleID, &ent.Text, &ent.Label, &ent.Start, &ent.End); err != nil {
			return nil, fmt.Errorf("scan entity row: %w", err)
		}
		entityMap[articleID] = append(entityMap[articleID], ent)
	}
	if err := entityRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entity rows: %w", err)
	}

	// Stitch entities into articles. We assign to articles[i].Entities rather
	// than modifying a ranging variable because ranging over a slice gives copies
	// in Go — modifying `a` inside a range loop doesn't affect the original slice.
	for i := range articles {
		articles[i].Entities = entityMap[articles[i].ID]
	}

	return articles, nil
}

// float64ToFloat32 converts a []float64 to []float32.
// pgvector stores float32; Article.Embedding is float64 (from the ML service).
// Precision loss at the 7th decimal place is acceptable for 384-dim embeddings.
func float64ToFloat32(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = float32(v)
	}
	return out
}

// float32ToFloat64 converts a []float32 to []float64.
func float32ToFloat64(in []float32) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}
