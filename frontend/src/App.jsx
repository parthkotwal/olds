import { useState, useEffect, useRef, useCallback } from 'react'
import { supabase } from './lib/supabase'
import Header from './components/Header.jsx'
import CategoryFilter from './components/CategoryFilter.jsx'
import HeroSection from './components/LeadStory.jsx'
import { ArticleCardHorizontal, ArticleCardDense } from './components/ArticleCard.jsx'
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

      <main className="max-w-layout mx-auto px-5 sm:px-8 py-8">
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
  if (loading) return <FeedSkeleton />

  if (error) {
    return (
      <p className="text-muted text-sm py-12">
        Could not load articles — {error}
      </p>
    )
  }

  if (articles.length === 0) {
    return (
      <p className="text-muted text-sm py-12 italic">
        No articles yet. Check back in a moment.
      </p>
    )
  }

  const heroArticles = articles.slice(0, 3)
  const midSection = articles.slice(3, 9)
  const denseSection = articles.slice(9)

  return (
    <div style={{ animation: 'fadeIn 300ms ease-out both' }}>
      <HeroSection articles={heroArticles} onArticleClick={onArticleClick} />

      {midSection.length > 0 && (
        <>
          <div className="border-t border-rule my-7" />
          <div className="editorial-label text-ink mb-4">
            More Stories
          </div>
          <div className="editorial-grid-2 grid grid-cols-1 md:grid-cols-2 gap-x-0">
            {midSection.map(article => (
              <div key={article.id}>
                <ArticleCardHorizontal
                  article={article}
                  onClick={() => onArticleClick(article)}
                />
              </div>
            ))}
          </div>
        </>
      )}

      {denseSection.length > 0 && (
        <>
          <div className="border-t border-rule my-7" />
          <div className="editorial-label text-ink mb-4">
            Latest
          </div>
          <div className="editorial-grid-3 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-0">
            {denseSection.map(article => (
              <div key={article.id}>
                <ArticleCardDense
                  article={article}
                  onClick={() => onArticleClick(article)}
                />
              </div>
            ))}
          </div>
        </>
      )}

      {hasMore && (
        <div className="flex justify-center py-10">
          <button
            onClick={onLoadMore}
            disabled={loadingMore}
            className="label-caps text-ink bg-accent transition-opacity duration-150 hover:opacity-80 px-6 py-2"
            style={{ fontSize: '0.65rem' }}
          >
            {loadingMore ? 'Loading…' : 'More stories'}
          </button>
        </div>
      )}
    </div>
  )
}

function FeedSkeleton() {
  return (
    <div style={{ animation: 'fadeIn 300ms ease-out both' }}>
      <div className="flex flex-col lg:flex-row gap-8 pb-6">
        <div className="lg:flex-[3]">
          <div style={{ ...skel, width: '100%', height: '16rem', marginBottom: '1rem' }} />
          <div style={{ ...skel, width: '4rem', height: '0.5rem', marginBottom: '0.75rem' }} />
          <div style={{ ...skel, width: '80%', height: '1.5rem', marginBottom: '0.5rem' }} />
          <div style={{ ...skel, width: '60%', height: '1.5rem', marginBottom: '1rem' }} />
          <div style={{ ...skel, width: '90%', height: '0.75rem', marginBottom: '0.4rem' }} />
          <div style={{ ...skel, width: '75%', height: '0.75rem' }} />
        </div>
        <div className="lg:flex-[2]">
          {[0, 1].map(i => (
            <div key={i} className="py-5 border-b border-rule">
              <div style={{ ...skel, width: '3rem', height: '0.5rem', marginBottom: '0.5rem' }} />
              <div style={{ ...skel, width: '90%', height: '0.875rem', marginBottom: '0.4rem' }} />
              <div style={{ ...skel, width: '70%', height: '0.875rem', marginBottom: '0.5rem' }} />
              <div style={{ ...skel, width: '6rem', height: '0.5rem' }} />
            </div>
          ))}
        </div>
      </div>
      <div className="border-t border-rule my-6" />
      <div className="grid grid-cols-1 md:grid-cols-2 gap-x-8">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="py-4 border-b border-rule">
            <div style={{ ...skel, width: '3rem', height: '0.45rem', marginBottom: '0.5rem' }} />
            <div style={{ ...skel, width: '85%', height: '0.8rem', marginBottom: '0.35rem' }} />
            <div style={{ ...skel, width: '65%', height: '0.8rem', marginBottom: '0.5rem' }} />
            <div style={{ ...skel, width: '5rem', height: '0.45rem' }} />
          </div>
        ))}
      </div>
    </div>
  )
}

const skel = {
  background: 'var(--color-rule)',
  animation: 'pulse 1.6s ease-in-out infinite',
}
