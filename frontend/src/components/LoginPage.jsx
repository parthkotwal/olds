import { useState } from 'react'
import { supabase } from '../lib/supabase'

// formatToday matches the dateline style in the main Header.
function formatToday() {
  return new Date().toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

// GoogleIcon is the official Google "G" logo in SVG.
// Inline SVG keeps it self-contained — no external icon dependency.
function GoogleIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
      <path
        fill="#4285F4"
        d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z"
      />
      <path
        fill="#34A853"
        d="M9 18c2.43 0 4.467-.806 5.956-2.184l-2.908-2.258c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"
      />
      <path
        fill="#FBBC05"
        d="M3.964 10.707A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.707V4.961H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.039l3.007-2.332z"
      />
      <path
        fill="#EA4335"
        d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.961L3.964 6.293C4.672 4.166 6.656 3.58 9 3.58z"
      />
    </svg>
  )
}

// LoginPage renders the authentication screen in the same editorial aesthetic
// as the main app — masthead, dateline, thin rules, no decoration.
//
// Two sign-in methods:
//   1. Google OAuth — one click, redirects through Google and back
//   2. Email magic link — passwordless, Supabase emails a sign-in link
//
// After a successful login, the Supabase client fires onAuthStateChange in
// App.jsx and the session state is set — this component unmounts automatically.
export default function LoginPage() {
  const [email, setEmail] = useState('')
  const [emailSent, setEmailSent] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  async function handleGoogleSignIn() {
    setLoading(true)
    setError(null)
    const { error } = await supabase.auth.signInWithOAuth({
      provider: 'google',
      options: {
        // After Google auth completes, redirect back to wherever the app is
        // running. window.location.origin is localhost:5173 in local dev and
        // the production URL in deployment — no hardcoding needed.
        redirectTo: window.location.origin,
      },
    })
    if (error) {
      setError(error.message)
      setLoading(false)
    }
    // On success, the browser navigates to Google — this component is gone.
  }

  async function handleEmailSignIn(e) {
    e.preventDefault()
    if (!email.trim()) return

    setLoading(true)
    setError(null)
    const { error } = await supabase.auth.signInWithOtp({
      email: email.trim(),
      options: {
        emailRedirectTo: window.location.origin,
      },
    })
    setLoading(false)
    if (error) {
      setError(error.message)
    } else {
      setEmailSent(true)
    }
  }

  return (
    <div className="min-h-screen bg-paper flex flex-col">

      {/* ── Masthead — identical structure to the main Header ──────────────── */}
      <header className="bg-paper border-t-[3px] border-ink">
        <div className="max-w-2xl mx-auto px-6">

          {/* Meta strip */}
          <div className="flex items-center justify-between py-2 border-b border-rule">
            <span className="label-caps text-muted">{formatToday()}</span>
            <span className="label-caps text-muted hidden sm:block">
              Cross-topic connections, served daily
            </span>
          </div>

          {/* Wordmark */}
          <div className="py-5 sm:py-7 text-center">
            <h1
              className="font-display font-black text-ink tracking-tight leading-none select-none"
              style={{ fontSize: 'clamp(3.5rem, 12vw, 8rem)' }}
            >
              OLDS
            </h1>
          </div>

          {/* Double bottom rule */}
          <div className="border-t-2 border-ink" />
          <div className="border-t border-ink mt-[3px]" />
        </div>
      </header>

      {/* ── Sign-in form ───────────────────────────────────────────────────── */}
      <main className="flex-1 flex items-start justify-center pt-14 px-6">
        <div className="w-full max-w-sm">

          {/* Section label */}
          <p className="label-caps text-muted text-center mb-8 tracking-widest">
            Sign in to read
          </p>

          {emailSent ? (
            // ── Confirmation state ──────────────────────────────────────────
            <div className="text-center">
              <div className="border-t border-rule mb-6" />
              <p className="font-display text-ink text-xl font-bold mb-3">
                Check your inbox
              </p>
              <p className="text-muted text-sm leading-relaxed mb-6">
                We sent a sign-in link to{' '}
                <span className="text-ink font-medium">{email}</span>.
                Click the link to continue — no password needed.
              </p>
              <button
                onClick={() => { setEmailSent(false); setEmail('') }}
                className="label-caps text-muted hover:text-ink transition-colors duration-150"
              >
                ← Use a different email
              </button>
              <div className="border-t border-rule mt-6" />
            </div>
          ) : (
            <>
              {/* ── Google OAuth ──────────────────────────────────────────── */}
              <button
                onClick={handleGoogleSignIn}
                disabled={loading}
                className="
                  w-full flex items-center justify-center gap-3
                  border border-ink
                  px-4 py-3
                  font-sans text-sm font-medium text-ink
                  hover:bg-ink hover:text-paper
                  transition-colors duration-150
                  disabled:opacity-50 disabled:cursor-not-allowed
                "
              >
                <GoogleIcon />
                Continue with Google
              </button>

              {/* ── Divider ───────────────────────────────────────────────── */}
              <div className="flex items-center gap-4 my-6">
                <div className="flex-1 border-t border-rule" />
                <span className="label-caps text-muted">or</span>
                <div className="flex-1 border-t border-rule" />
              </div>

              {/* ── Email magic link ──────────────────────────────────────── */}
              <form onSubmit={handleEmailSignIn} noValidate>
                <label
                  htmlFor="email"
                  className="label-caps text-muted block mb-2"
                >
                  Email address
                </label>
                <input
                  id="email"
                  type="email"
                  value={email}
                  onChange={e => setEmail(e.target.value)}
                  placeholder="you@example.com"
                  required
                  autoComplete="email"
                  className="
                    w-full bg-transparent
                    border-b border-ink
                    px-0 py-2 mb-4
                    font-sans text-sm text-ink
                    placeholder:text-muted
                    focus:outline-none focus:border-accent
                    transition-colors duration-150
                  "
                />
                <button
                  type="submit"
                  disabled={loading || !email.trim()}
                  className="
                    w-full
                    bg-ink text-paper
                    px-4 py-3
                    font-sans text-sm font-medium
                    hover:opacity-90
                    transition-opacity duration-150
                    disabled:opacity-40 disabled:cursor-not-allowed
                  "
                >
                  {loading ? 'Sending…' : 'Send magic link'}
                </button>
              </form>

              {/* ── Error message ─────────────────────────────────────────── */}
              {error && (
                <p className="mt-4 text-sm text-accent text-center">
                  {error}
                </p>
              )}
            </>
          )}

          {/* Footer note */}
          <p className="label-caps text-muted text-center mt-10 leading-relaxed">
            No password. No account creation.
            <br />
            Just your identity.
          </p>

        </div>
      </main>
    </div>
  )
}
