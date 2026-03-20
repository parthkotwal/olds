// Base URL for the Go backend API.
// In Docker: set VITE_API_BASE_URL=http://localhost:8080 in docker-compose.yml.
// Locally (npm run dev): defaults to localhost:8080 which is where the backend runs.
//
// import.meta.env is Vite's way to access environment variables at build time.
// Variables must be prefixed with VITE_ to be exposed to the browser bundle —
// unprefixed variables are server-side only and will be undefined here.
const API_BASE = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'

/**
 * Fetch articles from the backend, optionally filtered by category.
 *
 * @param {string} category - NewsAPI category slug (e.g. "business"). Pass ""
 *   or omit to fetch all categories.
 * @returns {Promise<Article[]>} - Array of article objects from the Go backend.
 */
export async function fetchArticles(category = '') {
  const url = category
    ? `${API_BASE}/articles?category=${encodeURIComponent(category)}`
    : `${API_BASE}/articles`

  const response = await fetch(url)

  if (!response.ok) {
    throw new Error(`Backend returned ${response.status} ${response.statusText}`)
  }

  const data = await response.json()
  // The Go backend wraps articles in { articles: [...], count: N }.
  // Guard against null (empty store returns null in Go — see handler nil check).
  return data.articles ?? []
}
