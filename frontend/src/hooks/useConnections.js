import { useState, useEffect } from 'react'

// Derive the WebSocket base URL from the same VITE_API_BASE_URL env var
// used by the REST API client. We just swap the protocol:
//   http://localhost:8080  →  ws://localhost:8080
//   https://api.example.com  →  wss://api.example.com
//
// This means there's a single place (the .env / docker-compose environment)
// that controls where both HTTP and WebSocket calls go.
const WS_BASE = (import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080')
  .replace(/^http/, 'ws')

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

    // Reset state whenever articleId changes (user opens a different article).
    setConnections([])
    setLoading(true)
    setError(null)

    const ws = new WebSocket(`${WS_BASE}/ws/connections/${articleId}`)

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)

        if (msg.type === 'connections') {
          // Initial graph traversal result — render immediately, no LLM wait.
          setConnections(msg.data.connections ?? [])
          setError(null)
          setLoading(false)

        } else if (msg.type === 'explanation') {
          const { article_id, explanation } = msg.data
          setConnections(prev =>
            prev.map(c =>
              c.article.id === article_id ? { ...c, explanation } : c
            )
          )
        }
      } catch {
        setError('Unexpected response from server.')
        setLoading(false)
      }
    }

    ws.onerror = () => {
      // onerror fires before onclose when the connection cannot be established
      // (e.g. backend not running, wrong port). We set a user-visible error
      // and stop showing the loading state.
      setError('Could not connect to the graph service.')
      setLoading(false)
    }

    ws.onclose = () => {
      // Normal close: server finished sending and held the connection until
      // the client (us) closed it on unmount. Loading may still be true if
      // the server closed before sending a message — clear it.
      setLoading(false)
    }

    // Cleanup: close the WebSocket when the component unmounts or articleId
    // changes. This is the React equivalent of "componentWillUnmount".
    // Without this, navigating away would leave an open connection on the
    // backend holding a goroutine open per article viewed.
    return () => {
      ws.close()
    }
  }, [articleId])

  return { connections, loading, error }
}
