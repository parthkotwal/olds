import { useMemo, useState } from 'react'
import { useConnections } from '../hooks/useConnections'

const CATEGORY_LABELS = {
  general: 'World',
  business: 'Business',
  technology: 'Tech',
  science: 'Science',
  health: 'Health',
  sports: 'Sport',
  entertainment: 'Culture',
}

function categoryLabel(category) {
  return CATEGORY_LABELS[category?.toLowerCase()] ?? category ?? 'Story'
}

function shortTitle(title, max = 54) {
  if (!title) return ''
  return title.length > max ? `${title.slice(0, max - 1)}…` : title
}

function pct(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '0%'
  return `${Math.round(value)}%`
}

function score(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '0.00'
  return value.toFixed(2)
}

function buildGraph(article, connections) {
  const width = 760
  const height = 460
  const center = { x: width / 2, y: height / 2 }
  const radius = 165
  const items = connections.slice(0, 10)

  const nodes = [
    {
      id: article.id,
      article,
      kind: 'source',
      x: center.x,
      y: center.y,
      r: 34,
    },
    ...items.map((connection, index) => {
      const angle = (-Math.PI / 2) + (index * 2 * Math.PI / Math.max(items.length, 1))
      return {
        id: connection.article.id,
        article: connection.article,
        connection,
        kind: 'connection',
        x: center.x + Math.cos(angle) * radius,
        y: center.y + Math.sin(angle) * radius,
        r: 22 + Math.min(connection.weight, 1) * 16,
      }
    }),
  ]

  const edges = items.map(connection => {
    const target = nodes.find(node => node.id === connection.article.id)
    return {
      id: connection.article.id,
      connection,
      x1: center.x,
      y1: center.y,
      x2: target.x,
      y2: target.y,
    }
  })

  return { width, height, nodes, edges }
}

export default function ConnectionGraphView({ article, onShowSource, onArticleClick }) {
  const { connections, loading, error } = useConnections(article.id)
  const [selectedId, setSelectedId] = useState(null)

  const graph = useMemo(() => buildGraph(article, connections), [article, connections])
  const selectedConnection = connections.find(connection => connection.article.id === selectedId) ?? connections[0]

  return (
    <div>
      <section className="border-y border-rule py-5 mb-6">
        <div className="editorial-label text-ink mb-2">Graph Explorer</div>
        <h1
          className="font-display font-normal text-ink leading-tight headline-tight max-w-3xl"
          style={{ fontSize: 'clamp(1.8rem, 4vw, 2.8rem)' }}
        >
          {article.title}
        </h1>
        <button
          onClick={onShowSource}
          className="label-caps text-muted hover:text-ink transition-colors mt-3"
          style={{ fontSize: '0.62rem' }}
        >
          Back to source article
        </button>
      </section>

      {loading && (
        <p className="text-muted text-sm py-8">Loading graph connections…</p>
      )}

      {!loading && error && (
        <p className="text-muted text-sm py-8">{error}</p>
      )}

      {!loading && !error && connections.length === 0 && (
        <p className="text-muted text-sm py-8 italic">No graph connections available yet.</p>
      )}

      {!loading && !error && connections.length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_19rem] gap-8">
          <div className="border border-rule bg-paper overflow-x-auto">
            <svg
              viewBox={`0 0 ${graph.width} ${graph.height}`}
              role="img"
              aria-label="Article connection graph"
              className="w-full min-w-[680px] h-auto"
            >
              <rect width={graph.width} height={graph.height} fill="var(--color-paper)" />

              {graph.edges.map(edge => {
                const active = edge.id === selectedConnection?.article.id
                return (
                  <g key={edge.id}>
                    <line
                      x1={edge.x1}
                      y1={edge.y1}
                      x2={edge.x2}
                      y2={edge.y2}
                      stroke={active ? 'var(--color-ink)' : 'var(--color-rule)'}
                      strokeWidth={active ? 3 : 1 + Math.max(edge.connection.weight * 4, 1)}
                    />
                    <text
                      x={(edge.x1 + edge.x2) / 2}
                      y={(edge.y1 + edge.y2) / 2 - 6}
                      textAnchor="middle"
                      className="label-caps"
                      fill={active ? 'var(--color-ink)' : 'var(--color-muted)'}
                      style={{ fontSize: '8px' }}
                    >
                      {score(edge.connection.weight)}
                    </text>
                  </g>
                )
              })}

              {graph.nodes.map(node => {
                const active = node.kind === 'source' || node.id === selectedConnection?.article.id
                return (
                  <g
                    key={node.id}
                    role={node.kind === 'source' ? undefined : 'button'}
                    tabIndex={node.kind === 'source' ? undefined : 0}
                    onClick={() => node.connection && setSelectedId(node.id)}
                    onKeyDown={event => {
                      if (node.connection && (event.key === 'Enter' || event.key === ' ')) {
                        event.preventDefault()
                        setSelectedId(node.id)
                      }
                    }}
                    style={{ cursor: node.connection ? 'pointer' : 'default' }}
                  >
                    <circle
                      cx={node.x}
                      cy={node.y}
                      r={node.r}
                      fill={node.kind === 'source' ? 'var(--color-ink)' : (node.connection?.cross_topic ? 'var(--color-accent)' : 'var(--color-warm)')}
                      stroke="var(--color-ink)"
                      strokeWidth={active ? 2 : 1}
                    />
                    <text
                      x={node.x}
                      y={node.y + 3}
                      textAnchor="middle"
                      className="label-caps"
                      fill={node.kind === 'source' ? 'var(--color-paper)' : 'var(--color-ink)'}
                      style={{ fontSize: node.kind === 'source' ? '9px' : '8px' }}
                    >
                      {node.kind === 'source' ? 'SOURCE' : categoryLabel(node.article.category).toUpperCase()}
                    </text>
                    {node.kind !== 'source' && (
                      <text
                        x={node.x}
                        y={node.y + node.r + 16}
                        textAnchor="middle"
                        fill="var(--color-muted)"
                        style={{ fontSize: '11px' }}
                      >
                        {shortTitle(node.article.source, 24)}
                      </text>
                    )}
                  </g>
                )
              })}
            </svg>
          </div>

          <aside className="border-l border-rule pl-6">
            {selectedConnection && (
              <>
                <div className="label-caps text-muted mb-2" style={{ fontSize: '0.6rem' }}>
                  Selected Edge
                </div>
                <button
                  onClick={() => onArticleClick(selectedConnection.article)}
                  className="font-display text-left text-ink leading-snug hover:underline headline-tight mb-3"
                  style={{ fontSize: '1.25rem', textDecorationThickness: '1px', textUnderlineOffset: '4px' }}
                >
                  {selectedConnection.article.title}
                </button>
                <div className="label-caps text-muted mb-5" style={{ fontSize: '0.58rem' }}>
                  {selectedConnection.article.source} · {categoryLabel(selectedConnection.article.category)}
                </div>

                <div className="grid grid-cols-2 gap-4 border-y border-rule py-4 mb-4">
                  <div>
                    <div className="label-caps text-faint" style={{ fontSize: '0.55rem' }}>Semantic</div>
                    <div className="font-display text-ink text-2xl">{pct(selectedConnection.breakdown?.semantic_pct)}</div>
                  </div>
                  <div>
                    <div className="label-caps text-faint" style={{ fontSize: '0.55rem' }}>Entities</div>
                    <div className="font-display text-ink text-2xl">{pct(selectedConnection.breakdown?.entity_pct)}</div>
                  </div>
                </div>

                <div className="label-caps text-muted mb-3" style={{ fontSize: '0.58rem', lineHeight: 1.7 }}>
                  Weight {score(selectedConnection.weight)} · Cosine {score(selectedConnection.breakdown?.semantic_similarity)} · Overlap {score(selectedConnection.breakdown?.entity_overlap)}
                </div>

                {selectedConnection.explanation && (
                  <p
                    className="text-muted border-y border-rule py-3 mb-4"
                    style={{ fontSize: '0.75rem', fontStyle: 'italic', lineHeight: 1.55 }}
                  >
                    {selectedConnection.explanation}
                  </p>
                )}

                {!selectedConnection.explanation && selectedConnection.explanation_pending && (
                  <div
                    className="label-caps text-faint border-y border-rule py-3 mb-4"
                    style={{ fontSize: '0.55rem', lineHeight: 1.5 }}
                  >
                    GPT-5-nano explanation loading…
                  </div>
                )}

                {(selectedConnection.breakdown?.shared_entities ?? []).length > 0 && (
                  <div className="flex flex-wrap gap-1.5">
                    {selectedConnection.breakdown.shared_entities.slice(0, 12).map(entity => (
                      <span
                        key={entity}
                        className="label-caps text-ink bg-warm border border-rule px-2 py-1"
                        style={{ fontSize: '0.55rem' }}
                      >
                        {entity}
                      </span>
                    ))}
                  </div>
                )}
              </>
            )}
          </aside>
        </div>
      )}
    </div>
  )
}
