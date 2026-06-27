const API_BASE = (import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080').replace(/\/$/, '')

const REQUEST_TIMEOUT_MS = 8000
const CONNECTIONS_TIMEOUT_MS = 3500
const LIST_RETRY_DELAYS_MS = [250, 750, 1500, 3000]

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

function isTransientStatus(status) {
  return status === 408 || status === 425 || status === 429 || (status >= 500 && status <= 504)
}

async function fetchWithTimeout(url, options = {}, timeoutMs = REQUEST_TIMEOUT_MS) {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), timeoutMs)

  try {
    return await fetch(url, {
      ...options,
      signal: controller.signal,
    })
  } finally {
    clearTimeout(timeout)
  }
}

async function fetchJsonWithRetry(url, {
  retryDelays = LIST_RETRY_DELAYS_MS,
  shouldRetryData = () => false,
} = {}) {
  let lastError

  for (let attempt = 0; attempt <= retryDelays.length; attempt += 1) {
    try {
      const response = await fetchWithTimeout(url, {
        headers: { Accept: 'application/json' },
      })

      if (!response.ok) {
        const error = new Error(`Backend returned ${response.status} ${response.statusText}`)
        error.status = response.status
        throw error
      }

      const data = await response.json()
      if (!shouldRetryData(data) || attempt === retryDelays.length) {
        return data
      }

      lastError = new Error('Backend returned an empty feed while warming up')
    } catch (error) {
      lastError = error

      const canRetryStatus = error.status == null || isTransientStatus(error.status)
      if (!canRetryStatus || attempt === retryDelays.length) {
        throw error
      }
    }

    await sleep(retryDelays[attempt])
  }

  throw lastError
}

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

  const data = await fetchJsonWithRetry(`${API_BASE}/articles?${params}`, {
    shouldRetryData: responseData => (
      page === 1
      && !category
      && (responseData.total ?? 0) === 0
      && (responseData.articles ?? []).length === 0
    ),
  })

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
  return fetchJsonWithRetry(`${API_BASE}/articles/${encodeURIComponent(id)}`, {
    retryDelays: [250, 750, 1500],
  })
}

export async function fetchConnections(id) {
  const params = new URLSearchParams({
    top_n: '10',
    min_weight: '0.1',
    explain: 'false',
  })

  const response = await fetchWithTimeout(
    `${API_BASE}/articles/${encodeURIComponent(id)}/connections?${params}`,
    { headers: { Accept: 'application/json' } },
    CONNECTIONS_TIMEOUT_MS,
  )

  if (!response.ok) {
    throw new Error(`Backend returned ${response.status} ${response.statusText}`)
  }

  return response.json()
}
