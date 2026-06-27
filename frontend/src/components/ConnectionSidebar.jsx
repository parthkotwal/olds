import { useConnections } from '../hooks/useConnections'

// Category label abbreviations — keeps the sidebar tight.
const CATEGORY_SHORT = {
  general: 'WORLD',
  business: 'BIZ',
  technology: 'TECH',
  science: 'SCI',
  health: 'HEALTH',
  sports: 'SPORT',
  entertainment: 'CULTURE',
}

function categoryShort(cat) {
  return CATEGORY_SHORT[cat?.toLowerCase()] ?? cat?.toUpperCase() ?? '—'
}

// Weight → visual confidence label.
// The edge weight is a 0–1 float combining cosine similarity + entity overlap.
// We bucket it into three human-readable tiers for the sidebar.
function weightLabel(w) {
  if (w >= 0.65) return 'strong'
  if (w >= 0.35) return 'related'
  return 'weak'
}

/**
 * A single connection entry in the sidebar.
 * Rendered as text-only marginalia — no cards, no borders, no shadows.
 * The cross-topic badge is the visual highlight when the connection crosses categories.
 */
function ConnectionEntry({ connection, onArticleClick }) {
  const { article, weight, cross_topic, explanation } = connection

  return (
    <button
      onClick={() => onArticleClick?.(article)}
      className="w-full text-left group"
      style={{ paddingBottom: '1rem', marginBottom: '1rem', borderBottom: '1px solid var(--color-rule)' }}
    >
      {/* Category + cross-topic badge */}
      <div className="flex items-center gap-1.5 mb-1">
        <span className="label-caps text-muted" style={{ fontSize: '0.6rem' }}>
          {categoryShort(article.category)}
        </span>
        {cross_topic && (
          <span
            className="label-caps"
            style={{
              fontSize: '0.55rem',
              color: 'var(--color-ink)',
              background: 'var(--color-accent)',
              padding: '1px 4px',
              letterSpacing: '0.05em',
            }}
          >
            cross
          </span>
        )}
      </div>

      {/* Headline — serif, quiet hover */}
      <p
        className="font-display text-ink leading-snug group-hover:opacity-70 transition-opacity duration-150"
        style={{
          fontSize: '0.86rem',
          fontWeight: 400,
          letterSpacing: '-0.02em',
          display: '-webkit-box',
          WebkitLineClamp: 3,
          WebkitBoxOrient: 'vertical',
          overflow: 'hidden',
        }}
      >
        {article.title}
      </p>

      {/* LLM-generated explanation — streams in after the initial connection
          render via a separate WebSocket message. */}
      {explanation && (
        <p
          className="text-muted"
          style={{
            fontSize: '0.62rem',
            fontStyle: 'italic',
            lineHeight: 1.6,
            marginTop: '0.5rem',
            paddingTop: '0.4rem',
            borderTop: '1px solid var(--color-rule)',
          }}
        >
          {explanation}
        </p>
      )}

      {/* Source + weight tier */}
      <div className="flex items-center justify-between mt-1.5">
        <span className="text-muted" style={{ fontSize: '0.6rem' }}>
          {article.source}
        </span>
        <span
          className="label-caps text-muted"
          style={{ fontSize: '0.55rem' }}
          title={`Edge weight: ${weight.toFixed(3)}`}
        >
          {weightLabel(weight)}
        </span>
      </div>
    </button>
  )
}

/**
 * ConnectionSidebar renders the live graph traversal results.
 *
 * It opens a WebSocket via useConnections() the moment the article mounts,
 * shows a quiet loading state while the Go backend traverses the graph,
 * then fades in each connection as marginalia beside the article text.
 *
 * Design intent: feels like a newspaper's "see also" column, not a
 * recommendation engine. Quiet, typography-led, no cards or borders.
 *
 * Props:
 *   articleId     string   — article being read (drives the WebSocket)
 *   onArticleClick fn      — called when a connection headline is clicked
 *   className     string   — optional extra classes for positioning
 */
export default function ConnectionSidebar({ articleId, onArticleClick, className = '' }) {
  const { connections, loading, error } = useConnections(articleId)

  const crossTopicConnections = connections.filter(c => c.cross_topic)
  const sameTopicConnections = connections.filter(c => !c.cross_topic)

  return (
    <div className={className}>
      {/* Section header — label-caps, matches entity tags style */}
      <div className="label-caps text-muted mb-4" style={{ fontSize: '0.65rem', letterSpacing: '0.12em' }}>
        Connections
      </div>

      {/* ── Loading state ───────────────────────────────────────────────── */}
      {/* A pulsing rule signals "working quietly" without any text.
          The animation is defined in index.css as @keyframes pulse. */}
      {loading && (
        <div>
          {/* Three lines of varying width mimic the shape of incoming headlines */}
          {[78, 62, 70].map((w, i) => (
            <div
              key={i}
              style={{
                height: '0.6rem',
                width: `${w}%`,
                background: 'var(--color-rule)',
                marginBottom: '0.5rem',
                animation: `pulse 1.6s ease-in-out ${i * 160}ms infinite`,
              }}
            />
          ))}
          {/* A fourth shorter line after a gap — feels like a byline */}
          <div
            style={{
              height: '0.45rem',
              width: '45%',
              background: 'var(--color-rule)',
              marginTop: '0.75rem',
              animation: 'pulse 1.6s ease-in-out 480ms infinite',
            }}
          />
        </div>
      )}

      {/* ── Error state ─────────────────────────────────────────────────── */}
      {!loading && error && (
        <p className="text-muted italic" style={{ fontSize: '0.65rem', lineHeight: 1.5 }}>
          {error}
        </p>
      )}

      {/* ── Empty state ─────────────────────────────────────────────────── */}
      {/* An intentional editorial statement rather than a system message. */}
      {!loading && !error && connections.length === 0 && (
        <div style={{ animation: 'fadeIn 300ms ease-out both' }}>
          <p
            className="font-display text-muted"
            style={{ fontSize: '0.75rem', fontStyle: 'italic', lineHeight: 1.6, marginBottom: '0.5rem' }}
          >
            This story stands alone.
          </p>
          <p className="text-muted" style={{ fontSize: '0.6rem', lineHeight: 1.5 }}>
            No cross-topic connections found yet.
          </p>
        </div>
      )}

      {/* ── Cross-topic connections (highlighted) ───────────────────────── */}
      {!loading && crossTopicConnections.length > 0 && (
        <div
          style={{
            animation: 'fadeIn 250ms ease-out',
          }}
        >
          <div
            className="label-caps"
            style={{
              fontSize: '0.55rem',
              color: 'var(--color-ink)',
              letterSpacing: '0.1em',
              marginBottom: '0.75rem',
            }}
          >
            Cross-topic
          </div>
          {crossTopicConnections.map((conn) => (
            <ConnectionEntry
              key={conn.article.id}
              connection={conn}
              onArticleClick={onArticleClick}
            />
          ))}
        </div>
      )}

      {/* ── Same-topic connections ───────────────────────────────────────── */}
      {!loading && sameTopicConnections.length > 0 && (
        <div
          style={{
            marginTop: crossTopicConnections.length > 0 ? '1.25rem' : 0,
            animation: 'fadeIn 250ms ease-out',
          }}
        >
          {crossTopicConnections.length > 0 && (
            <div
              className="label-caps text-muted"
              style={{ fontSize: '0.55rem', letterSpacing: '0.1em', marginBottom: '0.75rem' }}
            >
              Same topic
            </div>
          )}
          {sameTopicConnections.map((conn) => (
            <ConnectionEntry
              key={conn.article.id}
              connection={conn}
              onArticleClick={onArticleClick}
            />
          ))}
        </div>
      )}

    </div>
  )
}
