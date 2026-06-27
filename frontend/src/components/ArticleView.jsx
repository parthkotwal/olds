import { useState, useEffect } from 'react'
import ConnectionSidebar from './ConnectionSidebar'
import ConnectionGraphView from './ConnectionGraphView'
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
  const [fullArticle, setFullArticle] = useState(null)
  const [activeTab, setActiveTab] = useState('source')

  useEffect(() => {
    let cancelled = false
    fetchArticleById(article.id)
      .then(data => { if (!cancelled) setFullArticle(data) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [article.id])

  const display = fullArticle ?? article

  useBehaviorTracking(display, token)

  return (
    <div>
      <div className="flex items-center gap-2 mb-6">
        <button
          onClick={onBack}
          className="label-caps text-muted hover:text-ink transition-colors duration-150"
        >
          Top Stories
        </button>
        <span className="text-muted text-xs">/</span>
        <span className="label-caps text-muted">{display.category}</span>
      </div>

      <div className="border-y border-rule mb-7">
        <div className="flex items-stretch">
          <button
            onClick={() => setActiveTab('source')}
            className={[
              'label-caps px-4 py-3 border-r border-rule transition-colors',
              activeTab === 'source'
                ? 'bg-ink text-paper'
                : 'text-muted hover:text-ink',
            ].join(' ')}
            style={{ fontSize: '0.65rem' }}
          >
            Source Article
          </button>
          <button
            onClick={() => setActiveTab('graph')}
            className={[
              'label-caps px-4 py-3 transition-colors',
              activeTab === 'graph'
                ? 'bg-ink text-paper'
                : 'text-muted hover:text-ink',
            ].join(' ')}
            style={{ fontSize: '0.65rem' }}
          >
            Connection Graph
          </button>
        </div>
      </div>

      {activeTab === 'graph' && (
        <ConnectionGraphView
          article={display}
          onShowSource={() => setActiveTab('source')}
          onArticleClick={onArticleClick}
        />
      )}

      {activeTab === 'source' && (
        <div className="flex items-start gap-0 lg:gap-10">
          <article className="flex-1 min-w-0 max-w-article">
            <h1
              className="font-display font-normal text-ink leading-tight headline-tight mb-4"
              style={{ fontSize: 'clamp(2rem, 4vw, 3rem)' }}
            >
              {display.title}
            </h1>

            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mb-6 pb-4 border-b border-rule">
              <span className="label-caps text-muted" style={{ fontSize: '0.65rem' }}>{display.source}</span>
              {display.published_at && (
                <>
                  <span className="text-muted text-xs">·</span>
                  <span className="label-caps text-muted" style={{ fontSize: '0.65rem' }}>{formatDate(display.published_at)}</span>
                </>
              )}
              {display.url && (
                <>
                  <span className="text-muted text-xs">·</span>
                  <a
                    href={display.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="label-caps text-ink hover:underline"
                    style={{ fontSize: '0.65rem', textDecorationColor: 'var(--color-accent)', textUnderlineOffset: '3px' }}
                  >
                    Original
                  </a>
                </>
              )}
            </div>

            {display.image_url && (
              <div className="mb-8 -mx-5 sm:mx-0 overflow-hidden border-y border-rule" style={{ background: 'var(--color-rule)' }}>
                <img
                  src={display.image_url}
                  alt=""
                  className="w-full"
                  style={{ maxHeight: '420px', objectFit: 'cover' }}
                />
              </div>
            )}

            {(display.raw_text || display.description) ? (
              <div className="font-display text-ink text-[1.0625rem] leading-[1.72] mb-10 space-y-4">
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

            {display.entities && display.entities.length > 0 && (
              <div className="pt-5 border-t border-rule">
                <div className="editorial-label text-ink mb-3">
                  Key entities
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {display.entities.map((entity, i) => (
                    <span
                      key={i}
                      className="label-caps text-muted border border-rule px-2 py-1"
                      style={{ fontSize: '0.58rem' }}
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
            className="hidden lg:block w-60 flex-shrink-0 pl-8 border-l border-rule sidebar-scroll"
            style={{
              position: 'sticky',
              top: '4rem',
              maxHeight: 'calc(100vh - 5rem)',
              overflowY: 'auto',
              scrollbarWidth: 'none',
            }}
          >
            <ConnectionSidebar
              articleId={article.id}
              onArticleClick={onArticleClick}
              onOpenGraph={() => setActiveTab('graph')}
            />
          </aside>

          <div className="lg:hidden mt-8 pt-6 border-t border-rule w-full">
            <ConnectionSidebar
              articleId={article.id}
              onArticleClick={onArticleClick}
              onOpenGraph={() => setActiveTab('graph')}
            />
          </div>
        </div>
      )}
    </div>
  )
}
