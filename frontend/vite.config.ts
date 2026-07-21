import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Push-Point web — pure SPA, embedded into the Go binary in production.
// https://vite.dev/config/
export default defineConfig({
  // Origin-relative base: emitted asset URLs are "/assets/..." — same code path
  // for dev proxy and prod embed, with no hardcoded host. It must NOT be './':
  // document-relative URLs break deep links (/links/123 would resolve assets to
  // /links/assets/..., which the SPA fallback answers with index.html and the
  // browser rejects on MIME). "/assets/" also matches the immutable-cache branch
  // in backend/internal/web/spa.go.
  base: '/',
  plugins: [react(), tailwindcss()],
  server: {
    // Dev (:5173) proxies the backend surface to the Go server (:8080) so the
    // client only ever uses relative paths — identical to prod embed (same-origin).
    proxy: {
      '/api': 'http://localhost:8080',
      '/thumbs': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
  build: {
    // dist/ is embedded via //go:embed all:dist (build tag embed_frontend).
    outDir: 'dist',
  },
})
