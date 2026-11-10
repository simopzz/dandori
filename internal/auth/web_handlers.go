package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

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

// Handlers serve signing in and out, and keep every other page for the signed-in.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// SignedIn serves next only to a signed-in User, on a context carrying
// them; anyone else goes to sign-in.
func (h Handlers) SignedIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens, ok := cookieTokens(r)
		if !ok {
			http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
			return
		}
		userID, renewed, err := h.Service.Authenticate(r.Context(), tokens)
		if errors.Is(err, ErrSignedOut) {
			clearSession(w, r)
			http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
			return
		}
		if err != nil {
			httpx.ServerError(w, r, h.Log, "authenticate", err)
			return
		}
		if renewed != nil {
			keepSession(w, r, *renewed)
		}
		next.ServeHTTP(w, r.WithContext(db.WithUser(r.Context(), userID)))
	})
}

func (h Handlers) ShowSignIn(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, h.Log, http.StatusOK, signInPage(h.Clock.Now(), SignInForm{}))
}

func (h Handlers) SendLink(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	if !strings.Contains(email, "@") {
		httpx.Render(w, r, h.Log, http.StatusUnprocessableEntity, signInPage(h.Clock.Now(), SignInForm{Email: email, EmailErr: msgNotAnEmail}))
		return
	}
	if err := h.Service.SendMagicLink(r.Context(), email, origin(r)+"/auth/confirm"); err != nil {
		h.Log.ErrorContext(r.Context(), "send magic link", "err", err)
		httpx.Render(w, r, h.Log, http.StatusBadGateway, signInPage(h.Clock.Now(), SignInForm{Email: email, Failed: true}))
		return
	}
	http.Redirect(w, r, "/sign-in/sent", http.StatusSeeOther)
}

func (h Handlers) ShowSent(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, h.Log, http.StatusOK, sentPage(h.Clock.Now()))
}

// ShowConfirm is where a magic link leads: Supabase's email templates
// link here with the token hash and the link's type.
func (h Handlers) ShowConfirm(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	httpx.Render(w, r, h.Log, http.StatusOK, confirmPage(r.URL, h.Clock.Now(), q.Get("token_hash"), q.Get("type")))
}

// linkTypes are the links that sign in: a magic link, an invitation, and
// the signup confirmation an invited User who hasn't confirmed yet is sent
// in place of a magic link.
var linkTypes = map[string]bool{"email": true, "invite": true, "signup": true}

func (h Handlers) Confirm(w http.ResponseWriter, r *http.Request) {
	tokenHash, kind := r.PostFormValue("token_hash"), r.PostFormValue("type")
	tokens, err := Tokens{}, ErrBadLink
	if tokenHash != "" && linkTypes[kind] {
		tokens, err = h.Service.Redeem(r.Context(), tokenHash, kind)
	}
	if errors.Is(err, ErrBadLink) {
		httpx.Render(w, r, h.Log, http.StatusUnprocessableEntity, signInPage(h.Clock.Now(), SignInForm{BadLink: true}))
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "redeem magic link", err)
		return
	}
	keepSession(w, r, tokens)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h Handlers) SignOut(w http.ResponseWriter, r *http.Request) {
	if tokens, ok := cookieTokens(r); ok {
		// Signing out on this browser goes ahead even if Supabase can't
		// revoke the session.
		if err := h.Service.SignOut(r.Context(), tokens); err != nil {
			h.Log.WarnContext(r.Context(), "revoke session", "err", err)
		}
	}
	clearSession(w, r)
	http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
}

// cookieTokens reads the Tokens the cookies keep, if any.
func cookieTokens(r *http.Request) (Tokens, bool) {
	access, err := r.Cookie(accessCookie)
	if err != nil {
		return Tokens{}, false
	}
	t := Tokens{Access: access.Value}
	if refresh, err := r.Cookie(refreshCookie); err == nil {
		t.Refresh = refresh.Value
	}
	return t, true
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
		Secure:   httpx.Secure(r),
		SameSite: http.SameSiteLaxMode,
	}
}

// origin is the app's address as the browser reached it, which a magic
// link leads back to. Supabase Auth sends links only to its allowed
// redirect URLs, so a forged Host can't lead one elsewhere.
func origin(r *http.Request) string {
	scheme := "http"
	if httpx.Secure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
