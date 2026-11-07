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

// redirectWithNotice answers a plain post with a redirect to path, whose page says text
// in a floating Notice, once: a reload of that page, or Back to it, doesn't show it.
func redirectWithNotice(w http.ResponseWriter, r *http.Request, path, text string) {
	http.SetCookie(w, noticeCookieOf(r, base64.RawURLEncoding.EncodeToString([]byte(text)), 0))
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// noticeCookieOf is the notice cookie holding value, or with maxAge -1 its deletion.
func noticeCookieOf(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: noticeCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: httpx.Secure(r), SameSite: http.SameSiteLaxMode,
	}
}

// Notices hands the floating Notice redirectWithNotice left to the page r asks for, for
// Layout to show, and forgets it once that page renders. A fragment htmx swaps into part
// of a page, or a redirect on, as to sign in, leaves it for the page after.
func Notices(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(noticeCookie)
		if err != nil || r.Method != http.MethodGet || r.Header.Get("HX-Request-Type") == "partial" {
			next.ServeHTTP(w, r)
			return
		}
		if text, err := base64.RawURLEncoding.DecodeString(c.Value); err == nil && len(text) > 0 {
			r = r.WithContext(WithNotice(r.Context(), string(text)))
		}
		next.ServeHTTP(&forgetsNotice{ResponseWriter: w, r: r}, r)
	})
}

// forgetsNotice deletes the notice cookie with the response's header, unless the
// response is a redirect, which shows no page.
type forgetsNotice struct {
	http.ResponseWriter
	r           *http.Request
	wroteHeader bool
}

func (f *forgetsNotice) WriteHeader(status int) {
	if !f.wroteHeader {
		f.wroteHeader = true
		if status < 300 || status >= 400 {
			http.SetCookie(f.ResponseWriter, noticeCookieOf(f.r, "", -1))
		}
	}
	f.ResponseWriter.WriteHeader(status)
}

func (f *forgetsNotice) Write(b []byte) (int, error) {
	if !f.wroteHeader {
		f.WriteHeader(http.StatusOK)
	}
	return f.ResponseWriter.Write(b)
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

// FragmentWithNotice is c, a fragment htmx swaps, followed by a floating Notice saying
// text, which htmx shows over the page, an open sheet included. c is nil for an answer
// that only confirms.
func FragmentWithNotice(c templ.Component, text string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if c != nil {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		return noticePartial(text).Render(ctx, w)
	})
}
