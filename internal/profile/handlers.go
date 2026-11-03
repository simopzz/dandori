package profile

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// Handlers serve the Language choice at the first sign-in and the Profile
// page, and keep every other page for Users who have chosen.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
	// Email is the signed-in User's email, which the Profile shows.
	Email func(context.Context) (string, error)
}

// LanguageChosen serves next in the signed-in User's Language, once they
// have chosen one; until then every page asks for it first.
func (h Handlers) LanguageChosen(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l, ok, err := h.Service.Language(r.Context())
		if err != nil {
			httpx.ServerError(w, h.Log, "load the User's Language", err)
			return
		}
		if !ok {
			http.Redirect(w, r, "/language", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(web.WithLanguage(r.Context(), l)))
	})
}

// ShowChoice asks a signed-in User for their Language, with the one the
// pages were switched to chosen. One who has chosen goes Home.
func (h Handlers) ShowChoice(w http.ResponseWriter, r *http.Request) {
	_, ok, err := h.Service.Language(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load the User's Language", err)
		return
	}
	if ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	h.render(w, r, http.StatusOK, choicePage(h.Clock.Now(), LanguageForm{Language: web.LanguageOf(r.Context())}))
}

func (h Handlers) Choose(w http.ResponseWriter, r *http.Request) {
	l, ok := domain.ParseLanguage(r.PostFormValue("language"))
	if !ok {
		h.render(w, r, http.StatusUnprocessableEntity, choicePage(h.Clock.Now(), LanguageForm{Err: msgChooseOne}))
		return
	}
	if !h.setLanguage(w, r, l) {
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	f := LanguageForm{Language: web.LanguageOf(r.Context()), Saved: r.URL.Query().Has("saved")}
	h.renderProfile(w, r, http.StatusOK, f)
}

func (h Handlers) ChangeLanguage(w http.ResponseWriter, r *http.Request) {
	l, ok := domain.ParseLanguage(r.PostFormValue("language"))
	if !ok {
		h.renderProfile(w, r, http.StatusUnprocessableEntity, LanguageForm{Err: msgChooseOne})
		return
	}
	if !h.setLanguage(w, r, l) {
		return
	}
	http.Redirect(w, r, "/profile?saved", http.StatusSeeOther)
}

// setLanguage sets the User's Language to l and remembers it in this
// browser, for the pages after signing out. When it can't, it answers
// and is false.
func (h Handlers) setLanguage(w http.ResponseWriter, r *http.Request, l domain.Language) bool {
	err := h.Service.SetLanguage(r.Context(), l)
	if errors.Is(err, ErrLanguageStays) {
		h.renderProfile(w, r, http.StatusUnprocessableEntity, LanguageForm{Language: web.LanguageOf(r.Context()), Err: msgLanguageStays})
		return false
	}
	if err != nil {
		httpx.ServerError(w, h.Log, "set the User's Language", err)
		return false
	}
	web.KeepLanguage(w, r, l)
	return true
}

func (h Handlers) renderProfile(w http.ResponseWriter, r *http.Request, status int, f LanguageForm) {
	email, err := h.Email(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load the User's email", err)
		return
	}
	h.render(w, r, status, profilePage(h.Clock.Now(), email, f))
}

func (h Handlers) render(w http.ResponseWriter, r *http.Request, status int, page templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := page.Render(r.Context(), w); err != nil {
		h.Log.Error("render", "path", r.URL.Path, "err", err)
	}
}
