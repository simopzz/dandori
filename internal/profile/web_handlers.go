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
	"github.com/simopzz/dandori/internal/ingredients"
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
	refusedTimes         httpx.Refusal = "times of day refused"
	refusedMealOut       httpx.Refusal = "Meal Out estimate refused"
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
	page, err := h.page(r, f, nil, nil, nil)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Profile", err)
		return
	}
	httpx.Render(w, r, h.Log, status, page)
}

// page is the Profile page with its Language section f, and its Targets, times-of-day
// and Meal Out sections as given, or as saved when not. Its exclusions and allergies and
// its Planning Brief, never refused, are as saved.
func (h Handlers) page(r *http.Request, f LanguageForm, targets *TargetsForm, times *TimesForm, mealOut *MealOutForm) (templ.Component, error) {
	ctx := r.Context()
	if targets == nil {
		t, set, err := h.Service.Targets(ctx)
		if err != nil {
			return nil, err
		}
		saved := shownTargets(t, set)
		targets = &saved
	}
	if times == nil {
		t, err := h.Service.SlotTimes(ctx)
		if err != nil {
			return nil, err
		}
		saved := shownTimes(ctx, t)
		times = &saved
	}
	if mealOut == nil {
		n, err := h.Service.MealOut(ctx)
		if err != nil {
			return nil, err
		}
		mealOut = &MealOutForm{Values: ingredients.ValuesOf(n)}
	}
	exclusions, allergies, err := h.Service.Foods(ctx)
	if err != nil {
		return nil, err
	}
	brief, err := h.Service.Brief(ctx)
	if err != nil {
		return nil, err
	}
	email, err := h.Email(ctx)
	return profilePage(h.Clock.Now(), email, f, *targets, *times, *mealOut, exclusions, allergies, brief), err
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
	_, set, err := h.Service.Targets(ctx)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Targets", err)
		return
	}
	f := TargetsForm{Values: typed, Errs: map[string]string{}, Set: set}
	for field, problem := range problems {
		f.Errs[field] = web.T(ctx, web.ProblemMessage(problem, numberProblems))
	}
	httpx.Refused(r, refusedTargets, f.Errs)
	page, err := h.page(r, LanguageForm{Language: web.LanguageOf(ctx)}, &f, nil, nil)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Profile", err)
		return
	}
	web.Answer{Status: http.StatusUnprocessableEntity, Page: page, Fragment: targetsForm(f)}.Write(w, r, h.Log)
}

// shownTimes is t as the times-of-day form shows it, as the Language writes a time.
func shownTimes(ctx context.Context, t domain.SlotTimes) TimesForm {
	f := TimesForm{Values: map[string]string{}}
	for k, at := range t {
		f.Values[string(k)] = web.TimeOfDay(ctx, at)
	}
	return f
}

// SaveTimes saves the times of day posted, answering with the times-of-day section saved
// or refused as typed; a plain post lands on the Profile page.
func (h Handlers) SaveTimes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typed := map[string]string{}
	for _, k := range domain.SlotKinds {
		typed[string(k)] = r.PostFormValue(string(k))
	}
	t, problems, err := h.Service.SetSlotTimes(ctx, typed)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "save the User's times of day", err)
		return
	}
	if len(problems) == 0 {
		web.Answer{RedirectTo: "/profile", Notice: web.T(ctx, msgSaved), Fragment: timesForm(shownTimes(ctx, t))}.Write(w, r, h.Log)
		return
	}
	f := TimesForm{Values: typed, Errs: map[string]string{}}
	for field, problem := range problems {
		f.Errs[field] = web.T(ctx, web.ProblemMessage(problem, nil))
		if problem == domain.NotAfterTheOneBefore {
			f.Errs[field] = web.Tf(ctx, msgAfter, web.T(ctx, web.SlotKindNames[domain.SlotKind(field).Before()]))
		}
	}
	httpx.Refused(r, refusedTimes, f.Errs)
	page, err := h.page(r, LanguageForm{Language: web.LanguageOf(ctx)}, nil, &f, nil)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Profile", err)
		return
	}
	web.Answer{Status: http.StatusUnprocessableEntity, Page: page, Fragment: timesForm(f)}.Write(w, r, h.Log)
}

// SaveMealOut saves the Meal Out estimate posted, answering with the Meal Out section
// saved or refused as typed; a plain post lands on the Profile page.
func (h Handlers) SaveMealOut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typed := ingredients.PostedNutrition(r)
	n, problems, err := h.Service.SetMealOut(ctx, typed)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "save the User's Meal Out estimate", err)
		return
	}
	if len(problems) == 0 {
		web.Answer{RedirectTo: "/profile", Notice: web.T(ctx, msgSaved), Fragment: mealOutForm(MealOutForm{Values: ingredients.ValuesOf(n)})}.Write(w, r, h.Log)
		return
	}
	f := MealOutForm{Values: typed, Errs: map[string]string{}}
	for field, problem := range problems {
		f.Errs[field] = web.T(ctx, web.ProblemMessage(problem, numberProblems))
	}
	httpx.Refused(r, refusedMealOut, f.Errs)
	page, err := h.page(r, LanguageForm{Language: web.LanguageOf(ctx)}, nil, nil, &f)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Profile", err)
		return
	}
	web.Answer{Status: http.StatusUnprocessableEntity, Page: page, Fragment: mealOutForm(f)}.Write(w, r, h.Log)
}

// SaveFoods saves the exclusions and allergies posted, answering with their section as
// saved, normalised; a plain post lands on the Profile page.
func (h Handlers) SaveFoods(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	exclusions, allergies, err := h.Service.SetFoods(ctx, r.PostFormValue("exclusions"), r.PostFormValue("allergies"))
	if err != nil {
		httpx.ServerError(w, r, h.Log, "save the User's exclusions and allergies", err)
		return
	}
	web.Answer{RedirectTo: "/profile", Notice: web.T(ctx, msgSaved), Fragment: foodsForm(exclusions, allergies)}.Write(w, r, h.Log)
}

// SaveBrief saves the Planning Brief posted, answering with its section as saved; a plain
// post lands on the Profile page.
func (h Handlers) SaveBrief(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	brief, err := h.Service.SetBrief(ctx, r.PostFormValue("brief"))
	if err != nil {
		httpx.ServerError(w, r, h.Log, "save the User's Planning Brief", err)
		return
	}
	web.Answer{RedirectTo: "/profile", Notice: web.T(ctx, msgSaved), Fragment: briefForm(brief)}.Write(w, r, h.Log)
}
