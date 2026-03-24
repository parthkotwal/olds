// Package llm provides the OpenAI client used to generate natural language
// explanations of cross-topic article connections.
//
// Scope: connection explanations only. Do not expand this package to
// summarization, search, or any other LLM feature (see CLAUDE.md guardrails).
package llm

// Go uses net/http + encoding/json from the standard library for HTTP calls.
// No SDK is needed — the OpenAI REST API is a simple POST request.
// This keeps the dependency footprint small and keeps the code transparent.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// gpt-5-nano uses the Responses API, not the older Chat Completions API.
	// The endpoints and JSON shapes are different — see Explain() below.
	openAIURL    = "https://api.openai.com/v1/responses"
	defaultModel = "gpt-5-nano"

	// maxDescriptionLen caps how many characters of an article's description
	// we include in the prompt. Longer context rarely improves a 2-sentence
	// explanation and increases token cost.
	maxDescriptionLen = 220
)

// Client calls the OpenAI chat completions API to generate connection
// explanations. It is created once at startup and shared across requests.
//
// The client is safe for concurrent use — it holds no mutable state.
// All request data is passed as function arguments.
type Client struct {
	apiKey     string
	httpClient *http.Client
	model      string
}

// NewClient returns an LLM client configured with the given API key.
// The HTTP client has a 20-second timeout — generous enough for OpenAI's
// typical response times but short enough to not block WebSocket responses.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		model: defaultModel,
	}
}

// Explain returns a 1-2 sentence natural language explanation of why two
// articles are connected. It is called once per article pair (cache misses
// only — the ExplanationCache in cache.go handles deduplication).
//
// Parameters:
//   - titleA, descA   — title and description of the source article
//   - categoryA       — category of the source article (e.g. "general")
//   - titleB, descB   — title and description of the connected article
//   - categoryB       — category of the connected article
//   - sharedEntities  — entity texts common to both articles (from graph.SharedEntities)
//   - weight          — edge weight from the graph (0.0–1.0)
//
// Returns an empty string on error so callers can degrade gracefully.
func (c *Client) Explain(
	titleA, descA, categoryA string,
	titleB, descB, categoryB string,
	sharedEntities []string,
	weight float64,
) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()

	userPrompt := buildPrompt(
		titleA, descA, categoryA,
		titleB, descB, categoryB,
		sharedEntities, weight,
	)

	// The Responses API (used by gpt-5-nano) uses "input" instead of "messages"
	// and "max_output_tokens" instead of "max_completion_tokens".
	// Each item in "input" has "role" + "content" just like Chat Completions,
	// but the envelope fields differ.
	reqBody := struct {
		Model string `json:"model"`
		Input []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"input"`
		MaxOutputTokens int `json:"max_output_tokens"`
		Reasoning       struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
	}{
		Model: c.model,
		Input: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{
				Role: "system",
				Content: "You are an editor for Olds, a cross-topic news connection engine. " +
					"Given two articles from different topic categories, write exactly 1-2 tight sentences (max 30 words) " +
					"explaining the specific non-obvious link between them. Focus on what connects them " +
					"across topics — not what makes them similar within a topic. Be concrete. " +
					"Do not start with 'Both' or 'These articles' or similar. Do not pad with filler phrases." +
					"Output only the final answer. Do not include reasoning or analysis.",
			},
			{Role: "user", Content: userPrompt},
		},
		MaxOutputTokens: 400,
		Reasoning: struct {
			Effort string `json:"effort"`
		}{
			Effort: "minimal",
		},
	}

	// json.Marshal encodes the struct to JSON bytes.
	// bytes.NewReader wraps it so it can be read as an io.Reader (what http.NewRequest expects).
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	// http.NewRequestWithContext attaches a context so the request is
	// cancelled if the 18s timeout fires. This prevents goroutine leaks.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: http call: %w", err)
	}
	defer resp.Body.Close()

	// io.ReadAll reads the entire response body into memory.
	// Fine here — OpenAI responses are small (a few hundred bytes).
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: OpenAI returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse the Responses API shape:
	//   output[0].content[0].text
	// Each output item has a "type" ("message") and a "content" array.
	// Each content item has a "type" ("output_text") and the actual "text".
	var parsed struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}

	if err := json.Unmarshal(respBody, &parsed); err != nil {
		log.Printf("llm: parse error — raw response: %s", string(respBody))
		return "", fmt.Errorf("llm: parse response: %w", err)
	}

	for _, item := range parsed.Output {
		if item.Type == "message" {
			for _, c := range item.Content {
				if c.Type == "output_text" {
					text := strings.TrimSpace(c.Text)
					if text != "" {
						return text, nil
					}
				}
			}
		}
	}

	// Log unexpected shapes so they're visible in docker logs.
	log.Printf("llm: could not extract text — raw response: %s", string(respBody))
	return "", fmt.Errorf("llm: could not extract text from response")
}

// buildPrompt constructs the user-facing prompt for the OpenAI call.
// Kept separate from Explain() so it's easy to read and adjust.
func buildPrompt(
	titleA, descA, categoryA string,
	titleB, descB, categoryB string,
	sharedEntities []string,
	weight float64,
) string {
	// Truncate descriptions to keep prompts concise.
	descA = truncate(descA, maxDescriptionLen)
	descB = truncate(descB, maxDescriptionLen)

	entitiesStr := "none detected"
	if len(sharedEntities) > 0 {
		entitiesStr = strings.Join(sharedEntities, ", ")
	}

	return fmt.Sprintf(
		"Article 1 [%s]: %s\n%s\n\nArticle 2 [%s]: %s\n%s\n\nShared named entities: %s\nConnection strength: %.0f%%\n\nExplain the cross-topic connection in 1-2 sentences.",
		strings.ToUpper(categoryA), titleA, descA,
		strings.ToUpper(categoryB), titleB, descB,
		entitiesStr,
		weight*100,
	)
}

// truncate returns s trimmed to at most n characters, appending "…" if cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
