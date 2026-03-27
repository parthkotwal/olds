// formatDate converts an ISO 8601 date string to "March 19, 2026"
function formatDate(dateStr) {
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

// LeadStory renders the dominant front-page article — full width, large headline.
//
// This is the first article in the feed. Its headline is 3–4× larger than the
// article grid below, establishing clear visual hierarchy. In newspaper terms,
// it's the "above the fold" lead story.
//
// Props:
//   article  Article  — the lead article object from the Go backend
//   onClick  fn       — called when the user clicks the headline or "Read more"
export default function LeadStory({ article, onClick }) {
  return (
    <div className="pb-8 border-b border-rule">
      {/* Category label — small-caps, accent-colored dot prefix */}
      <div className="label-caps text-muted mb-3">
        <span className="text-accent mr-1.5">●</span>
        {article.category}
      </div>

      {/* Dominant headline — the largest type on the page */}
      <h2
        className="font-display font-black text-ink leading-tight headline-link mb-4"
        style={{ fontSize: 'clamp(2rem, 5vw, 3.75rem)' }}
        onClick={onClick}
      >
        {article.title}
      </h2>

      {/* Dateline: Source · Date — the classic newspaper attribution line */}
      <div className="label-caps text-muted mb-5">
        {article.source}
        {article.published_at && (
          <> · {formatDate(article.published_at)}</>
        )}
      </div>

      {/* Lead image — fetchpriority="high" tells the browser this is the LCP
          element and to start downloading it as early as possible.
          The aspect-ratio container reserves space so the headline and dateline
          above don't jump when the image loads. */}
      {article.image_url && (
        <div style={{ aspectRatio: '16/9', overflow: 'hidden', marginBottom: '1.5rem', background: 'var(--color-rule)', maxWidth: '42rem', margin: '0 auto 1.5rem' }}>
          <img
            src={article.image_url}
            alt={article.title}
            fetchPriority="high"
            loading="eager"
            style={{ width: '100%', height: '100%', objectFit: 'cover' }}
          />
        </div>
      )}

      {/* Deck copy — the lede beneath the headline */}
      {article.description && (
        <p className="text-ink text-lg leading-relaxed max-w-3xl mx-auto mb-6 text-center">
          {article.description}
        </p>
      )}

      <button
        onClick={onClick}
        className="label-caps text-muted hover:text-accent transition-colors duration-150"
      >
        Read more →
      </button>
    </div>
  )
}
