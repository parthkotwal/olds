/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
  ],
  theme: {
    extend: {
      fontFamily: {
        // "display" → use as `font-display` in className
        display: ['"Playfair Display"', 'Georgia', 'serif'],
        // Override the default sans stack to use Inter
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
      },
      colors: {
        paper: '#FAF8F5',   // warm off-white — aged newsprint
        ink:   '#1A1A1A',   // near-black
        muted: '#8A8A8A',   // secondary text, dividers
        accent:'#C0392B',   // ink-red — hover, active category, live indicators
        rule:  '#D4D0C8',   // thin column rules and border lines
      },
      maxWidth: {
        article: '42.5rem', // ~680px — comfortable single-column reading width
      },
    },
  },
  plugins: [],
}
