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
  const { article, weight, cross_topic } = connection

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
              color: 'var(--color-accent)',
              border: '1px solid var(--color-accent)',
              padding: '0 3px',
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
          fontSize: '0.8rem',
          fontWeight: 600,
          display: '-webkit-box',
          WebkitLineClamp: 3,
          WebkitBoxOrient: 'vertical',
          overflow: 'hidden',
        }}
      >
        {article.title}
      </p>

      {/* Source + weight tier */}
      <div className="flex items-center justify-between mt-1">
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
      {loading && (
        <div style={{ opacity: 0.4 }}>
          {/* Three skeleton lines suggest content is coming */}
          {[80, 65, 72].map((w, i) => (
            <div
              key={i}
              style={{
                height: '0.65rem',
                width: `${w}%`,
                background: 'var(--color-muted)',
                marginBottom: '0.5rem',
                opacity: 0.3,
              }}
            />
          ))}
          <p className="text-muted" style={{ fontSize: '0.65rem', marginTop: '0.75rem' }}>
            Traversing graph…
          </p>
        </div>
      )}

      {/* ── Error state ─────────────────────────────────────────────────── */}
      {!loading && error && (
        <p className="text-muted italic" style={{ fontSize: '0.65rem', lineHeight: 1.5 }}>
          {error}
        </p>
      )}

      {/* ── Empty state ─────────────────────────────────────────────────── */}
      {!loading && !error && connections.length === 0 && (
        <p className="text-muted italic" style={{ fontSize: '0.65rem', lineHeight: 1.5 }}>
          No connected stories found yet. Check back after more articles are ingested.
        </p>
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
              color: 'var(--color-accent)',
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

      {/* Fade-in keyframe — inlined so it doesn't require a separate CSS file */}
      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; }
          to   { opacity: 1; }
        }
      `}</style>
    </div>
  )
}
