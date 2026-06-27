// Package embedclient provides an HTTP client for OpenAI's embeddings API.
//
// This replaces the sentence-transformers embedding that was previously
// handled by the ML service. Using OpenAI's text-embedding-3-small with
// dimensions=384 keeps the same vector size as the old all-MiniLM-L6-v2
// model, so no schema migration is needed.
package embedclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	openAIEmbeddingsURL = "https://api.openai.com/v1/embeddings"
	defaultModel        = "text-embedding-3-small"
	defaultDimensions   = 384
)

// Client calls the OpenAI embeddings API. Safe for concurrent use.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates an embedding client with the given OpenAI API key.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type embeddingRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Dimensions int    `json:"dimensions"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// Embed generates a 384-dimensional embedding for the given text using
// OpenAI's text-embedding-3-small model.
func (c *Client) Embed(ctx context.Context, text string) ([]float64, error) {
	reqBody, err := json.Marshal(embeddingRequest{
		Model:      defaultModel,
		Input:      text,
		Dimensions: defaultDimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("embedclient: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEmbeddingsURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("embedclient: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedclient: http call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embedclient: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedclient: OpenAI returned %d: %s", resp.StatusCode, string(body))
	}

	var result embeddingResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("embedclient: parse response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embedclient: no embedding returned")
	}

	return result.Data[0].Embedding, nil
}
