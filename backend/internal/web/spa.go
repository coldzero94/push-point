//go:build embed_frontend

// Package web serves the Vite SPA embedded into the pushpoint binary so the
// single static binary ships the full web client too.
//
// It compiles only under the `embed_frontend` build tag. `//go:embed all:dist`
// fails to compile when dist/ is absent, so keeping the embed behind the tag
// lets the default backend build and CI stay green tag-less (see
// spa_noembed.go); only release builds bundle the front end:
//
//	just web-build && go build -tags embed_frontend ./cmd/pushpoint
//
// The embed directive can reach only files under this package directory — a
// relative `../../../frontend/dist` is impossible — so `just web-build` copies
// frontend/dist → backend/internal/web/dist before the tagged build. That
// copied (gitignored) tree is what `dist` below embeds.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded SPA. NewRouter hangs it off chi's NotFound hook,
// so it only ever sees paths no contract route matched — and the router itself
// answers /api, /thumbs and /healthz misses with a JSON error instead of the
// shell. Behaviour:
//   - a request resolving to a real embedded file is served by
//     http.FileServerFS (correct Content-Type, Range, If-Modified-Since);
//   - /assets/* (Vite content-hashed bundles) get an immutable long cache,
//     everything else no-cache so a new deploy is picked up immediately;
//   - an unmatched extension-less path falls back to index.html so client-side
//     routes (TanStack Router) deep-link and survive a hard reload;
//   - an unmatched path *with* an extension (/nope.js) is a 404, never the
//     shell — it is an asset request, and a silent 200 text/html would mask
//     base-path bugs.
//
// The SPA app shell is served without auth by design: a browser cannot attach a
// Bearer token on initial navigation. The API key is entered in the Settings
// screen and applied only to /api calls, which stay behind BearerAuth.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// Unreachable: the //go:embed above guarantees the dist subtree exists.
		panic("web: embedded dist subtree missing: " + err.Error())
	}
	fileServer := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path (defuses ".." traversal) and map "/" to index.html.
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "index.html" {
			serveShell(w, r, fileServer)
			return
		}
		info, statErr := fs.Stat(sub, name)
		if statErr != nil || info.IsDir() {
			// Not a concrete file (unknown path or directory).
			if path.Ext(name) != "" {
				// A path with an extension asks for an asset, not a client
				// route: answer 404 instead of the shell, so a broken base
				// path or a stale bundle reference fails loudly rather than
				// hiding behind a 200 text/html the browser cannot execute.
				http.NotFound(w, r)
				return
			}
			serveShell(w, r, fileServer)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// serveShell returns index.html for the app shell / SPA fallback with no-cache
// so the shell (which references content-hashed assets) is always revalidated.
// It reuses FileServerFS by serving the "/" directory index (index.html).
func serveShell(w http.ResponseWriter, r *http.Request, fileServer http.Handler) {
	w.Header().Set("Cache-Control", "no-cache")
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.Path = "/"
	u.RawPath = ""
	r2.URL = &u
	fileServer.ServeHTTP(w, r2)
}
