import { useState, useEffect } from 'react'
import { fetchConnectionExplanations, fetchConnections } from '../api/articles.js'

// Derive the WebSocket base URL from the same VITE_API_BASE_URL env var
// used by the REST API client. We just swap the protocol:
//   http://localhost:8080  →  ws://localhost:8080
//   https://api.example.com  →  wss://api.example.com
//
// This means there's a single place (the .env / docker-compose environment)
// that controls where both HTTP and WebSocket calls go.
const WS_BASE = (import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080')
  .replace(/^http/, 'ws')

const WS_RETRY_DELAYS_MS = [1500, 3500]

/**
 * useConnections opens a WebSocket to /ws/connections/:articleId,
 * receives the graph traversal result, and returns it as React state.
 *
 * The connection lifecycle is tied to the component via useEffect cleanup:
 * when ArticleView unmounts (user hits "Back to feed"), the WebSocket is
 * closed automatically — no dangling connections.
 *
 * @param {string} articleId - The article ID to find connections for.
 * @returns {{ connections: Connection[], loading: boolean, error: string|null }}
 */
export function useConnections(articleId) {
  const [connections, setConnections] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    if (!articleId) return

    let cancelled = false
    let retryTimer = null
    let ws = null

    // Reset state whenever articleId changes (user opens a different article).
    setConnections([])
    setLoading(true)
    setError(null)

    async function loadInitialConnections() {
      if (cancelled) return

      try {
        const data = await fetchConnections(articleId)
        if (cancelled) return
        setConnections((data.connections ?? []).map(connection => ({
          ...connection,
          explanation_pending: true,
        })))
        setError(null)
        setLoading(false)
        connectForExplanations()
        loadExplanationFallback()
      } catch {
        if (cancelled) return
        setError('Could not load connections.')
        setLoading(false)
      }
    }

    async function loadExplanationFallback() {
      try {
        const data = await fetchConnectionExplanations(articleId)
        if (cancelled) return

        const explanationsById = new Map(
          (data.connections ?? [])
            .filter(connection => connection.explanation)
            .map(connection => [connection.article.id, connection.explanation])
        )

        setConnections(prev =>
          prev.map(connection => {
            const explanation = explanationsById.get(connection.article.id)
            return {
              ...connection,
              explanation: connection.explanation ?? explanation,
              explanation_pending: false,
              explanation_unavailable: !connection.explanation && !explanation,
            }
          })
        )
      } catch {
        if (cancelled) return
        setConnections(prev =>
          prev.map(connection => ({
            ...connection,
            explanation_pending: false,
            explanation_unavailable: !connection.explanation,
          }))
        )
      }
    }

    function connectForExplanations(attempt = 0) {
      if (cancelled) return

      ws = new WebSocket(`${WS_BASE}/ws/connections/${articleId}`)

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)

          if (msg.type === 'explanation') {
            const { article_id, explanation } = msg.data
            setConnections(prev =>
              prev.map(c =>
                c.article.id === article_id
                  ? { ...c, explanation, explanation_pending: false }
                  : c
              )
            )
          }
        } catch {
          // Explanations are progressive enhancement; initial connections have
          // already loaded over HTTP, so ignore malformed stream messages.
        }
      }

      ws.onerror = () => {
        // The browser does not expose WebSocket HTTP status codes. During
        // backend cold starts, Railway returns a temporary 503 while the graph
        // hydrates; onclose below handles retry without flashing an error.
      }

      ws.onclose = () => {
        if (cancelled) {
          return
        }

        const nextAttempt = attempt + 1
        if (nextAttempt <= WS_RETRY_DELAYS_MS.length) {
          retryTimer = setTimeout(
            () => connectForExplanations(nextAttempt),
            WS_RETRY_DELAYS_MS[nextAttempt - 1],
          )
        }
      }
    }

    loadInitialConnections()
    const pendingFallback = setTimeout(() => {
      if (!cancelled) {
        setConnections(prev =>
          prev.map(c => ({
            ...c,
            explanation_pending: false,
            explanation_unavailable: !c.explanation,
          }))
        )
      }
    }, 45000)

    // Cleanup: close the WebSocket when the component unmounts or articleId
    // changes. This is the React equivalent of "componentWillUnmount".
    // Without this, navigating away would leave an open connection on the
    // backend holding a goroutine open per article viewed.
    return () => {
      cancelled = true
      clearTimeout(pendingFallback)
      if (retryTimer) clearTimeout(retryTimer)
      if (ws) ws.close()
    }
  }, [articleId])

  return { connections, loading, error }
}
