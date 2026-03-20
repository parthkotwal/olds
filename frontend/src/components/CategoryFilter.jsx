const CATEGORIES = [
  { value: 'all',           label: 'All' },
  { value: 'general',       label: 'General' },
  { value: 'business',      label: 'Business' },
  { value: 'technology',    label: 'Technology' },
  { value: 'science',       label: 'Science' },
  { value: 'health',        label: 'Health' },
  { value: 'sports',        label: 'Sports' },
  { value: 'entertainment', label: 'Entertainment' },
]

// CategoryFilter renders the horizontal category navigation strip.
//
// Design: small-caps, letterspaced labels separated by whitespace.
// Active category gets an accent-red underline — the only color on the strip.
// No pills, no background fills, no rounded corners. Newspapers don't have those.
//
// Props:
//   selected  string  — the currently selected category value
//   onSelect  fn      — called with the new category value on click
export default function CategoryFilter({ selected, onSelect }) {
  return (
    <nav className="bg-paper border-b border-rule sticky top-0 z-10">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center gap-5 sm:gap-7 py-3 overflow-x-auto no-scrollbar">
          {CATEGORIES.map(({ value, label }) => (
            <button
              key={value}
              onClick={() => onSelect(value)}
              className={[
                'label-caps whitespace-nowrap flex-shrink-0 pb-px transition-colors duration-150',
                selected === value
                  ? 'text-ink border-b border-accent'
                  : 'text-muted hover:text-ink',
              ].join(' ')}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
    </nav>
  )
}
