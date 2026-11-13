// Package web is the app shell: the Layout every page renders in, the
// shell's own pages, and the static assets. Each feature keeps its own pages.
package web

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
)

// static holds the vendored htmx.min.js, which is htmx 4.0.1 (BSD-2-Clause)
// from https://unpkg.com/htmx.org@4.0.1/dist/htmx.min.js. It boosts every
// page (ADR 0009). Next to it, hx-sse.min.js is its SSE extension, of the same version
// and licence, from https://unpkg.com/htmx.org@4.0.1/dist/ext/hx-sse.min.js: a Run's
// page follows its Trace live through it.
// The htmx-guidance and htmx-debugging skills in .agents/skills and
// .claude/skills come from the same package's dist/skills: upgrade them
// with htmx.
//
// static/faro-web-sdk.min.js is Grafana's Faro Web SDK 2.13.0 (Apache-2.0), from
// https://unpkg.com/@grafana/faro-web-sdk@2.13.0/dist/bundle/faro-web-sdk.iife.js
// (sha256-hXQcRf0f1IdcXyWpWnlA4RceoR4wioPYj9XzpdyrjBw=): Faro starts it, static/faro.js,
// in production only (ADR 0020).
//
// static/fonts holds DotGothic16 (400) and Zen Kaku Gothic New (400, 700), both SIL OFL
// 1.1 (static/fonts/OFL.txt). Zen Kaku Gothic New is Fontsource's latin subset
// (@fontsource/zen-kaku-gothic-new). DotGothic16 is subset from google/fonts'
// ofl/dotgothic16 to Latin-1, typographic punctuation and 段取り, the titles it sets:
//
//	pyftsubset DotGothic16-Regular.ttf --flavor=woff2 --layout-features='*' \
//	  --unicodes=U+0020-007E,U+00A0-00FF,U+2013-2014,U+2018-2019,U+201C-201D \
//	  --text='…段取り'
//
// static/icon-192.png and icon-512.png are drawn from icon.svg: 段 in sumi
// ink on the paper with a vermilion seal, inside the maskable safe zone.
// Its 段 is set in Noto Serif CJK JP Bold (SIL OFL); render it again with
// `rsvg-convert -w 512 -h 512 icon.svg -o static/icon-512.png`, and -w 192 -h 192
// for icon-192.png.
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

type faroKey struct{}

// Faro serves next with the Faro collector's url, which the Layout sends the browser's
// telemetry to (static/faro.js). Without one, as in `mise run dev` and the tests, the
// pages load no Faro at all.
func Faro(url string, next http.Handler) http.Handler {
	if url == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), faroKey{}, url)))
	})
}

func faroURL(ctx context.Context) string {
	url, _ := ctx.Value(faroKey{}).(string)
	return url
}
