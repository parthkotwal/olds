function relativeAge(dateStr) {
  if (!dateStr) return ''
  const hours = (Date.now() - new Date(dateStr).getTime()) / 3600000
  if (hours < 1) return 'Just now'
  if (hours < 24) return `${Math.floor(hours)}h ago`
  if (hours < 48) return 'Yesterday'
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function decayOpacity(dateStr) {
  if (!dateStr) return 1
  const hours = (Date.now() - new Date(dateStr).getTime()) / 3600000
  if (hours < 24) return 1
  if (hours < 48) return 0.85
  return 0.65
}

export function ArticleCardHorizontal({ article, onClick }) {
  return (
    <article
      className="py-4 border-b border-rule cursor-pointer group"
      style={{ opacity: decayOpacity(article.published_at) }}
      onClick={onClick}
    >
      <div className="flex gap-4">
        <div className="flex-1 min-w-0">
          <div className="editorial-label text-ink mb-1">
            {article.category}
          </div>
          <h3
            className="font-display font-normal text-ink leading-snug headline-tight group-hover:underline mb-1.5"
            style={{
              fontSize: '1rem',
              textDecorationColor: 'var(--color-ink)',
              textUnderlineOffset: '3px',
              textDecorationThickness: '1px',
            }}
          >
            {article.title}
          </h3>
          <div className="label-caps text-faint" style={{ fontSize: '0.55rem' }}>
            {article.source} · {relativeAge(article.published_at)}
          </div>
        </div>
        {article.image_url && (
          <div
            className="flex-shrink-0 w-24 h-16 sm:w-28 sm:h-[4.5rem] overflow-hidden"
            style={{ background: 'var(--color-rule)' }}
          >
            <img
              src={article.image_url}
              alt=""
              loading="lazy"
              className="w-full h-full object-cover"
            />
          </div>
        )}
      </div>
    </article>
  )
}

export function ArticleCardDense({ article, onClick }) {
  return (
    <article
      className="py-3 border-b border-rule cursor-pointer group"
      style={{ opacity: decayOpacity(article.published_at) }}
      onClick={onClick}
    >
      <h3
        className="font-display font-normal text-ink leading-snug headline-tight group-hover:underline mb-1"
        style={{
          fontSize: '0.95rem',
          textDecorationColor: 'var(--color-ink)',
          textUnderlineOffset: '3px',
          textDecorationThickness: '1px',
        }}
      >
        {article.title}
      </h3>
      <div className="label-caps text-faint" style={{ fontSize: '0.55rem' }}>
        {article.source} · {relativeAge(article.published_at)}
      </div>
    </article>
  )
}

export default function ArticleCard({ article, onClick }) {
  return <ArticleCardHorizontal article={article} onClick={onClick} />
}
