function formatToday() {
  return new Date().toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

export default function Header({ userEmail, onSignOut, onSignIn }) {
  return (
    <header className="bg-paper">
      <div className="border-t-2 border-ink" />
      <div className="max-w-layout mx-auto px-5 sm:px-8">
        <div className="flex items-center justify-between py-3.5">
          <div className="flex items-baseline gap-4">
            <h1
              className="font-display font-black text-ink leading-none select-none headline-tight"
              style={{ fontSize: '1.9rem' }}
            >
              Olds
            </h1>
            <span className="label-caps text-muted hidden sm:inline">
              {formatToday()}
            </span>
          </div>

          <div className="flex items-center gap-3">
            {userEmail ? (
              <>
                <span className="label-caps text-muted hidden md:inline">{userEmail}</span>
                <button
                  onClick={onSignOut}
                  className="label-caps text-muted hover:text-ink transition-colors duration-150"
                >
                  Sign out
                </button>
              </>
            ) : userEmail === null ? (
              <button
                onClick={onSignIn}
                className="label-caps bg-accent text-ink px-4 py-2 transition-opacity duration-150 hover:opacity-80"
              >
                Sign in
              </button>
            ) : null}
          </div>
        </div>
      </div>
      <div className="border-b border-rule" />
    </header>
  )
}
