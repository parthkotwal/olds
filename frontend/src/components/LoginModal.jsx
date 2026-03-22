import { useState, useEffect, useRef } from 'react'
import { supabase } from '../lib/supabase'

function GoogleIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
      <path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z" />
      <path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.184l-2.908-2.258c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" />
      <path fill="#FBBC05" d="M3.964 10.707A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.707V4.961H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.039l3.007-2.332z" />
      <path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.961L3.964 6.293C4.672 4.166 6.656 3.58 9 3.58z" />
    </svg>
  )
}

/**
 * LoginModal appears when an unauthenticated user clicks an article.
 *
 * Design intent: it should feel like a moment in the newspaper — an insert
 * that arrives when you try to go deeper. Not a blocker, not a hard wall.
 * Dismissible (Escape key or click outside the panel).
 *
 * After a successful sign-in, App.jsx's onAuthStateChange handler fires,
 * sets the session, and opens the pending article — this component unmounts.
 *
 * Props:
 *   onClose  fn  — called when the modal should be dismissed
 */
export default function LoginModal({ onClose }) {
  const [email, setEmail] = useState('')
  const [emailSent, setEmailSent] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const overlayRef = useRef(null)

  // Close on Escape key — expected behaviour for any modal.
  useEffect(() => {
    function onKeyDown(e) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  // Prevent body scroll while the modal is open.
  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = prev }
  }, [])

  // Click-outside: only close if the click landed on the overlay itself,
  // not on the modal panel (which is a child of the overlay).
  function handleOverlayClick(e) {
    if (e.target === overlayRef.current) onClose()
  }

  async function handleGoogleSignIn() {
    setLoading(true)
    setError(null)
    const { error } = await supabase.auth.signInWithOAuth({
      provider: 'google',
      options: { redirectTo: window.location.origin },
    })
    if (error) {
      setError(error.message)
      setLoading(false)
    }
  }

  async function handleEmailSignIn(e) {
    e.preventDefault()
    if (!email.trim()) return
    setLoading(true)
    setError(null)
    const { error } = await supabase.auth.signInWithOtp({
      email: email.trim(),
      options: { emailRedirectTo: window.location.origin },
    })
    setLoading(false)
    if (error) setError(error.message)
    else setEmailSent(true)
  }

  return (
    // ── Overlay ─────────────────────────────────────────────────────────────
    <div
      ref={overlayRef}
      onClick={handleOverlayClick}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 50,
        backgroundColor: 'rgba(26, 26, 26, 0.60)',
        backdropFilter: 'blur(2px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '1.5rem',
      }}
    >
      {/* ── Modal panel ───────────────────────────────────────────────────── */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Sign in to read"
        style={{
          backgroundColor: 'var(--color-paper)',
          borderTop: '3px solid var(--color-ink)',
          width: '100%',
          maxWidth: '23rem',
          padding: '2rem',
          position: 'relative',
          animation: 'modalSlideIn 200ms ease-out both',
        }}
      >
        {/* Dismiss button */}
        <button
          onClick={onClose}
          aria-label="Close"
          style={{
            position: 'absolute',
            top: '1rem',
            right: '1rem',
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: 'var(--color-muted)',
            fontSize: '1.25rem',
            lineHeight: 1,
            padding: '0.25rem',
            transition: 'color 150ms',
          }}
          onMouseEnter={e => { e.currentTarget.style.color = 'var(--color-ink)' }}
          onMouseLeave={e => { e.currentTarget.style.color = 'var(--color-muted)' }}
        >
          ×
        </button>

        {/* Header */}
        <p
          className="label-caps text-muted text-center"
          style={{ marginBottom: '0.375rem', fontSize: '0.6rem' }}
        >
          Olds
        </p>
        <p
          className="font-display font-black text-ink text-center"
          style={{ fontSize: '1.25rem', marginBottom: '0.25rem' }}
        >
          Sign in to read
        </p>
        <p
          className="text-muted text-center"
          style={{ fontSize: '0.75rem', marginBottom: '1.75rem', lineHeight: 1.5 }}
        >
          Cross-topic connections, personalized to you.
        </p>

        <div style={{ borderTop: '1px solid var(--color-rule)', marginBottom: '1.75rem' }} />

        {emailSent ? (
          // ── Confirmation ────────────────────────────────────────────────
          <div style={{ textAlign: 'center' }}>
            <p
              className="font-display font-bold text-ink"
              style={{ fontSize: '1rem', marginBottom: '0.5rem' }}
            >
              Check your inbox
            </p>
            <p className="text-muted" style={{ fontSize: '0.75rem', lineHeight: 1.6, marginBottom: '1.25rem' }}>
              We sent a link to{' '}
              <span style={{ color: 'var(--color-ink)', fontWeight: 500 }}>{email}</span>.
              Click it to continue — no password needed.
            </p>
            <button
              onClick={() => { setEmailSent(false); setEmail('') }}
              className="label-caps text-muted"
              style={{ background: 'none', border: 'none', cursor: 'pointer', transition: 'color 150ms' }}
              onMouseEnter={e => { e.currentTarget.style.color = 'var(--color-ink)' }}
              onMouseLeave={e => { e.currentTarget.style.color = 'var(--color-muted)' }}
            >
              ← Different email
            </button>
          </div>
        ) : (
          <>
            {/* Google */}
            <button
              onClick={handleGoogleSignIn}
              disabled={loading}
              style={{
                width: '100%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: '0.625rem',
                border: '1px solid var(--color-ink)',
                padding: '0.625rem 1rem',
                background: 'none',
                cursor: loading ? 'not-allowed' : 'pointer',
                opacity: loading ? 0.5 : 1,
                fontSize: '0.8125rem',
                fontWeight: 500,
                color: 'var(--color-ink)',
                transition: 'background 150ms, color 150ms',
                marginBottom: '1.25rem',
              }}
              onMouseEnter={e => {
                if (!loading) {
                  e.currentTarget.style.background = 'var(--color-ink)'
                  e.currentTarget.style.color = 'var(--color-paper)'
                }
              }}
              onMouseLeave={e => {
                e.currentTarget.style.background = 'none'
                e.currentTarget.style.color = 'var(--color-ink)'
              }}
            >
              <GoogleIcon />
              Continue with Google
            </button>

            {/* Divider */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '1.25rem' }}>
              <div style={{ flex: 1, borderTop: '1px solid var(--color-rule)' }} />
              <span className="label-caps text-muted" style={{ fontSize: '0.6rem' }}>or</span>
              <div style={{ flex: 1, borderTop: '1px solid var(--color-rule)' }} />
            </div>

            {/* Email magic link */}
            <form onSubmit={handleEmailSignIn} noValidate>
              <label htmlFor="modal-email" className="label-caps text-muted" style={{ display: 'block', marginBottom: '0.375rem', fontSize: '0.6rem' }}>
                Email address
              </label>
              <input
                id="modal-email"
                type="email"
                value={email}
                onChange={e => setEmail(e.target.value)}
                placeholder="you@example.com"
                required
                autoComplete="email"
                style={{
                  width: '100%',
                  background: 'transparent',
                  border: 'none',
                  borderBottom: '1px solid var(--color-ink)',
                  padding: '0.375rem 0',
                  marginBottom: '1rem',
                  fontSize: '0.875rem',
                  color: 'var(--color-ink)',
                  outline: 'none',
                  boxSizing: 'border-box',
                }}
              />
              <button
                type="submit"
                disabled={loading || !email.trim()}
                style={{
                  width: '100%',
                  background: 'var(--color-ink)',
                  color: 'var(--color-paper)',
                  border: 'none',
                  padding: '0.625rem 1rem',
                  fontSize: '0.8125rem',
                  fontWeight: 500,
                  cursor: loading || !email.trim() ? 'not-allowed' : 'pointer',
                  opacity: loading || !email.trim() ? 0.45 : 1,
                  transition: 'opacity 150ms',
                }}
              >
                {loading ? 'Sending…' : 'Send magic link'}
              </button>
            </form>

            {error && (
              <p
                className="text-center"
                style={{ marginTop: '0.875rem', fontSize: '0.75rem', color: 'var(--color-accent)' }}
              >
                {error}
              </p>
            )}
          </>
        )}
      </div>
    </div>
  )
}
