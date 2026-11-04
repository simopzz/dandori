// Package httpx holds the small HTTP helpers every feature's handlers share.
package httpx

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
)

// PathID reads the {id} path value, answering 404 if it is not a number.
func PathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

// IsHTMX reports whether htmx sent the request: a boosted one, or a form
// swapping its own fragment.
func IsHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// Render answers with page as HTML, with status.
func Render(w http.ResponseWriter, r *http.Request, log *slog.Logger, status int, page templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := page.Render(r.Context(), w); err != nil {
		log.Error("render", "path", r.URL.Path, "err", err)
	}
}

// Secure reports whether the browser reached the app over https: directly, or through
// Caddy, which says so in X-Forwarded-Proto.
func Secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// ServerError logs err as what failed and answers 500 without its details.
func ServerError(w http.ResponseWriter, log *slog.Logger, what string, err error) {
	log.Error(what, "err", err)
	http.Error(w, "Something went wrong; see the server log.", http.StatusInternalServerError)
}
