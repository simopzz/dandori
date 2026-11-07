package web

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/httpx"
)

// noticeCookie carries the floating Notice of a plain post across its redirect, to the
// page it lands on, which shows it once.
const noticeCookie = "notice"

// CloseSheet is the event an answer to a form in a sheet triggers, with HX-Trigger, to
// close that sheet: the form's work is done, and the floating Notice says so.
const CloseSheet = "close-sheet"

// Confirm says text in a floating Notice on the page the redirect after a plain post
// lands on, once: a reload of that page, or Back to it, doesn't show it again.
func Confirm(w http.ResponseWriter, r *http.Request, text string) {
	http.SetCookie(w, &http.Cookie{
		Name:     noticeCookie,
		Value:    base64.RawURLEncoding.EncodeToString([]byte(text)),
		Path:     "/",
		HttpOnly: true,
		Secure:   httpx.Secure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// Notices hands the floating Notice Confirm left to the page r asks for, for Layout to
// show, and forgets it. A fragment htmx swaps into part of a page leaves it for the page.
func Notices(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(noticeCookie)
		if err != nil || r.Method != http.MethodGet || r.Header.Get("HX-Request-Type") == "partial" {
			next.ServeHTTP(w, r)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: noticeCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: httpx.Secure(r), SameSite: http.SameSiteLaxMode})
		if text, err := base64.RawURLEncoding.DecodeString(c.Value); err == nil && len(text) > 0 {
			r = r.WithContext(WithNotice(r.Context(), string(text)))
		}
		next.ServeHTTP(w, r)
	})
}

type noticeKey struct{}

// WithNotice is ctx with text for Layout to show in a floating Notice, for a page that
// confirms something without a redirect, such as the Ingredient just picked.
func WithNotice(ctx context.Context, text string) context.Context {
	return context.WithValue(ctx, noticeKey{}, text)
}

// noticeOf is the text of the floating Notice the page in ctx shows, or "".
func noticeOf(ctx context.Context) string {
	text, _ := ctx.Value(noticeKey{}).(string)
	return text
}

// Confirmed is c, a fragment htmx swaps, followed by a floating Notice saying text,
// which htmx shows over the page, an open sheet included. c is nil for an answer that
// only confirms.
func Confirmed(c templ.Component, text string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if c != nil {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		return noticePartial(text).Render(ctx, w)
	})
}
