// formatToday returns the current date as a long-form dateline string:
// "Thursday, March 19, 2026" — matches the newspaper convention.
function formatToday() {
  return new Date().toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

// Header renders the newspaper masthead.
//
// Structure (top to bottom):
//   ─── thick top rule (3px, ink) ───────────────────────────────────
//   [date]                   [tagline · sign out]   ← meta strip
//   ─── thin rule ───────────────────────────────────────────────────
//               O L D S              ← masthead wordmark
//   ─── double bottom rule ──────────────────────────────────────────
//
// Props:
//   userEmail  string|undefined — shown when signed in; undefined while auth is
//                                 still resolving (avoids a flicker)
//   onSignOut  fn|null          — present when signed in
//   onSignIn   fn               — opens the login modal when signed out
export default function Header({ userEmail, onSignOut, onSignIn }) {
  return (
    <header className="bg-paper border-t-[3px] border-ink">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">

        {/* Meta strip: date left, auth controls right */}
        <div className="flex items-center justify-between py-2 border-b border-rule">
          <span className="label-caps text-muted">{formatToday()}</span>

          {/* Right side: show nothing while auth is resolving (userEmail === undefined),
              "Sign in" when logged out, "email · Sign out" when logged in. */}
          <div className="hidden sm:flex items-center gap-3">
            {userEmail ? (
              // ── Signed in ────────────────────────────────────────────────
              <>
                <span className="label-caps text-muted">{userEmail}</span>
                <span className="label-caps text-muted select-none">·</span>
                <button
                  onClick={onSignOut}
                  className="label-caps text-muted hover:text-ink transition-colors duration-150"
                >
                  Sign out
                </button>
              </>
            ) : userEmail === null ? (
              // ── Signed out (auth resolved, no session) ───────────────────
              <button
                onClick={onSignIn}
                className="label-caps text-muted hover:text-ink transition-colors duration-150"
              >
                Sign in
              </button>
            ) : null /* userEmail === undefined: auth still resolving, show nothing */ }
          </div>
        </div>

        {/* Masthead wordmark */}
        <div className="py-5 sm:py-7 text-center">
          <h1
            className="font-display font-black text-ink tracking-tight leading-none select-none"
            style={{ fontSize: 'clamp(3.5rem, 12vw, 8rem)' }}
          >
            OLDS
          </h1>
        </div>

        {/* Double bottom rule — a classic broadsheet finish */}
        <div className="border-t-2 border-ink" />
        <div className="border-t border-ink mt-[3px]" />
      </div>
    </header>
  )
}
