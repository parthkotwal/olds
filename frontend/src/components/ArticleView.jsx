import { useState, useEffect } from 'react'
import ConnectionSidebar from './ConnectionSidebar'
import { useBehaviorTracking } from '../hooks/useBehaviorTracking'
import { fetchArticleById } from '../api/articles.js'

function formatDate(dateStr) {
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

export default function ArticleView({ article, token, onBack, onArticleClick }) {
  // article prop is the slim summary from the feed. Fetch full detail for
  // raw_text and entities on mount.
  const [fullArticle, setFullArticle] = useState(null)

  useEffect(() => {
    let cancelled = false
    fetchArticleById(article.id)
      .then(data => { if (!cancelled) setFullArticle(data) })
      .catch(() => { /* Fall back to the summary data we already have */ })
    return () => { cancelled = true }
  }, [article.id])

  // Use full article data when available, fall back to summary.
  const display = fullArticle ?? article

  useBehaviorTracking(display, token)

  return (
    <div>
      <button
        onClick={onBack}
        className="label-caps text-muted hover:text-ink transition-colors duration-150 mb-8 inline-block"
      >
        ← Back to feed
      </button>

      <div className="flex items-start gap-0 lg:gap-12">
        <article className="flex-1 min-w-0 max-w-article">
          <div className="label-caps text-accent mb-4">
            <span className="mr-1.5">●</span>
            {display.category}
          </div>

          <h1
            className="font-display font-black text-ink leading-tight mb-6"
            style={{ fontSize: 'clamp(1.75rem, 4vw, 3rem)' }}
          >
            {display.title}
          </h1>

          <div className="label-caps text-muted mb-6">
            {display.source}
            {display.published_at && (
              <> · {formatDate(display.published_at)}</>
            )}
          </div>

          {display.image_url && (
            <div className="mb-7">
              <img
                src={display.image_url}
                alt={display.title}
                className="w-full"
                style={{ maxHeight: '480px', objectFit: 'cover' }}
              />
            </div>
          )}

          <div className="border-t border-rule mb-7" />

          {(display.raw_text || display.description) ? (
            <div className="text-ink text-base leading-loose mb-10 space-y-4">
              {(display.raw_text || display.description)
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

          {display.url && (
            <a
              href={display.url}
              target="_blank"
              rel="noopener noreferrer"
              className="label-caps text-muted hover:text-accent transition-colors duration-150"
            >
              Read original →
            </a>
          )}

          {display.entities && display.entities.length > 0 && (
            <div className="mt-10 pt-6 border-t border-rule">
              <div className="label-caps text-muted mb-3">Entities detected</div>
              <div className="flex flex-wrap gap-2">
                {display.entities.map((entity, i) => (
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

        <aside
          className="hidden lg:block w-56 flex-shrink-0 pl-8 border-l border-rule sidebar-scroll"
          style={{
            position: 'sticky',
            top: '6rem',
            maxHeight: 'calc(100vh - 7rem)',
            overflowY: 'auto',
            scrollbarWidth: 'none',
          }}
        >
          <ConnectionSidebar
            articleId={article.id}
            onArticleClick={onArticleClick}
          />
        </aside>

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
