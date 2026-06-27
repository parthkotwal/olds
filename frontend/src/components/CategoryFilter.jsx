const CATEGORIES = [
  { value: 'all',           label: 'Top Stories' },
  { value: 'general',       label: 'World' },
  { value: 'business',      label: 'Business' },
  { value: 'technology',    label: 'Tech' },
  { value: 'science',       label: 'Science' },
  { value: 'health',        label: 'Health' },
  { value: 'sports',        label: 'Sports' },
  { value: 'entertainment', label: 'Culture' },
]

export default function CategoryFilter({ selected, onSelect }) {
  return (
    <nav className="bg-paper sticky top-0 z-10 border-y border-rule">
      <div className="max-w-layout mx-auto px-5 sm:px-8">
        <div className="flex items-center overflow-x-auto no-scrollbar py-2">
          {CATEGORIES.map(({ value, label }, index) => (
            <div key={value} className="flex flex-shrink-0 items-center">
              {index > 0 && (
                <span className="text-faint px-1.5 text-xs" aria-hidden="true">|</span>
              )}
              <button
                onClick={() => onSelect(value)}
                className={[
                  'editorial-label whitespace-nowrap transition-colors duration-150',
                  'px-1 py-1',
                  selected === value
                    ? 'text-ink'
                    : 'text-muted hover:text-ink',
                ].join(' ')}
              >
                {label}
              </button>
            </div>
          ))}
        </div>
      </div>
    </nav>
  )
}
