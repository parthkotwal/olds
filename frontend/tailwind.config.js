/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
  ],
  theme: {
    extend: {
      fontFamily: {
        display: ['"Playfair Display"', 'Georgia', 'serif'],
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
      },
      colors: {
        paper:  '#FDFCF3',
        surface:'#FFFFFF',
        warm:   '#FDFBE4',
        ink:    '#000000',
        charcoal:'#211D1C',
        muted:  '#6E6E6E',
        faint:  '#B3B3B3',
        accent: '#FFC500',
        rule:   '#D9D9D9',
      },
      maxWidth: {
        article: '42.5rem',
        layout:  '76rem',
      },
    },
  },
  plugins: [],
}
