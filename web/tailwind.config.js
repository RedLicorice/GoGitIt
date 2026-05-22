/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{svelte,js,ts}'],
  theme: {
    extend: {
      fontFamily: {
        // GitHub Desktop uses the system font stack; we keep it for native feel
        // but expose JetBrains Mono for code/hashes.
        sans: [
          '-apple-system',
          'BlinkMacSystemFont',
          '"Segoe UI"',
          'Roboto',
          'Helvetica',
          'Arial',
          'sans-serif',
        ],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      colors: {
        // GitHub Primer-inspired palette, dark theme primary.
        canvas: {
          DEFAULT: '#0d1117',
          subtle: '#161b22',
          inset: '#010409',
        },
        border: {
          DEFAULT: '#30363d',
          muted: '#21262d',
        },
        fg: {
          DEFAULT: '#e6edf3',
          muted: '#7d8590',
          subtle: '#6e7681',
        },
        accent: {
          DEFAULT: '#2f81f7',
          emphasis: '#1f6feb',
          subtle: 'rgba(56,139,253,0.15)',
        },
        success: { DEFAULT: '#3fb950', emphasis: '#238636' },
        danger:  { DEFAULT: '#f85149', emphasis: '#da3633' },
        attention: { DEFAULT: '#d29922', emphasis: '#9e6a03' },
      },
    },
  },
  plugins: [],
};
