// Package newsapi provides a client for the NewsAPI.org REST API.
// It handles all HTTP communication with NewsAPI and converts the wire
// format into the application's domain Article type.
//
// Nothing outside this package needs to know what the NewsAPI JSON looks like.
package newsapi

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/olds/backend/internal/article"
)

// Categories is the list of NewsAPI topic categories we ingest.
// These map reasonably well to the CLAUDE.md interest areas
// (global affairs → general, sports → sports, etc.).
var Categories = []string{"general", "technology", "science", "health"}

// Client is an HTTP client for the NewsAPI.org /v2/top-headlines endpoint.
//
// We wrap http.Client rather than using http.Get (the package-level default)
// because http.DefaultClient has no timeout — if NewsAPI is slow or unreachable,
// a call without a timeout blocks forever, hanging the goroutine. Always set a
// timeout on outbound HTTP clients.
type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a ready-to-use NewsAPI client.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: "https://newsapi.org",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ── Wire types ────────────────────────────────────────────────────────────────
// These structs mirror the JSON shape returned by NewsAPI. They are unexported
// (lowercase first letter) because they are an implementation detail — nothing
// outside this package needs to know the wire format. The application works with
// article.Article exclusively. This is the Go equivalent of a private inner class.

type topHeadlinesResponse struct {
	Status   string           `json:"status"`
	Articles []newsAPIArticle `json:"articles"`
}

type newsAPIArticle struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Content     string    `json:"content"`
	Source      struct {
		Name string `json:"name"`
	} `json:"source"`
	// time.Time with a json tag: encoding/json automatically parses
	// ISO 8601 strings ("2024-03-19T12:00:00Z") into time.Time values.
	// No manual parsing needed — this is one of Go's stdlib conveniences.
	PublishedAt time.Time `json:"publishedAt"`
}

// ── Public methods ────────────────────────────────────────────────────────────

// FetchCategory retrieves the top headlines for a single category from NewsAPI.
// It returns a slice of domain Article values, or an error if the fetch failed.
//
// In Go, the convention for functions that can fail is to return (result, error)
// as the last two return values. The caller always checks the error before using
// the result. This replaces try/except — errors are values, not exceptions.
func (c *Client) FetchCategory(category string) ([]article.Article, error) {
	// Build the request URL using url.Values — the standard library's
	// equivalent of URLSearchParams in JavaScript or urllib.parse.urlencode
	// in Python. It handles percent-encoding automatically.
	params := url.Values{}
	params.Set("apiKey", c.apiKey)
	params.Set("category", category)
	params.Set("language", "en")
	params.Set("pageSize", "20")
	endpoint := c.baseURL + "/v2/top-headlines?" + params.Encode()

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		// %w wraps the original error, preserving it for callers who want to
		// inspect it with errors.Is() or errors.As(). Think of it as:
		//   raise RuntimeError("newsapi fetch failed") from original_err  (Python)
		// The error chain is: "newsapi fetch failed: <original http error>"
		return nil, fmt.Errorf("newsapi fetch failed for category %q: %w", category, err)
	}
	// defer schedules resp.Body.Close() to run when FetchCategory returns.
	// Response bodies are I/O streams — if you don't close them, the underlying
	// TCP connection is never returned to the pool, causing a slow leak.
	// Forgetting this is one of the most common Go mistakes; defer makes it
	// impossible to forget if you write it immediately after the call.
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("newsapi returned HTTP %d for category %q", resp.StatusCode, category)
	}

	var result topHeadlinesResponse
	// json.NewDecoder streams directly from the response body — more efficient
	// than reading the whole body into a []byte first (no intermediate allocation).
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("newsapi decode failed: %w", err)
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("newsapi error status %q for category %q", result.Status, category)
	}

	// Convert wire format → domain type.
	// make([]article.Article, 0, len(result.Articles)) pre-allocates the slice
	// with length 0 and capacity == expected count, so append never needs to
	// reallocate as the slice grows. Minor optimization for clarity.
	articles := make([]article.Article, 0, len(result.Articles))
	for _, a := range result.Articles {
		// NewsAPI sometimes returns placeholder articles with empty fields — skip them.
		if a.URL == "" || a.Title == "" {
			continue
		}
		articles = append(articles, article.Article{
			ID:          deriveID(a.URL),
			Title:       a.Title,
			Description: a.Description,
			URL:         a.URL,
			Source:      a.Source.Name,
			Category:    category,
			PublishedAt: a.PublishedAt,
			RawText:     a.Content,
		})
	}
	return articles, nil
}

// FetchAll fetches top headlines for all Categories and returns the combined results.
// Individual category errors are logged but do not abort the whole fetch —
// a partial result is better than nothing.
func (c *Client) FetchAll() ([]article.Article, error) {
	var all []article.Article
	for _, cat := range Categories {
		articles, err := c.FetchCategory(cat)
		if err != nil {
			// Return the error immediately. The caller (the startup goroutine
			// and the /ingest handler) can decide how to handle it.
			return nil, fmt.Errorf("FetchAll: %w", err)
		}
		all = append(all, articles...)
	}
	return all, nil
}

// ── Private helpers ───────────────────────────────────────────────────────────

// deriveID generates a deterministic string ID by MD5-hashing the article URL.
// Using the URL as input means the same article always gets the same ID,
// which gives the Store free deduplication on re-ingestion.
//
// MD5 is fine here — we are not doing cryptography, just making a short,
// stable, unique-enough identifier from a URL.
func deriveID(rawURL string) string {
	// md5.Sum returns a [16]byte array (fixed-size, not a slice).
	// fmt.Sprintf with %x formats it as a 32-character lowercase hex string.
	sum := md5.Sum([]byte(rawURL))
	return fmt.Sprintf("%x", sum)
}
