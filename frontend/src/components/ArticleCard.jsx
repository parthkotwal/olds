/**
 * Returns a human-readable relative age string and a decay tier.
 *
 * tier values:
 *   'fresh'  — < 6h   — full opacity, no badge
 *   'recent' — 6–24h  — full opacity, no badge
 *   'aging'  — 24–48h — slightly muted, no badge
 *   'stale'  — > 48h  — noticeably muted, FADING badge
 */
function getAgeInfo(dateStr) {
  if (!dateStr) return { label: '', tier: 'fresh' }
  const ageMs = Date.now() - new Date(dateStr).getTime()
  const ageHours = ageMs / (1000 * 60 * 60)

  let label
  if (ageHours < 1) {
    label = 'Just now'
  } else if (ageHours < 24) {
    label = `${Math.floor(ageHours)}h ago`
  } else if (ageHours < 48) {
    label = 'Yesterday'
  } else {
    const days = Math.floor(ageHours / 24)
    label = `${days}d ago`
  }

  let tier
  if (ageHours < 6)       tier = 'fresh'
  else if (ageHours < 24) tier = 'recent'
  else if (ageHours < 48) tier = 'aging'
  else                    tier = 'stale'

  return { label, tier }
}

// Opacity by decay tier — subtle, never invisible.
const TIER_OPACITY = { fresh: 1, recent: 1, aging: 0.82, stale: 0.65 }

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
  const { label: ageLabel, tier } = getAgeInfo(article.published_at)
  const opacity = TIER_OPACITY[tier]

  return (
    // padding and border-bottom are applied by .article-grid > article in index.css
    // Column rules (border-right) are added by the nth-child rules there too.
    // Opacity reflects the backend's decay score — aging articles visually recede.
    <article style={{ opacity, transition: 'opacity 0.2s' }}>
      {/* Category label + FADING badge for stale articles */}
      <div className="label-caps text-muted mb-2.5 flex items-center gap-2">
        {article.category}
        {tier === 'stale' && (
          <span style={{
            fontSize: '0.6rem',
            letterSpacing: '0.08em',
            padding: '1px 5px',
            border: '1px solid currentColor',
            opacity: 0.5,
          }}>
            FADING
          </span>
        )}
      </div>

      {/* Headline — smaller than lead, but still the dominant element in the card */}
      <h3
        className="font-display font-bold text-ink leading-snug headline-link mb-3"
        style={{ fontSize: 'clamp(1.1rem, 2vw, 1.375rem)' }}
        onClick={onClick}
      >
        {article.title}
      </h3>

      {/* Dateline — relative age replaces the static date for recency awareness */}
      <div className="label-caps text-muted mb-3">
        {article.source}
        {ageLabel && <> · {ageLabel}</>}
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
