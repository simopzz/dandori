// Package web is the app shell: the Layout every page renders in, the
// shell's own pages, and the static assets. Each feature keeps its own pages.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// static holds the vendored htmx.min.js, which is htmx 4.0.0 (BSD-2-Clause)
// from https://unpkg.com/htmx.org@4.0.0/dist/htmx.min.js. It boosts every
// page (ADR 0009), and the database-unavailable page relies on its polling
// and on the HX-Refresh header. Next to it, hx-sse.min.js is its SSE
// extension, of the same version and licence, from
// https://unpkg.com/htmx.org@4.0.0/dist/ext/hx-sse.min.js: the trace panel
// follows a running Run through it, and trace.js keeps its newest events
// in view.
// The htmx-guidance and htmx-debugging skills in .agents/skills and
// .claude/skills come from the same package's dist/skills: upgrade them
// with htmx.
//
// static/fonts holds the latin subsets of Shippori Mincho (500, 700) and
// Zen Kaku Gothic New (400, 700), both SIL OFL 1.1 (static/fonts/OFL.txt),
// from Fontsource (@fontsource/shippori-mincho, @fontsource/zen-kaku-gothic-new).
//
//go:embed static
var static embed.FS

// themeColour is the browser chrome's colour: the paper, --color-bg in
// app.css. The manifest's theme_color and background_color match it.
const themeColour = "#fdfcfa"

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
