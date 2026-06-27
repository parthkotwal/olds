import { useState, useEffect, useRef } from 'react'
import { supabase } from '../lib/supabase'

function GoogleIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 18 18" aria-hidden="true">
      <path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z" />
      <path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.184l-2.908-2.258c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" />
      <path fill="#FBBC05" d="M3.964 10.707A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.707V4.961H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.039l3.007-2.332z" />
      <path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.961L3.964 6.293C4.672 4.166 6.656 3.58 9 3.58z" />
    </svg>
  )
}

export default function LoginModal({ onClose }) {
  const [email, setEmail] = useState('')
  const [emailSent, setEmailSent] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const overlayRef = useRef(null)

  useEffect(() => {
    function onKeyDown(e) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = prev }
  }, [])

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
    <div
      ref={overlayRef}
      onClick={handleOverlayClick}
      className="fixed inset-0 z-50 flex items-center justify-center p-6"
      style={{ backgroundColor: 'rgba(0, 0, 0, 0.48)', backdropFilter: 'blur(2px)' }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Sign in"
        className="bg-paper w-full relative border border-rule"
        style={{
          maxWidth: '22rem',
          padding: '2rem 1.75rem',
          borderTop: '2px solid var(--color-ink)',
          animation: 'modalSlideIn 200ms ease-out both',
        }}
      >
        <button
          onClick={onClose}
          aria-label="Close"
          className="absolute text-muted hover:text-ink transition-colors"
          style={{ top: '0.75rem', right: '0.75rem', background: 'none', border: 'none', cursor: 'pointer', fontSize: '1.1rem' }}
        >
          ×
        </button>

        <h2 className="font-display font-normal text-ink text-2xl headline-tight mb-1">Sign in to read</h2>
        <p className="text-muted text-xs mb-6 leading-relaxed">
          Your feed adapts to how you read.
        </p>

        {emailSent ? (
          <div>
            <p className="font-display font-bold text-ink mb-2">Check your inbox</p>
            <p className="text-muted text-xs leading-relaxed mb-4">
              We sent a link to <span className="text-ink font-medium">{email}</span>.
            </p>
            <button
              onClick={() => { setEmailSent(false); setEmail('') }}
                className="label-caps text-muted hover:text-ink transition-colors"
              style={{ background: 'none', border: 'none', cursor: 'pointer' }}
            >
              ← Different email
            </button>
          </div>
        ) : (
          <>
            <button
              onClick={handleGoogleSignIn}
              disabled={loading}
              className="w-full flex items-center justify-center gap-2.5 border border-ink px-4 py-2.5 text-sm font-medium text-ink hover:bg-ink hover:text-paper transition-colors duration-150 disabled:opacity-50 disabled:cursor-not-allowed mb-5"
            >
              <GoogleIcon />
              Continue with Google
            </button>

            <div className="flex items-center gap-3 mb-5">
              <div className="flex-1 border-t border-rule" />
              <span className="text-muted text-[0.6rem] uppercase tracking-widest">or</span>
              <div className="flex-1 border-t border-rule" />
            </div>

            <form onSubmit={handleEmailSignIn} noValidate>
              <input
                type="email"
                value={email}
                onChange={e => setEmail(e.target.value)}
                placeholder="Email address"
                required
                autoComplete="email"
                className="w-full bg-transparent border-b border-ink px-0 py-2 mb-4 text-sm text-ink placeholder:text-muted focus:outline-none focus:border-accent transition-colors"
              />
              <button
                type="submit"
                disabled={loading || !email.trim()}
                className="w-full bg-accent text-ink px-4 py-2.5 text-sm font-bold hover:opacity-80 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {loading ? 'Sending…' : 'Send magic link'}
              </button>
            </form>

            {error && (
              <p className="mt-3 bg-warm border border-rule px-3 py-2 text-xs text-ink text-center">{error}</p>
            )}
          </>
        )}
      </div>
    </div>
  )
}
