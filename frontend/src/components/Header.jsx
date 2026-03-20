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
//   [date]                           [tagline]   ← meta strip
//   ─── thin rule ───────────────────────────────────────────────────
//               O L D S              ← masthead wordmark
//   ─── double bottom rule ──────────────────────────────────────────
//
// The bold top border + large centered wordmark is the newspaper's
// primary visual identity signal. Everything below it is content.
export default function Header() {
  return (
    <header className="bg-paper border-t-[3px] border-ink">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">

        {/* Meta strip: date left, tagline right */}
        <div className="flex items-center justify-between py-2 border-b border-rule">
          <span className="label-caps text-muted">{formatToday()}</span>
          <span className="label-caps text-muted hidden sm:block">
            Cross-topic connections, served daily
          </span>
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
