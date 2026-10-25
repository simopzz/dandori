// Package web is the app shell: the Layout every page renders in, the
// shell's own pages, and the static assets. Each feature keeps its own pages.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// static holds the vendored htmx.min.js, which is htmx 4.0.0 (BSD-2-Clause)
// from https://unpkg.com/htmx.org@4.0.0/dist/htmx.min.js. The database-
// unavailable page relies on its polling and on the HX-Refresh header.
//
//go:embed static
var static embed.FS

//go:embed root/manifest.webmanifest
var manifest []byte

//go:embed root/sw.js
var serviceWorker []byte

// Assets serves the static files under /static/, the PWA manifest and the
// service worker (which must live at the root to control the whole app).
func Assets(mux *http.ServeMux) {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(sub)))
	mux.HandleFunc("GET /manifest.webmanifest", serveBytes("application/manifest+json", manifest))
	mux.HandleFunc("GET /sw.js", serveBytes("text/javascript; charset=utf-8", serviceWorker))
}

func serveBytes(contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Write(body)
	}
}
