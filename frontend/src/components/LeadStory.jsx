function relativeAge(dateStr) {
  if (!dateStr) return ''
  const hours = (Date.now() - new Date(dateStr).getTime()) / 3600000
  if (hours < 1) return 'Just now'
  if (hours < 24) return `${Math.floor(hours)}h ago`
  if (hours < 48) return 'Yesterday'
  return new Date(dateStr).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

export default function HeroSection({ articles, onArticleClick }) {
  if (!articles || articles.length === 0) return null

  const lead = articles[0]
  const sides = articles.slice(1, 3)

  return (
    <section className="pb-6 mb-2">
      <div className="flex flex-col lg:flex-row gap-0">
        <div className="lg:flex-[3] lg:pr-8 lg:border-r lg:border-rule pb-6 lg:pb-0">
          {lead.image_url && (
            <div
              className="mb-4 overflow-hidden"
              style={{ aspectRatio: '3/2', background: 'var(--color-rule)' }}
            >
              <img
                src={lead.image_url}
                alt=""
                fetchPriority="high"
                className="w-full h-full object-cover"
              />
            </div>
          )}
          <div className="editorial-label text-ink mb-2">
            {lead.category}
          </div>
          <h2
            className="font-display font-normal text-ink leading-tight headline-link headline-tight mb-3"
            style={{ fontSize: 'clamp(1.9rem, 4vw, 2.85rem)' }}
            onClick={() => onArticleClick(lead)}
          >
            {lead.title}
          </h2>
          {lead.description && (
            <p className="font-display text-muted text-base leading-relaxed mb-3 max-w-xl">
              {lead.description}
            </p>
          )}
          <div className="label-caps text-faint" style={{ fontSize: '0.6rem' }}>
            {lead.source} · {relativeAge(lead.published_at)}
          </div>
        </div>

        {sides.length > 0 && (
          <div className="lg:flex-[2] lg:pl-8 flex flex-col">
            {sides.map((article, i) => (
              <div
                key={article.id}
                className={`py-5 ${i < sides.length - 1 ? 'border-b border-rule' : ''} ${i === 0 ? 'lg:pt-0' : ''}`}
              >
                <div className="flex gap-4">
                  <div className="flex-1 min-w-0">
                    <div className="editorial-label text-ink mb-1.5">
                      {article.category}
                    </div>
                    <h3
                      className="font-display font-normal text-ink leading-snug headline-link headline-tight mb-2"
                      style={{ fontSize: 'clamp(1rem, 1.8vw, 1.25rem)' }}
                      onClick={() => onArticleClick(article)}
                    >
                      {article.title}
                    </h3>
                    {article.description && (
                      <p
                        className="font-display text-muted text-sm leading-relaxed mb-2"
                        style={{
                          display: '-webkit-box',
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: 'vertical',
                          overflow: 'hidden',
                        }}
                      >
                        {article.description}
                      </p>
                    )}
                    <div className="label-caps text-faint" style={{ fontSize: '0.55rem' }}>
                      {article.source} · {relativeAge(article.published_at)}
                    </div>
                  </div>
                  {article.image_url && (
                    <div
                      className="flex-shrink-0 w-28 h-20 overflow-hidden"
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
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}
