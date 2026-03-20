function formatDate(dateStr) {
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

// ArticleCard renders a secondary article in the columnar grid below the lead story.
//
// Sizing is deliberately smaller than LeadStory — the type hierarchy guides
// the eye from the dominant headline down through the grid. Stories are
// separated by thin column rules (via the .article-grid CSS class on the
// parent grid) rather than card shadows or background fills.
//
// Props:
//   article  Article  — article object from the Go backend
//   onClick  fn       — called when the user clicks the headline
export default function ArticleCard({ article, onClick }) {
  return (
    // padding and border-bottom are applied by .article-grid > article in index.css
    // Column rules (border-right) are added by the nth-child rules there too.
    <article>
      {/* Category label */}
      <div className="label-caps text-muted mb-2.5">
        {article.category}
      </div>

      {/* Headline — smaller than lead, but still the dominant element in the card */}
      <h3
        className="font-display font-bold text-ink leading-snug headline-link mb-3"
        style={{ fontSize: 'clamp(1.1rem, 2vw, 1.375rem)' }}
        onClick={onClick}
      >
        {article.title}
      </h3>

      {/* Dateline */}
      <div className="label-caps text-muted mb-3">
        {article.source}
        {article.published_at && (
          <> · {formatDate(article.published_at)}</>
        )}
      </div>

      {/* Thumbnail — shown only when available; sits above the description.
          Kept small (max 160px tall) so it doesn't overwhelm the card. */}
      {article.image_url && (
        <div className="mb-3">
          <img
            src={article.image_url}
            alt={article.title}
            className="w-full"
            style={{ maxHeight: '160px', objectFit: 'cover' }}
          />
        </div>
      )}

      {/* Description — clamped to 3 lines so the grid stays visually even */}
      {article.description && (
        <p
          className="text-ink text-sm leading-relaxed"
          style={{
            display: '-webkit-box',
            WebkitLineClamp: 3,
            WebkitBoxOrient: 'vertical',
            overflow: 'hidden',
          }}
        >
          {article.description}
        </p>
      )}
    </article>
  )
}
