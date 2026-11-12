// Package httpx holds the small HTTP helpers every feature's handlers share.
package httpx

import (
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
		log.ErrorContext(r.Context(), "render", "path", r.URL.Path, "err", err)
	}
}

// Secure reports whether the browser reached the app over https: directly, or through
// Caddy, which says so in X-Forwarded-Proto.
func Secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// ServerError logs err as what failed, in r's span, and answers 500 without its details
// but with the span's trace id, for a report to quote.
func ServerError(w http.ResponseWriter, r *http.Request, log *slog.Logger, what string, err error) {
	log.ErrorContext(r.Context(), what, "err", err)
	msg := "Something went wrong; see the server log."
	if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
		msg += "\nTrace " + sc.TraceID().String()
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

// Refused records on r's span why the app refuses it, in English for whoever reads the
// span: reason, followed by the fields of errs, a form's, if it has any. Never what the
// User typed.
func Refused(r *http.Request, reason string, errs map[string]string) {
	if len(errs) > 0 {
		reason += ": " + strings.Join(slices.Sorted(maps.Keys(errs)), ", ")
	}
	trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("dandori.refusal", reason))
}
