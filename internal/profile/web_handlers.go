package profile

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// Handlers serve the Language choice at the first sign-in and the Profile page, and keep
// every other page for Users who have chosen.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
	// Email is the signed-in User's email, which the Profile shows.
	Email func(context.Context) (string, error)
}

// LanguageChosen serves next in the signed-in User's Language, once they have chosen one;
// until then every page asks for it first.
func (h Handlers) LanguageChosen(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l, ok, err := h.Service.Language(r.Context())
		if err != nil {
			httpx.ServerError(w, r, h.Log, "load the User's Language", err)
			return
		}
		if !ok {
			http.Redirect(w, r, "/language", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(web.WithLanguage(r.Context(), l)))
	})
}

// ShowChoice asks a signed-in User for their Language, with the one the pages were
// switched to chosen. One who has chosen goes to Today.
func (h Handlers) ShowChoice(w http.ResponseWriter, r *http.Request) {
	if h.goneToTodayIfChosen(w, r) {
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, choicePage(h.Clock.Now(), LanguageForm{Language: web.LanguageOf(r.Context())}))
}

// Choose makes the first choice of Language; changing it later is the Profile's.
func (h Handlers) Choose(w http.ResponseWriter, r *http.Request) {
	if h.goneToTodayIfChosen(w, r) {
		return
	}
	l, ok := domain.ParseLanguage(r.PostFormValue("language"))
	if !ok {
		httpx.Refused(r, "no Language chosen", nil)
		httpx.Render(w, r, h.Log, http.StatusUnprocessableEntity, choicePage(h.Clock.Now(), LanguageForm{Language: web.LanguageOf(r.Context()), Err: msgChooseOne}))
		return
	}
	if !h.setLanguage(w, r, l) {
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	h.renderProfile(w, r, http.StatusOK, LanguageForm{Language: web.LanguageOf(r.Context())})
}

func (h Handlers) ChangeLanguage(w http.ResponseWriter, r *http.Request) {
	l, ok := domain.ParseLanguage(r.PostFormValue("language"))
	if !ok {
		httpx.Refused(r, "no Language chosen", nil)
		h.renderProfile(w, r, http.StatusUnprocessableEntity, LanguageForm{Language: web.LanguageOf(r.Context()), Err: msgChooseOne})
		return
	}
	if !h.setLanguage(w, r, l) {
		return
	}
	web.Answer{RedirectTo: "/profile", Notice: web.T(web.WithLanguage(r.Context(), l), msgSaved)}.Write(w, r, h.Log)
}

// goneToTodayIfChosen sends a User who has chosen their Language to Today, and reports
// whether it did, or answered with an error.
func (h Handlers) goneToTodayIfChosen(w http.ResponseWriter, r *http.Request) bool {
	_, ok, err := h.Service.Language(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Language", err)
		return true
	}
	if ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
	return ok
}

// setLanguage sets the User's Language to l and remembers it in this browser, for the
// pages after signing out. When it can't, it answers and is false.
func (h Handlers) setLanguage(w http.ResponseWriter, r *http.Request, l domain.Language) bool {
	err := h.Service.SetLanguage(r.Context(), l)
	if errors.Is(err, ErrLanguageStays) {
		httpx.Refused(r, "Language stays: the User has written Food Terms in it", nil)
		h.renderProfile(w, r, http.StatusUnprocessableEntity, LanguageForm{Language: web.LanguageOf(r.Context()), Err: msgLanguageStays})
		return false
	}
	if errors.Is(err, ErrParsing) {
		httpx.Refused(r, "Language stays: a parse Run is going", nil)
		h.renderProfile(w, r, http.StatusUnprocessableEntity, LanguageForm{Language: web.LanguageOf(r.Context()), Err: msgParsing})
		return false
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "set the User's Language", err)
		return false
	}
	web.KeepLanguage(w, r, l)
	return true
}

func (h Handlers) renderProfile(w http.ResponseWriter, r *http.Request, status int, f LanguageForm) {
	email, err := h.Email(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's email", err)
		return
	}
	httpx.Render(w, r, h.Log, status, profilePage(h.Clock.Now(), email, f))
}
