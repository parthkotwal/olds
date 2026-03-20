import { useState, useEffect } from 'react'
import Header from './components/Header.jsx'
import CategoryFilter from './components/CategoryFilter.jsx'
import LeadStory from './components/LeadStory.jsx'
import ArticleCard from './components/ArticleCard.jsx'
import ArticleView from './components/ArticleView.jsx'
import { fetchArticles } from './api/articles.js'

// App is the state machine for the whole UI.
// Two views: feed (selectedArticle === null) and article (selectedArticle !== null).
// All data lives here and is passed down as props — no global state needed.
export default function App() {
  const [articles, setArticles] = useState([])
  const [selectedCategory, setSelectedCategory] = useState('all')
  const [selectedArticle, setSelectedArticle] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  // Re-fetch whenever the selected category changes.
  // The dependency array [selectedCategory] means this effect runs once on mount
  // and again whenever selectedCategory changes — equivalent to Python:
  //   @app.route + calling it whenever the filter button is clicked.
  useEffect(() => {
    let cancelled = false // prevent state updates if the component unmounts mid-fetch

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

  function handleCategorySelect(category) {
    setSelectedCategory(category)
    setSelectedArticle(null) // return to feed on category change
  }

  function handleArticleClick(article) {
    setSelectedArticle(article)
    window.scrollTo({ top: 0, behavior: 'instant' })
  }

  function handleBackToFeed() {
    setSelectedArticle(null)
  }

  return (
    <div className="min-h-screen bg-paper">
      <Header />
      <CategoryFilter selected={selectedCategory} onSelect={handleCategorySelect} />

      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-10">
        {selectedArticle ? (
          <ArticleView article={selectedArticle} onBack={handleBackToFeed} />
        ) : (
          <FeedView
            articles={articles}
            loading={loading}
            error={error}
            onArticleClick={handleArticleClick}
          />
        )}
      </main>
    </div>
  )
}

// FeedView is kept private to this file — it only makes sense as the "not reading
// an article" state of App. No need for its own file.
function FeedView({ articles, loading, error, onArticleClick }) {
  if (loading) {
    return (
      <p className="label-caps text-muted text-center py-24 tracking-widest">
        Fetching today&apos;s news...
      </p>
    )
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
      <p className="text-muted text-sm py-12">
        No articles yet. POST to{' '}
        <code className="font-mono text-ink">http://localhost:8080/ingest</code>{' '}
        to fetch the latest news.
      </p>
    )
  }

  const [lead, ...rest] = articles

  return (
    <>
      {/* Lead story — full-width, dominant headline */}
      <LeadStory article={lead} onClick={() => onArticleClick(lead)} />

      {/* Remaining articles in a newspaper-style columnar grid */}
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
