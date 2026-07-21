import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Ports are configurable so a busy port on the dev machine never blocks a run.
// `just dev` picks the first free port from 8420 and records it; `just web-dev`
// reads that port and passes it here as PUSHPOINT_API_PORT, so parallel
// checkouts (one worktree per agent) each proxy to their own backend.
const apiPort = process.env.PUSHPOINT_API_PORT ?? '8420'
const apiTarget = `http://localhost:${apiPort}`
const webPort = Number(process.env.PUSHPOINT_WEB_PORT ?? 8421)

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
    // Dev (:8421 by default) proxies the backend surface to the Go server so the
    // client only ever uses relative paths — identical to prod embed (same-origin).
    port: webPort,
    // strictPort off: if 8421 is taken, Vite moves to the next free port itself.
    strictPort: false,
    proxy: {
      '/api': apiTarget,
      '/thumbs': apiTarget,
      '/healthz': apiTarget,
    },
  },
  build: {
    // dist/ is embedded via //go:embed all:dist (build tag embed_frontend).
    outDir: 'dist',
  },
})
