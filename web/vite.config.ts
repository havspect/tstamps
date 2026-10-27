import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Every server serves workspaces under /w/{slug}; the root is the public
// homepage and the workspace prompt. Vite serves the shell in dev, so mirror the
// marker the Go handler injects.
const hosted = true

export default defineConfig({
  plugins: [
    svelte(),
    {
      name: 'tstamps-hosted-marker',
      transformIndexHtml: (html) =>
        hosted ? html.replace('</head>', '<meta name="tstamps-hosted" content="1"></head>') : html,
    },
  ],
  server: {
    // Keep the browser's Host header (localhost:5173) on Go routes instead of
    // setting changeOrigin: true. That satisfies the API's Origin == Host CSRF
    // check without header rewriting and ensures OAuth/MCP discovery advertises
    // http://localhost:5173 (where Vite serves /w/{slug}/oauth/authorize and
    // /w/{slug}/auth/device) rather than the UI-less Go server on :8080.
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
      },
      '/.well-known': {
        target: 'http://localhost:8080',
      },
      '/w': {
        target: 'http://localhost:8080',
        // A browser navigation under /w is an SPA route, not an API call: let
        // Vite serve the shell instead of the Go build's embedded HTML.
        bypass(req) {
          if (req.headers.accept?.includes('text/html')) return req.url
        },
      },
      '/login': {
        target: 'http://localhost:8080',
      },
      '/docs': {
        target: 'http://localhost:5174',
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
