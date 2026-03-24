import ConnectionSidebar from './ConnectionSidebar'
import { useBehaviorTracking } from '../hooks/useBehaviorTracking'

function formatDate(dateStr) {
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

// ArticleView renders the article reading experience.
//
// Layout:
//   [← Back to feed]
//   ┌──────────────────────────────────────┬────────────┐
//   │  Article column (~680px max-width)   │  Sidebar   │
//   │  category · headline · dateline      │ CONNECTIONS│
//   │  body text · read original →         │  (Phase 8) │
//   └──────────────────────────────────────┴────────────┘
//
// The sidebar is separated by a thin vertical rule and feels like
// marginalia — it's intentionally narrow and quiet. Phase 8 replaces
// the placeholder with real WebSocket-driven connections.
//
// Props:
//   article  Article  — the article to display
//   token    string   — Supabase JWT, forwarded to behavior tracking
//   onBack   fn       — called when "Back to feed" is clicked
export default function ArticleView({ article, token, onBack, onArticleClick }) {
  // Track reading signals — dwell time, scroll depth, re-opens.
  // This hook fires immediately on mount (reopen signal) and sends
  // dwell + scroll_depth to the backend when the component unmounts.
  // The token is passed through so the backend can key signals to the user.
  useBehaviorTracking(article, token)

  return (
    <div>
      {/* Back navigation */}
      <button
        onClick={onBack}
        className="label-caps text-muted hover:text-ink transition-colors duration-150 mb-8 inline-block"
      >
        ← Back to feed
      </button>

      {/* Two-column layout: article + connection sidebar */}
      <div className="flex items-start gap-0 lg:gap-12">

        {/* ── Article column ──────────────────────────────────────────────── */}
        <article className="flex-1 min-w-0 max-w-article">

          {/* Category */}
          <div className="label-caps text-accent mb-4">
            <span className="mr-1.5">●</span>
            {article.category}
          </div>

          {/* Headline */}
          <h1
            className="font-display font-black text-ink leading-tight mb-6"
            style={{ fontSize: 'clamp(1.75rem, 4vw, 3rem)' }}
          >
            {article.title}
          </h1>

          {/* Dateline */}
          <div className="label-caps text-muted mb-6">
            {article.source}
            {article.published_at && (
              <> · {formatDate(article.published_at)}</>
            )}
          </div>

          {/* Lead image — full width of the article column, below the dateline */}
          {article.image_url && (
            <div className="mb-7">
              <img
                src={article.image_url}
                alt={article.title}
                className="w-full"
                style={{ maxHeight: '480px', objectFit: 'cover' }}
              />
            </div>
          )}

          {/* Section rule */}
          <div className="border-t border-rule mb-7" />

          {/* Body copy.
              Priority: raw_text (Guardian full article, plain text after HTML strip)
                        > description (NewsAPI editorial summary, always short)
                        > fallback message.
              Guardian articles have several paragraphs of prose in raw_text.
              NewsAPI articles will show the description until a paid key is used. */}
          {(article.raw_text || article.description) ? (
            <div className="text-ink text-base leading-loose mb-10 space-y-4">
              {(article.raw_text || article.description)
                .split('\n')
                .filter(p => p.trim())
                .map((paragraph, i) => (
                  <p key={i}>{paragraph}</p>
                ))}
            </div>
          ) : (
            <p className="text-muted text-sm mb-10 italic">
              No preview available.
            </p>
          )}

          {/* External link to original article */}
          {article.url && (
            <a
              href={article.url}
              target="_blank"
              rel="noopener noreferrer"
              className="label-caps text-muted hover:text-accent transition-colors duration-150"
            >
              Read original →
            </a>
          )}

          {/* Entity tags — shown only if the ML service has run on this article */}
          {article.entities && article.entities.length > 0 && (
            <div className="mt-10 pt-6 border-t border-rule">
              <div className="label-caps text-muted mb-3">Entities detected</div>
              <div className="flex flex-wrap gap-2">
                {article.entities.map((entity, i) => (
                  <span
                    key={i}
                    className="label-caps text-muted border border-rule px-2 py-0.5"
                    title={entity.label}
                  >
                    {entity.text}
                  </span>
                ))}
              </div>
            </div>
          )}
        </article>

        {/* ── Connection sidebar (desktop) ─────────────────────────────────── */}
        {/* Hidden on mobile — shown as a bottom section there instead.
            On desktop: narrow column separated by a thin vertical rule.
            The aside is itself the sticky + scroll container so it tracks the
            viewport independently from the article column. */}
        <aside
          className="hidden lg:block w-56 flex-shrink-0 pl-8 border-l border-rule sidebar-scroll"
          style={{
            position: 'sticky',
            top: '6rem',
            maxHeight: 'calc(100vh - 7rem)',
            overflowY: 'auto',
            // Hide the scrollbar visually — still scrollable via trackpad/mouse
            scrollbarWidth: 'none',
          }}
        >
          <ConnectionSidebar
            articleId={article.id}
            onArticleClick={onArticleClick}
          />
        </aside>

        {/* ── Connection sidebar (mobile) ──────────────────────────────────── */}
        <div className="lg:hidden mt-10 pt-6 border-t border-rule w-full">
          <ConnectionSidebar
            articleId={article.id}
            onArticleClick={onArticleClick}
          />
        </div>
      </div>
    </div>
  )
}
