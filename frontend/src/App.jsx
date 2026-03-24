import { useState, useEffect, useRef } from 'react'
import { supabase } from './lib/supabase'
import Header from './components/Header.jsx'
import CategoryFilter from './components/CategoryFilter.jsx'
import LeadStory from './components/LeadStory.jsx'
import ArticleCard from './components/ArticleCard.jsx'
import ArticleView from './components/ArticleView.jsx'
import LoginModal from './components/LoginModal.jsx'
import { fetchArticles } from './api/articles.js'

// App is the root state machine for the whole UI.
//
// Auth model (Phase 13 change):
//   The feed is public — anyone can browse headlines without signing in.
//   Signing in is only required to read an article (which enables behavior
//   tracking, personalized ranking, and connection history).
//
//   When an unauthenticated user clicks an article:
//     1. The article is saved in pendingArticleRef
//     2. The LoginModal appears
//     3. After sign-in, onAuthStateChange fires, finds the pending article,
//        and opens it automatically — the user lands exactly where they intended.
export default function App() {
  // ── Auth state ─────────────────────────────────────────────────────────────
  const [session, setSession] = useState(null)
  // authChecked: true once getSession() has resolved. Prevents the Header from
  // flickering between "Sign in" and the email before auth is confirmed.
  const [authChecked, setAuthChecked] = useState(false)
  const [loginModalOpen, setLoginModalOpen] = useState(false)

  // pendingArticleRef stores the article the user tried to open before login.
  // A ref (not state) because we read it inside the onAuthStateChange callback —
  // using state here would require it in the effect's dependency array, which
  // would re-register the subscription on every click. A ref avoids that.
  const pendingArticleRef = useRef(null)

  // ── Feed state ─────────────────────────────────────────────────────────────
  const [articles, setArticles] = useState([])
  const [selectedCategory, setSelectedCategory] = useState('all')
  const [selectedArticle, setSelectedArticle] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  // ── Auth effect ────────────────────────────────────────────────────────────
  useEffect(() => {
    // Resolve the current session from localStorage (instant) or from the URL
    // hash after an OAuth / magic-link redirect.
    supabase.auth.getSession().then(({ data: { session } }) => {
      setSession(session)
      setAuthChecked(true)
    })

    const { data: { subscription } } = supabase.auth.onAuthStateChange(
      (_event, session) => {
        setSession(session)

        if (session) {
          // User just signed in. If there's a pending article (they clicked
          // before logging in), open it now and clear the pending state.
          if (pendingArticleRef.current) {
            setSelectedArticle(pendingArticleRef.current)
            pendingArticleRef.current = null
            window.scrollTo({ top: 0, behavior: 'instant' })
          }
          setLoginModalOpen(false)
        } else {
          // User signed out — return to the feed.
          setSelectedArticle(null)
        }
      }
    )

    return () => subscription.unsubscribe()
  }, []) // empty deps — the callback captures pendingArticleRef, which is stable

  async function handleSignOut() {
    await supabase.auth.signOut()
    // onAuthStateChange will fire with null session → setSelectedArticle(null)
  }

  // ── Feed effect ────────────────────────────────────────────────────────────
  useEffect(() => {
    let cancelled = false

    async function load() {
      setLoading(true)
      setError(null)
      try {
        const data = await fetchArticles(selectedCategory === 'all' ? '' : selectedCategory)
        if (!cancelled) setArticles(data)
      } catch (err) {
        if (!cancelled) setError(err.message)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    load()
    return () => { cancelled = true }
  }, [selectedCategory])

  // ── Handlers ───────────────────────────────────────────────────────────────
  function handleCategorySelect(category) {
    setSelectedCategory(category)
    setSelectedArticle(null)
  }

  function handleArticleClick(article) {
    if (!session) {
      // Save where they were trying to go, then prompt login.
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

  // ── Render ─────────────────────────────────────────────────────────────────
  return (
    <div className="min-h-screen bg-paper">
      <Header
        // Three distinct states for the auth strip in the Header:
        //   undefined → auth still resolving, show nothing (no flicker)
        //   null      → auth resolved, no session → show "Sign in"
        //   string    → signed in → show email + "Sign out"
        // `?? null` converts the `undefined` from optional chaining into null.
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
            error={error}
            onArticleClick={handleArticleClick}
          />
        )}
      </main>

      {/* Login modal — rendered at the root so it sits above everything */}
      {loginModalOpen && (
        <LoginModal onClose={() => {
          setLoginModalOpen(false)
          pendingArticleRef.current = null
        }} />
      )}
    </div>
  )
}

// FeedView is the public landing experience — visible to all users.
// Article cards fade in with a gentle stagger (50ms per card, up to 600ms total)
// so the feed populates gracefully rather than appearing all at once.
function FeedView({ articles, loading, error, onArticleClick }) {
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
      {/* Lead story fades in first */}
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
    </>
  )
}

// FeedSkeleton renders placeholder blocks that match the feed's rough layout.
// This prevents the jarring "blank → everything at once" pop that plain text
// loading states cause. The pulse animation signals "content is loading"
// without being distracting.
function FeedSkeleton() {
  return (
    <div style={{ animation: 'articleFadeIn 300ms ease-out both' }}>
      {/* Lead story skeleton */}
      <div className="pb-8 border-b border-rule">
        <div style={{ ...skeletonStyle, width: '5rem', height: '0.6rem', marginBottom: '0.75rem' }} />
        <div style={{ ...skeletonStyle, width: '75%', height: '2.5rem', marginBottom: '0.5rem' }} />
        <div style={{ ...skeletonStyle, width: '55%', height: '2.5rem', marginBottom: '1rem' }} />
        <div style={{ ...skeletonStyle, width: '10rem', height: '0.6rem', marginBottom: '1.5rem' }} />
        <div style={{ ...skeletonStyle, width: '100%', maxWidth: '32rem', height: '12rem', marginBottom: '1.5rem' }} />
        <div style={{ ...skeletonStyle, width: '90%', height: '1rem', marginBottom: '0.5rem' }} />
        <div style={{ ...skeletonStyle, width: '80%', height: '1rem' }} />
      </div>

      {/* Grid skeleton */}
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

// Shared style for skeleton placeholder blocks.
// The pulse animation is defined in index.css.
const skeletonStyle = {
  background: 'var(--color-rule)',
  animation: 'pulse 1.6s ease-in-out infinite',
}
