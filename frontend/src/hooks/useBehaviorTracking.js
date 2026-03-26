import { useEffect, useRef } from 'react'
import { sendBehaviorEvent } from '../api/behavior'

// Throttle interval for scroll tracking. 500ms balances precision against
// the cost of repeated DOM reads on every scroll frame.
const SCROLL_THROTTLE_MS = 500

/**
 * useBehaviorTracking tracks implicit reading signals for a given article
 * and sends them to the backend when the user navigates away.
 *
 * Three signals:
 *   reopen      — fires immediately on mount (every article open is counted)
 *   scroll_depth — the deepest scroll position reached, sent on unmount
 *   dwell        — seconds spent reading, sent on unmount
 *
 * All signals are "fire and forget" — they do not affect the local UI.
 * The backend accumulates them and re-ranks the feed on the next GET /articles.
 *
 * Why useRef for maxScrollDepth and openedAt?
 * useRef creates a mutable container that persists for the component's lifetime
 * without triggering re-renders when changed. It's the Go equivalent of a
 * struct field — you mutate it in place. useState would cause unnecessary
 * re-renders on every scroll event.
 *
 * @param {object|null} article — the article being viewed (null = not viewing)
 * @param {string} token — Supabase JWT from session.access_token, forwarded
 *   to sendBehaviorEvent so the backend can attach a user ID to each event.
 */
export function useBehaviorTracking(article, token) {
  // useRef holds mutable values that should NOT trigger re-renders when updated.
  // This is different from useState — changing a ref never causes a re-render.
  const openedAtRef = useRef(null)
  const maxScrollDepthRef = useRef(0)
  const throttleTimerRef = useRef(null)

  useEffect(() => {
    if (!article?.id || !token) return

    // ── Initialise for this article ─────────────────────────────────────────
    openedAtRef.current = Date.now()
    maxScrollDepthRef.current = 0

    // ── Reopen signal — fires immediately ───────────────────────────────────
    // Every open is tracked, including re-opens. The backend increments a
    // counter; articles opened multiple times surface higher in the feed.
    sendBehaviorEvent({ article_id: article.id, type: 'reopen', value: 1 }, token)

    // ── Scroll depth tracking ───────────────────────────────────────────────
    function handleScroll() {
      // Throttle: only compute scroll depth every SCROLL_THROTTLE_MS ms.
      // Without throttling, this fires ~60 times/second on fast scrolls.
      if (throttleTimerRef.current) return
      throttleTimerRef.current = setTimeout(() => {
        throttleTimerRef.current = null

        const scrollable = document.documentElement.scrollHeight - window.innerHeight
        if (scrollable > 0) {
          // Clamp to [0, 1] — scrollY can slightly exceed scrollable on some browsers.
          const depth = Math.min(window.scrollY / scrollable, 1)
          if (depth > maxScrollDepthRef.current) {
            maxScrollDepthRef.current = depth
          }
        }
      }, SCROLL_THROTTLE_MS)
    }

    window.addEventListener('scroll', handleScroll, { passive: true })
    // passive: true tells the browser this listener never calls preventDefault(),
    // allowing the browser to optimise scroll performance.

    // ── Cleanup — fires when user navigates away ────────────────────────────
    // This runs when:
    //   - The user clicks "Back to feed" (selectedArticle → null)
    //   - The user clicks a connection in the sidebar (selectedArticle changes)
    //   - The component unmounts for any other reason
    return () => {
      window.removeEventListener('scroll', handleScroll)
      if (throttleTimerRef.current) {
        clearTimeout(throttleTimerRef.current)
        throttleTimerRef.current = null
      }

      const dwellSeconds = (Date.now() - openedAtRef.current) / 1000

      // Only send signals if the user actually spent time reading (>2s).
      // Sub-2s visits are accidental clicks or quick bounces — not meaningful signal.
      if (dwellSeconds > 2) {
        sendBehaviorEvent({ article_id: article.id, type: 'dwell', value: dwellSeconds }, token)
        sendBehaviorEvent({ article_id: article.id, type: 'scroll_depth', value: maxScrollDepthRef.current }, token)
      }
    }
  }, [article?.id]) // re-run if the user opens a different article
  // token is intentionally excluded from the dependency array — it's captured
  // by the cleanup closure at mount time and is stable within a session.
  // Adding it would cause the effect to re-run on every token refresh.
}
