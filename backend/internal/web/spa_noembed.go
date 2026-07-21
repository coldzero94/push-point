//go:build !embed_frontend

// Package web serves the embedded Vite SPA under the `embed_frontend` build tag
// (see spa.go). This tag-less variant is a no-op: the default backend build and
// CI ship no bundled front end.
package web

import "net/http"

// Handler returns nil without the `embed_frontend` build tag, so NewRouter
// answers every unmatched path with a JSON 404 instead of an app shell. Release
// builds embed the front end via spa.go
// (`go build -tags embed_frontend` after `just web-build`).
func Handler() http.Handler { return nil }
