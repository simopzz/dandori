package profile

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/a-h/templ"

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

// Why a Language is refused.
const (
	refusedNoLanguage    httpx.Refusal = "no Language chosen"
	refusedLanguageStays httpx.Refusal = "Language stays: the User has written Food Terms in it"
	refusedParsing       httpx.Refusal = "Language stays: a parse Run is going"
	refusedTargets       httpx.Refusal = "Targets refused"
)

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
		httpx.Refused(r, refusedNoLanguage, nil)
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
		httpx.Refused(r, refusedNoLanguage, nil)
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
		httpx.Refused(r, refusedLanguageStays, nil)
		h.renderProfile(w, r, http.StatusUnprocessableEntity, LanguageForm{Language: web.LanguageOf(r.Context()), Err: msgLanguageStays})
		return false
	}
	if errors.Is(err, ErrParsing) {
		httpx.Refused(r, refusedParsing, nil)
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
	targets, err := h.savedTargets(r)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Targets", err)
		return
	}
	page, err := h.page(r, f, targets)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's email", err)
		return
	}
	httpx.Render(w, r, h.Log, status, page)
}

// page is the Profile page, its Language section f and its Targets section targets.
func (h Handlers) page(r *http.Request, f LanguageForm, targets TargetsForm) (templ.Component, error) {
	email, err := h.Email(r.Context())
	return profilePage(h.Clock.Now(), email, f, targets), err
}

// savedTargets is the signed-in User's Targets as saved.
func (h Handlers) savedTargets(r *http.Request) (TargetsForm, error) {
	t, set, err := h.Service.Targets(r.Context())
	return shownTargets(t, set), err
}

// shownTargets is t as the Targets form shows it, only its tolerance until it is set.
func shownTargets(t domain.Targets, set bool) TargetsForm {
	f := TargetsForm{Set: set, Values: map[string]string{}}
	for field, n := range t.ByField() {
		if set || field == domain.TargetTolerance {
			f.Values[field] = strconv.Itoa(n)
		}
	}
	return f
}

// SaveTargets saves the Targets posted, answering with the Targets section saved or
// refused as typed; a plain post lands on the Profile page.
func (h Handlers) SaveTargets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typed := map[string]string{}
	for _, field := range domain.TargetFields {
		typed[field] = r.PostFormValue(field)
	}
	t, problems, err := h.Service.SetTargets(ctx, typed)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "save the User's Targets", err)
		return
	}
	if len(problems) == 0 {
		web.Answer{RedirectTo: "/profile", Notice: web.T(ctx, msgSaved), Fragment: targetsForm(shownTargets(t, true))}.Write(w, r, h.Log)
		return
	}
	saved, err := h.savedTargets(r)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Targets", err)
		return
	}
	f := TargetsForm{Values: typed, Errs: map[string]string{}, Set: saved.Set}
	for field, problem := range problems {
		f.Errs[field] = web.T(ctx, web.ProblemMessage(problem, targetProblems))
	}
	httpx.Refused(r, refusedTargets, f.Errs)
	page, err := h.page(r, LanguageForm{Language: web.LanguageOf(ctx)}, f)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's email", err)
		return
	}
	web.Answer{Status: http.StatusUnprocessableEntity, Page: page, Fragment: targetsForm(f)}.Write(w, r, h.Log)
}
