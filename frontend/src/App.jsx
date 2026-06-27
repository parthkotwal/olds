import { useState, useEffect, useRef, useCallback } from 'react'
import { supabase } from './lib/supabase'
import Header from './components/Header.jsx'
import CategoryFilter from './components/CategoryFilter.jsx'
import LeadStory from './components/LeadStory.jsx'
import ArticleCard from './components/ArticleCard.jsx'
import ArticleView from './components/ArticleView.jsx'
import LoginModal from './components/LoginModal.jsx'
import { fetchArticles } from './api/articles.js'

const PAGE_SIZE = 30

export default function App() {
  const [session, setSession] = useState(null)
  const [authChecked, setAuthChecked] = useState(false)
  const [loginModalOpen, setLoginModalOpen] = useState(false)

  const pendingArticleRef = useRef(null)

  const [articles, setArticles] = useState([])
  const [selectedCategory, setSelectedCategory] = useState('all')
  const [selectedArticle, setSelectedArticle] = useState(null)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState(null)
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)

  useEffect(() => {
    supabase.auth.getSession().then(({ data: { session } }) => {
      setSession(session)
      setAuthChecked(true)
    })

    const { data: { subscription } } = supabase.auth.onAuthStateChange(
      (_event, session) => {
        setSession(session)

        if (session) {
          if (pendingArticleRef.current) {
            setSelectedArticle(pendingArticleRef.current)
            pendingArticleRef.current = null
            window.scrollTo({ top: 0, behavior: 'instant' })
          }
          setLoginModalOpen(false)
        } else {
          setSelectedArticle(null)
        }
      }
    )

    return () => subscription.unsubscribe()
  }, [])

  async function handleSignOut() {
    await supabase.auth.signOut()
  }

  // Initial load and category change
  useEffect(() => {
    let cancelled = false

    async function load() {
      setLoading(true)
      setError(null)
      setPage(1)
      setArticles([])
      try {
        const category = selectedCategory === 'all' ? '' : selectedCategory
        const data = await fetchArticles({ category, page: 1, pageSize: PAGE_SIZE })
        if (!cancelled) {
          setArticles(data.articles)
          setTotal(data.total)
          setPage(1)
        }
      } catch (err) {
        if (!cancelled) setError(err.message)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    load()
    return () => { cancelled = true }
  }, [selectedCategory])

  const hasMore = articles.length < total

  const loadMore = useCallback(async () => {
    if (loadingMore || !hasMore) return
    setLoadingMore(true)
    try {
      const nextPage = page + 1
      const category = selectedCategory === 'all' ? '' : selectedCategory
      const data = await fetchArticles({ category, page: nextPage, pageSize: PAGE_SIZE })
      setArticles(prev => [...prev, ...data.articles])
      setPage(nextPage)
    } catch (err) {
      // Silently fail — user can retry
    } finally {
      setLoadingMore(false)
    }
  }, [page, selectedCategory, loadingMore, hasMore])

  function handleCategorySelect(category) {
    setSelectedCategory(category)
    setSelectedArticle(null)
  }

  function handleArticleClick(article) {
    if (!session) {
      pendingArticleRef.current = article
      setLoginModalOpen(true)
      return
    }
    setSelectedArticle(article)
    window.scrollTo({ top: 0, behavior: 'instant' })
  }

  function handleBackToFeed() {
    setSelectedArticle(null)
  }

  return (
    <div className="min-h-screen bg-paper">
      <Header
        userEmail={authChecked ? (session?.user?.email ?? null) : undefined}
        onSignOut={session ? handleSignOut : null}
        onSignIn={() => setLoginModalOpen(true)}
      />
      <CategoryFilter selected={selectedCategory} onSelect={handleCategorySelect} />

      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-10">
        {selectedArticle ? (
          <ArticleView
            article={selectedArticle}
            token={session?.access_token}
            onBack={handleBackToFeed}
            onArticleClick={handleArticleClick}
          />
        ) : (
          <FeedView
            articles={articles}
            loading={loading}
            loadingMore={loadingMore}
            hasMore={hasMore}
            error={error}
            onArticleClick={handleArticleClick}
            onLoadMore={loadMore}
          />
        )}
      </main>

      {loginModalOpen && (
        <LoginModal onClose={() => {
          setLoginModalOpen(false)
          pendingArticleRef.current = null
        }} />
      )}
    </div>
  )
}

function FeedView({ articles, loading, loadingMore, hasMore, error, onArticleClick, onLoadMore }) {
  if (loading) {
    return <FeedSkeleton />
  }

  if (error) {
    return (
      <p className="text-muted text-sm py-12">
        Could not load articles — {error}. Make sure the backend is running and POST /ingest has been called.
      </p>
    )
  }

  if (articles.length === 0) {
    return (
      <p className="text-muted text-sm py-12" style={{ fontStyle: 'italic' }}>
        No articles yet — the feed is loading. Check back in a moment.
      </p>
    )
  }

  const [lead, ...rest] = articles

  return (
    <>
      <div style={{ animation: 'articleFadeIn 400ms ease-out both' }}>
        <LeadStory article={lead} onClick={() => onArticleClick(lead)} />
      </div>

      {rest.length > 0 && (
        <section className="mt-0 border-t border-rule">
          <div className="article-grid grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
            {rest.map((article) => (
              <ArticleCard
                key={article.id}
                article={article}
                onClick={() => onArticleClick(article)}
              />
            ))}
          </div>
        </section>
      )}

      {hasMore && (
        <div className="flex justify-center py-10">
          <button
            onClick={onLoadMore}
            disabled={loadingMore}
            className="label-caps text-muted hover:text-ink transition-colors duration-150 px-6 py-2 border border-rule"
            style={{ letterSpacing: '0.1em', fontSize: '0.7rem' }}
          >
            {loadingMore ? 'Loading…' : 'More stories'}
          </button>
        </div>
      )}
    </>
  )
}

function FeedSkeleton() {
  return (
    <div style={{ animation: 'articleFadeIn 300ms ease-out both' }}>
      <div className="pb-8 border-b border-rule">
        <div style={{ ...skeletonStyle, width: '5rem', height: '0.6rem', marginBottom: '0.75rem' }} />
        <div style={{ ...skeletonStyle, width: '75%', height: '2.5rem', marginBottom: '0.5rem' }} />
        <div style={{ ...skeletonStyle, width: '55%', height: '2.5rem', marginBottom: '1rem' }} />
        <div style={{ ...skeletonStyle, width: '10rem', height: '0.6rem', marginBottom: '1.5rem' }} />
        <div style={{ ...skeletonStyle, width: '100%', maxWidth: '32rem', height: '12rem', marginBottom: '1.5rem' }} />
        <div style={{ ...skeletonStyle, width: '90%', height: '1rem', marginBottom: '0.5rem' }} />
        <div style={{ ...skeletonStyle, width: '80%', height: '1rem' }} />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 mt-0 border-t border-rule">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} style={{ padding: '1.5rem', borderBottom: '1px solid var(--color-rule)' }}>
            <div style={{ ...skeletonStyle, width: '4rem', height: '0.55rem', marginBottom: '0.75rem' }} />
            <div style={{ ...skeletonStyle, width: '90%', height: '1rem', marginBottom: '0.4rem' }} />
            <div style={{ ...skeletonStyle, width: '70%', height: '1rem', marginBottom: '0.75rem' }} />
            <div style={{ ...skeletonStyle, width: '8rem', height: '0.55rem' }} />
          </div>
        ))}
      </div>
    </div>
  )
}

const skeletonStyle = {
  background: 'var(--color-rule)',
  animation: 'pulse 1.6s ease-in-out infinite',
}
