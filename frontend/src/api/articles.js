const API_BASE = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'

/**
 * Fetch a page of articles from the backend.
 *
 * @param {Object} options
 * @param {string} options.category - Category filter (empty string for all).
 * @param {number} options.page - 1-indexed page number.
 * @param {number} options.pageSize - Articles per page.
 * @returns {Promise<{ articles: Article[], total: number, page: number }>}
 */
export async function fetchArticles({ category = '', page = 1, pageSize = 30 } = {}) {
  const params = new URLSearchParams()
  if (category) params.set('category', category)
  params.set('page', String(page))
  params.set('page_size', String(pageSize))

  const response = await fetch(`${API_BASE}/articles?${params}`)

  if (!response.ok) {
    throw new Error(`Backend returned ${response.status} ${response.statusText}`)
  }

  const data = await response.json()
  return {
    articles: data.articles ?? [],
    total: data.total ?? 0,
    page: data.page ?? 1,
  }
}

/**
 * Fetch a single article with full detail (raw_text, entities).
 */
export async function fetchArticleById(id) {
  const response = await fetch(`${API_BASE}/articles/${encodeURIComponent(id)}`)

  if (!response.ok) {
    throw new Error(`Backend returned ${response.status} ${response.statusText}`)
  }

  return response.json()
}
