const API_BASE = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'

/**
 * Send a single behavioral event to the backend.
 *
 * Uses fetch with keepalive: true so the request survives component unmount
 * and tab close. This is critical for dwell/scroll_depth events which are
 * fired in useEffect cleanup (i.e. when the user navigates away).
 *
 * keepalive is the web-standard alternative to navigator.sendBeacon for
 * JSON payloads — sendBeacon only supports plain text or FormData.
 *
 * Errors are swallowed intentionally: behavior tracking is best-effort.
 * A failed event should never surface as a UI error to the user.
 *
 * @param {{ article_id: string, type: 'dwell'|'scroll_depth'|'reopen', value: number }} event
 */
export async function sendBehaviorEvent(event) {
  try {
    await fetch(`${API_BASE}/behavior`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(event),
      keepalive: true, // survives component unmount and tab close
    })
  } catch {
    // Silently ignore — behavior tracking is non-critical.
    // If the backend is down, the feed still works; it just won't re-rank.
  }
}
