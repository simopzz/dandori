package web

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/httpx"
)

// Answer is the answer to a write, as ADR-0009 has it. To htmx, it is Fragment followed
// by Notice in a floating Notice, with Trigger in HX-Trigger. To a plain browser, it is
// a redirect to RedirectTo, whose page says Notice, when that is set, or else Page,
// saying Notice. One with neither Fragment nor Trigger answers a boosted form, so htmx
// gets what a plain browser does.
type Answer struct {
	// Status is the status of a fragment or page, 200 when unset.
	Status                      int
	Page, Fragment              templ.Component
	Notice, Trigger, RedirectTo string
}

// Redirects reports whether a answers r with its redirect whatever its Page and Fragment,
// so a write needn't load what they show.
func (a Answer) Redirects(r *http.Request) bool {
	return a.RedirectTo != "" && !httpx.IsHTMX(r)
}

// Write answers r with a.
func (a Answer) Write(w http.ResponseWriter, r *http.Request, log *slog.Logger) {
	status := a.Status
	if status == 0 {
		status = http.StatusOK
	}
	if httpx.IsHTMX(r) && (a.Fragment != nil || a.Trigger != "") {
		if a.Trigger != "" {
			w.Header().Set("HX-Trigger", a.Trigger)
		}
		body := a.Fragment
		if body == nil {
			body = templ.NopComponent
		}
		if a.Notice != "" {
			body = FragmentWithNotice(a.Fragment, a.Notice)
		}
		httpx.Render(w, r, log, status, body)
		return
	}
	if a.RedirectTo != "" { // a plain browser, or htmx from a boosted form
		redirectWithNotice(w, r, a.RedirectTo, a.Notice)
		return
	}
	if a.Notice != "" {
		r = r.WithContext(WithNotice(r.Context(), a.Notice))
	}
	httpx.Render(w, r, log, status, a.Page)
}
