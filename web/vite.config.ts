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
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        // The server compares Origin to Host for cookie-authenticated writes;
        // the browser sends localhost:5173, so speak as the API server.
        configure(proxy) {
          proxy.on('proxyReq', (request) => {
            request.setHeader('origin', 'http://localhost:8080')
          })
        },
      },
      '/w': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        // A browser navigation under /w is an SPA route, not an API call: let
        // Vite serve the shell instead of the Go build's embedded HTML.
        bypass(req) {
          if (req.headers.accept?.includes('text/html')) return req.url
        },
        // The server compares Origin to Host for cookie-authenticated writes;
        // the browser sends localhost:5173, so speak as the API server.
        configure(proxy) {
          proxy.on('proxyReq', (request) => {
            request.setHeader('origin', 'http://localhost:8080')
          })
        },
      },
      '/login': {
        target: 'http://localhost:8080',
        changeOrigin: true,
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
