/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        apple: {
          blue: '#007AFF',
          gray: '#8E8E93',
          lightGray: '#F2F2F7',
          darkGray: '#1C1C1E',
        },
        sidebar: {
          DEFAULT: '#0f172a',
          hover: '#1e293b',
          active: '#1e3a5f',
          border: '#1e293b',
        },
      },
      boxShadow: {
        'card': '0 1px 3px 0 rgb(0 0 0 / 0.04), 0 1px 2px -1px rgb(0 0 0 / 0.04)',
        'card-hover': '0 4px 12px 0 rgb(0 0 0 / 0.06), 0 2px 4px -2px rgb(0 0 0 / 0.04)',
        'elevated': '0 8px 24px -4px rgb(0 0 0 / 0.08), 0 4px 8px -4px rgb(0 0 0 / 0.04)',
        'modal': '0 24px 48px -12px rgb(0 0 0 / 0.18)',
        'toast': '0 8px 16px -4px rgb(0 0 0 / 0.08), 0 0 0 1px rgb(0 0 0 / 0.02)',
      },
    },
  },
  plugins: [],
}
