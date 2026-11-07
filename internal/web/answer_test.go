package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// shows is a component writing text, and the floating Notice its page shows, if any.
func shows(text string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if n := noticeOf(ctx); n != "" {
			text += " notice:" + n
		}
		_, err := io.WriteString(w, text)
		return err
	})
}

// An Answer gives htmx the fragment with its Notice and Trigger, and a plain browser
// the redirect, or else the page with its Notice; one with no fragment and no Trigger
// answers htmx as it does a plain browser, as a boosted form expects.
func TestAnswerToHTMXAndAPlainBrowser(t *testing.T) {
	for _, tc := range []struct {
		name     string
		htmx     bool
		answer   Answer
		status   int
		has      []string
		lacks    []string
		trigger  string
		location string
	}{
		{name: "htmx, saved, with a fragment", htmx: true,
			answer: Answer{Page: shows("page"), Fragment: shows("fragment"), Notice: "Saved.", Trigger: "saved", RedirectTo: "/done"},
			status: http.StatusOK, has: []string{"fragment", "Saved."}, lacks: []string{"page"}, trigger: "saved"},
		{name: "htmx, refused, with a fragment", htmx: true,
			answer: Answer{Status: http.StatusUnprocessableEntity, Page: shows("page"), Fragment: shows("fragment")},
			status: http.StatusUnprocessableEntity, has: []string{"fragment"}, lacks: []string{"page", "notice"}},
		{name: "htmx, saved, only confirming", htmx: true,
			answer: Answer{Notice: "Moved.", Trigger: "moved, close", RedirectTo: "/done"},
			status: http.StatusOK, has: []string{"Moved."}, trigger: "moved, close"},
		{name: "htmx, saved, from a boosted form", htmx: true,
			answer: Answer{Notice: "Saved.", RedirectTo: "/done"},
			status: http.StatusSeeOther, location: "/done"},
		{name: "htmx, refused, from a boosted form", htmx: true,
			answer: Answer{Status: http.StatusUnprocessableEntity, Page: shows("page")},
			status: http.StatusUnprocessableEntity, has: []string{"page"}},
		{name: "plain, saved", answer: Answer{Page: shows("page"), Fragment: shows("fragment"), Notice: "Saved.", Trigger: "saved", RedirectTo: "/done"},
			status: http.StatusSeeOther, location: "/done"},
		{name: "plain, refused", answer: Answer{Status: http.StatusUnprocessableEntity, Page: shows("page"), Fragment: shows("fragment")},
			status: http.StatusUnprocessableEntity, has: []string{"page"}, lacks: []string{"fragment", "notice"}},
		{name: "plain, saved, the page confirming it", answer: Answer{Page: shows("page"), Fragment: shows("fragment"), Notice: "Saved."},
			status: http.StatusOK, has: []string{"page notice:Saved."}, lacks: []string{"fragment"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/form", nil)
			if tc.htmx {
				r.Header.Set("HX-Request", "true")
			}
			w := httptest.NewRecorder()
			tc.answer.Write(w, r, slog.New(slog.DiscardHandler))
			body := w.Body.String()
			if w.Code != tc.status {
				t.Errorf("status %d, want %d", w.Code, tc.status)
			}
			for _, s := range tc.has {
				if !strings.Contains(body, s) {
					t.Errorf("body lacks %q:\n%s", s, body)
				}
			}
			for _, s := range tc.lacks {
				if strings.Contains(body, s) {
					t.Errorf("body has %q:\n%s", s, body)
				}
			}
			if got := w.Header().Get("HX-Trigger"); got != tc.trigger {
				t.Errorf("HX-Trigger %q, want %q", got, tc.trigger)
			}
			if got := w.Header().Get("Location"); got != tc.location {
				t.Errorf("Location %q, want %q", got, tc.location)
			}
			if tc.location != "" && !strings.Contains(w.Header().Get("Set-Cookie"), noticeCookie+"=") {
				t.Errorf("a redirect carries no Notice: %v", w.Header())
			}
		})
	}
}
