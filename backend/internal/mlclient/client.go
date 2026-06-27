// Package mlclient provides an HTTP client for the Olds ML service.
//
// The ML service now handles NER only (spaCy). Embeddings are generated
// by the embedclient package via OpenAI's API.
package mlclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/olds/backend/internal/article"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type analyzeRequest struct {
	ArticleID string `json:"article_id"`
	Text      string `json:"text"`
}

type analyzeResponse struct {
	ArticleID string           `json:"article_id"`
	Entities  []article.Entity `json:"entities"`
	ModelMeta struct {
		SpacyModel string `json:"spacy_model"`
	} `json:"model_meta"`
}

// Analyze sends article text to the ML service and returns extracted entities.
func (c *Client) Analyze(articleID, text string) ([]article.Entity, error) {
	reqBody, err := json.Marshal(analyzeRequest{
		ArticleID: articleID,
		Text:      text,
	})
	if err != nil {
		return nil, fmt.Errorf("mlclient: marshal request: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		c.baseURL+"/analyze",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("mlclient: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mlclient: POST /analyze: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mlclient: ML service returned HTTP %d for article %q", resp.StatusCode, articleID)
	}

	var result analyzeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("mlclient: decode response: %w", err)
	}

	return result.Entities, nil
}
