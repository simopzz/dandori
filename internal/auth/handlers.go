package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/httpx"
)

// The cookies a session is kept in.
const (
	accessCookie  = "dandori_access"
	refreshCookie = "dandori_refresh"
)

// sessionAge is how long the browser keeps the cookies: as long as it
// allows. The session lasts until the User signs out.
const sessionAge = 400 * 24 * 60 * 60

// Handlers serve signing in and out, and keep every other page for the
// signed-in.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// SignedIn serves next only to a signed-in User, on a context carrying
// them; anyone else goes to sign-in.
func (h Handlers) SignedIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		access, err := r.Cookie(accessCookie)
		if err != nil {
			toSignIn(w, r)
			return
		}
		var tokens Tokens
		tokens.Access = access.Value
		if refresh, err := r.Cookie(refreshCookie); err == nil {
			tokens.Refresh = refresh.Value
		}
		userID, renewed, err := h.Service.Authenticate(r.Context(), tokens)
		if errors.Is(err, ErrSignedOut) {
			clearSession(w, r)
			toSignIn(w, r)
			return
		}
		if err != nil {
			httpx.ServerError(w, h.Log, "authenticate", err)
			return
		}
		if renewed != nil {
			keepSession(w, r, *renewed)
		}
		next.ServeHTTP(w, r.WithContext(db.WithUser(r.Context(), userID)))
	})
}

// toSignIn sends the browser to sign-in: htmx, which would swap the page
// into the one it is on, loads it in full.
func toSignIn(w http.ResponseWriter, r *http.Request) {
	if httpx.IsHTMX(r) {
		w.Header().Set("HX-Redirect", "/sign-in")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
}

func (h Handlers) ShowSignIn(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, signInPage(h.Clock.Now(), SignInForm{}))
}

func (h Handlers) SendLink(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	if !strings.Contains(email, "@") {
		h.render(w, r, http.StatusUnprocessableEntity, signInPage(h.Clock.Now(), SignInForm{Email: email, EmailErr: "Enter the email you were invited with."}))
		return
	}
	if err := h.Service.SendMagicLink(r.Context(), email, origin(r)+"/auth/confirm"); err != nil {
		h.Log.Error("send magic link", "err", err)
		h.render(w, r, http.StatusBadGateway, signInPage(h.Clock.Now(), SignInForm{Email: email, Failed: true}))
		return
	}
	http.Redirect(w, r, "/sign-in/sent", http.StatusSeeOther)
}

func (h Handlers) ShowSent(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, sentPage(h.Clock.Now()))
}

// ShowConfirm is where a magic link leads: Supabase's email templates
// link here with the token hash and the link's type.
func (h Handlers) ShowConfirm(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	h.render(w, r, http.StatusOK, confirmPage(h.Clock.Now(), q.Get("token_hash"), q.Get("type")))
}

// linkTypes are the links that sign in: a magic link, and an invitation.
var linkTypes = map[string]bool{"email": true, "magiclink": true, "invite": true}

func (h Handlers) Confirm(w http.ResponseWriter, r *http.Request) {
	tokenHash, kind := r.PostFormValue("token_hash"), r.PostFormValue("type")
	if tokenHash == "" || !linkTypes[kind] {
		h.render(w, r, http.StatusUnprocessableEntity, signInPage(h.Clock.Now(), SignInForm{BadLink: true}))
		return
	}
	tokens, err := h.Service.Redeem(r.Context(), tokenHash, kind)
	if errors.Is(err, ErrBadLink) {
		h.render(w, r, http.StatusUnprocessableEntity, signInPage(h.Clock.Now(), SignInForm{BadLink: true}))
		return
	}
	if err != nil {
		httpx.ServerError(w, h.Log, "redeem magic link", err)
		return
	}
	keepSession(w, r, tokens)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h Handlers) SignOut(w http.ResponseWriter, r *http.Request) {
	if access, err := r.Cookie(accessCookie); err == nil {
		// Signing out on this browser goes ahead even if Supabase can't
		// revoke the session.
		if err := h.Service.SignOut(r.Context(), Tokens{Access: access.Value}); err != nil {
			h.Log.Warn("revoke session", "err", err)
		}
	}
	clearSession(w, r)
	http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
}

func (h Handlers) render(w http.ResponseWriter, r *http.Request, status int, page templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := page.Render(r.Context(), w); err != nil {
		h.Log.Error("render", "path", r.URL.Path, "err", err)
	}
}

func keepSession(w http.ResponseWriter, r *http.Request, t Tokens) {
	http.SetCookie(w, cookie(r, accessCookie, t.Access, sessionAge))
	http.SetCookie(w, cookie(r, refreshCookie, t.Refresh, sessionAge))
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, cookie(r, accessCookie, "", -1))
	http.SetCookie(w, cookie(r, refreshCookie, "", -1))
}

func cookie(r *http.Request, name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure(r),
		SameSite: http.SameSiteLaxMode,
	}
}

// secure reports whether the browser reached the app over https: directly,
// or through Caddy, which says so in X-Forwarded-Proto.
func secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// origin is the app's address as the browser reached it, which a magic
// link leads back to. Supabase Auth sends links only to its allowed
// redirect URLs, so a forged Host can't lead one elsewhere.
func origin(r *http.Request) string {
	scheme := "http"
	if secure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
