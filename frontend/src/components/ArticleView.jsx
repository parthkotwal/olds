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
//   onBack   fn       — called when "Back to feed" is clicked
export default function ArticleView({ article, onBack }) {
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

        {/* ── Connection sidebar ──────────────────────────────────────────── */}
        {/* Hidden on mobile — shown as a bottom section there instead.
            On desktop: narrow column with thin vertical rule separator. */}
        <aside className="hidden lg:block w-56 flex-shrink-0 pl-8 border-l border-rule self-stretch">
          <div className="sticky top-24">
            <div className="label-caps text-muted mb-5">Connections</div>
            <p className="text-muted text-xs leading-relaxed mb-4">
              As you read, related stories across different topics surface here.
            </p>
            <p className="text-muted text-xs leading-relaxed italic">
              Real-time graph traversal via WebSocket — Phase 8.
            </p>
          </div>
        </aside>

        {/* Mobile-only connections note — below article */}
        <div className="lg:hidden mt-10 pt-6 border-t border-rule w-full">
          <div className="label-caps text-muted mb-3">Connections</div>
          <p className="text-muted text-xs leading-relaxed italic">
            Cross-topic connections will appear here in real time — Phase 8.
          </p>
        </div>
      </div>
    </div>
  )
}
