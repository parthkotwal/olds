// Package mlclient provides an HTTP client for the Olds ML service.
//
// This package's structure deliberately mirrors internal/newsapi/client.go:
// unexported wire types, a Client struct wrapping http.Client, a constructor,
// and a single public method that returns domain types. The pattern is the same
// because the problem is the same — talk to an external HTTP service and
// translate its wire format into something the application can use.
package mlclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/olds/backend/internal/article"
)

// Client is an HTTP client for the ML service's /analyze endpoint.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a ready-to-use ML service client.
// baseURL should be the full scheme+host, e.g. "http://ml-service:8001".
// No trailing slash.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			// 30s timeout: embedding generation can take up to ~200ms per article,
			// but we allow generous headroom for container startup lag in CI.
			// The per-request timeout is the safety net; the WaitGroup in ingest.go
			// ensures we never block the response indefinitely.
			Timeout: 30 * time.Second,
		},
	}
}

// ── Wire types ─────────────────────────────────────────────────────────────────
// These mirror the Pydantic schemas in ml-service/app/schemas.py exactly.
// If you change one side, change the other.

type analyzeRequest struct {
	ArticleID string `json:"article_id"`
	Text      string `json:"text"`
}

// analyzeResponse is the full response envelope from POST /analyze.
// We only use Entities and Embedding; ModelMeta is logged on errors.
type analyzeResponse struct {
	ArticleID string          `json:"article_id"`
	Entities  []article.Entity `json:"entities"`
	Embedding []float64       `json:"embedding"`
	ModelMeta struct {
		SpacyModel     string `json:"spacy_model"`
		EmbeddingModel string `json:"embedding_model"`
		EmbeddingDim   int    `json:"embedding_dim"`
	} `json:"model_meta"`
}

// ── Public methods ─────────────────────────────────────────────────────────────

// Analyze sends article text to the ML service and returns the extracted
// entities and embedding vector.
//
// Go error handling pattern: this function returns (result, error).
// The caller checks the error first; if non-nil, the result is meaningless.
// This is Go's replacement for try/except — errors are values, not exceptions.
func (c *Client) Analyze(articleID, text string) ([]article.Entity, []float64, error) {
	// json.Marshal serializes the struct to a JSON []byte.
	// bytes.NewReader wraps it in an io.Reader so http.NewRequest can stream it —
	// the same "avoid unnecessary buffering" principle as json.NewDecoder in newsapi.
	reqBody, err := json.Marshal(analyzeRequest{
		ArticleID: articleID,
		Text:      text,
	})
	if err != nil {
		// This should never fail for a simple struct with string fields,
		// but we handle it explicitly — no underscore-ignoring.
		return nil, nil, fmt.Errorf("mlclient: marshal request: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		c.baseURL+"/analyze",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("mlclient: create request: %w", err)
	}
	// Tell the server the body is JSON. Without this header, FastAPI returns 422.
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("mlclient: POST /analyze: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("mlclient: ML service returned HTTP %d for article %q", resp.StatusCode, articleID)
	}

	var result analyzeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("mlclient: decode response: %w", err)
	}

	return result.Entities, result.Embedding, nil
}
