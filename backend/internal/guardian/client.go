// Package guardian provides a client for The Guardian's Content API.
//
// Unlike NewsAPI, The Guardian's free tier returns the full article body HTML
// via the show-fields=body parameter. We strip the HTML tags server-side so
// the ML service receives clean plain text, and the frontend gets readable prose.
package guardian

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/olds/backend/internal/article"
)

// sectionMap maps our internal category names (shared with NewsAPI) to
// Guardian section slugs. Guardian uses different names than NewsAPI —
// "sport" (singular), "world" for general news, "culture" for entertainment.
//
// We keep the same category vocabulary internally so the frontend filter
// works across both sources without needing to know where articles came from.
var sectionMap = map[string]string{
	"general":       "world",
	"business":      "business",
	"technology":    "technology",
	"science":       "science",
	"health":        "society",
	"sports":        "sport",
	"entertainment": "culture",
}

// Categories is the list of internal category names we fetch from The Guardian.
// Keep this in sync with newsapi.Categories to avoid gaps in the feed.
var Categories = []string{"general", "business", "technology", "science", "sports", "entertainment"}

// htmlTagRe matches any HTML tag. Used by stripHTML to remove markup from
// article body content before storing it for ML processing and display.
// Note: regex-based HTML stripping is not perfect (it can't handle malformed
// or deeply nested tags), but it is fast and good enough for article bodies
// from a trusted source like The Guardian.
var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// stripHTML removes HTML tags from s, unescapes HTML entities (e.g. &amp; → &),
// and normalises whitespace so the result is clean prose.
//
// Example:
//
//	"<p>Hello &amp; <strong>world</strong></p>" → "Hello & world"
func stripHTML(s string) string {
	// Replace all tags with a space (not empty string) to avoid words running
	// together when tags have no surrounding whitespace: "foo<br>bar" → "foo bar"
	noTags := htmlTagRe.ReplaceAllString(s, " ")
	// html.UnescapeString from the standard library handles the common entities:
	// &amp; &lt; &gt; &quot; &apos; &nbsp; and numeric references like &#39;
	unescaped := html.UnescapeString(noTags)
	// strings.Fields splits on any whitespace and drops empty tokens, then
	// Join reassembles with single spaces — equivalent to Python's " ".join(s.split())
	return strings.TrimSpace(strings.Join(strings.Fields(unescaped), " "))
}

// ── Wire types ────────────────────────────────────────────────────────────────
// These mirror the JSON structure returned by api.theguardian.com/search.
// Unexported because nothing outside this package should know the wire format.

type guardianResponse struct {
	Response guardianResponseBody `json:"response"`
}

type guardianResponseBody struct {
	Status  string            `json:"status"`
	Results []guardianArticle `json:"results"`
}

type guardianArticle struct {
	ID                 string         `json:"id"`     // e.g. "world/2026/mar/19/..."
	WebTitle           string         `json:"webTitle"`
	WebURL             string         `json:"webUrl"`
	WebPublicationDate time.Time      `json:"webPublicationDate"`
	SectionID          string         `json:"sectionId"`
	Fields             guardianFields `json:"fields"`
}

type guardianFields struct {
	// trailText is The Guardian's short article summary — equivalent to
	// NewsAPI's description. Better written than a truncated sentence.
	TrailText string `json:"trailText"`
	// body is the full article HTML — the key advantage over NewsAPI's free tier.
	Body string `json:"body"`
	// thumbnail is a CDN image URL for the article's lead image.
	Thumbnail string `json:"thumbnail"`
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client is an HTTP client for The Guardian Content API.
type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a ready-to-use Guardian API client.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: "https://content.guardianapis.com",
		httpClient: &http.Client{
			Timeout: 15 * time.Second, // Guardian bodies are larger than NewsAPI responses
		},
	}
}

// FetchCategory retrieves the newest articles for the given internal category
// from The Guardian, returning them as domain Article values.
//
// The Guardian's full article body is stripped of HTML and stored in RawText —
// this is what the ML service will embed and extract entities from in Phase 4.
func (c *Client) FetchCategory(category string) ([]article.Article, error) {
	section, ok := sectionMap[category]
	if !ok {
		return nil, fmt.Errorf("guardian: no section mapping for category %q", category)
	}

	params := url.Values{}
	params.Set("api-key", c.apiKey)
	params.Set("section", section)
	params.Set("page-size", "20")
	params.Set("order-by", "newest")
	// show-fields requests additional data not in the default response:
	//   body      — full article HTML (The Guardian's key free-tier feature)
	//   trailText — editorial summary (better than auto-truncated descriptions)
	//   thumbnail — lead image CDN URL
	params.Set("show-fields", "body,trailText,thumbnail")
	endpoint := c.baseURL + "/search?" + params.Encode()

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("guardian fetch failed for category %q: %w", category, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("guardian returned HTTP %d for category %q", resp.StatusCode, category)
	}

	var result guardianResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("guardian decode failed: %w", err)
	}

	if result.Response.Status != "ok" {
		return nil, fmt.Errorf("guardian error status %q for category %q", result.Response.Status, category)
	}

	articles := make([]article.Article, 0, len(result.Response.Results))
	for _, a := range result.Response.Results {
		if a.WebURL == "" || a.WebTitle == "" {
			continue
		}

		// Strip HTML from the full body for clean plain text.
		// trailText also contains light HTML (e.g. <strong>) — strip that too.
		bodyText := stripHTML(a.Fields.Body)
		trailText := stripHTML(a.Fields.TrailText)

		articles = append(articles, article.Article{
			ID:          deriveID(a.WebURL),
			Title:       a.WebTitle,
			Description: trailText, // editorial summary as the short description
			URL:         a.WebURL,
			ImageURL:    a.Fields.Thumbnail,
			Source:      "The Guardian",
			Category:    category,
			PublishedAt: a.WebPublicationDate,
			RawText:     bodyText, // full article text — sent to the ML service
		})
	}
	return articles, nil
}

// FetchAll fetches articles for all Categories and returns the combined slice.
// Mirrors the same pattern as newsapi.Client.FetchAll.
func (c *Client) FetchAll() ([]article.Article, error) {
	var all []article.Article
	for _, cat := range Categories {
		articles, err := c.FetchCategory(cat)
		if err != nil {
			return nil, fmt.Errorf("guardian FetchAll: %w", err)
		}
		all = append(all, articles...)
	}
	return all, nil
}

// deriveID generates a deterministic ID by MD5-hashing the article URL.
// Same approach as the NewsAPI client — consistent IDs across restarts
// and safe deduplication on re-ingestion.
func deriveID(rawURL string) string {
	sum := md5.Sum([]byte(rawURL))
	return fmt.Sprintf("%x", sum)
}
