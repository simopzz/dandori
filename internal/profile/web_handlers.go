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

// LanguageChosen serves `next` in the signed-in User's Language, once they have chosen
// one; until then every page asks for it first.
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
	forms, err := h.asSaved(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Profile", err)
		return
	}
	page, err := h.page(r.Context(), forms)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Profile", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, page)
}

func (h Handlers) ChangeLanguage(w http.ResponseWriter, r *http.Request) {
	l, ok := domain.ParseLanguage(r.PostFormValue("language"))
	if !ok {
		h.refuseLanguage(w, r, refusedNoLanguage, msgChooseOne)
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

// setLanguage sets the User's Language to `l` and remembers it in this browser, for the
// pages after signing out. When it can't, it answers and is false.
func (h Handlers) setLanguage(w http.ResponseWriter, r *http.Request, l domain.Language) bool {
	err := h.Service.SetLanguage(r.Context(), l)
	if errors.Is(err, ErrLanguageStays) {
		h.refuseLanguage(w, r, refusedLanguageStays, msgLanguageStays)
		return false
	}
	if errors.Is(err, ErrParsing) {
		h.refuseLanguage(w, r, refusedParsing, msgParsing)
		return false
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "set the User's Language", err)
		return false
	}
	web.KeepLanguage(w, r, l)
	return true
}

// refuseLanguage refuses a change of Language, keeping the User's, and says `why`.
func (h Handlers) refuseLanguage(w http.ResponseWriter, r *http.Request, why httpx.Refusal, msg web.Message) {
	h.answerRefused(w, r, why, nil, func(f *profileForms) templ.Component {
		f.Language.Err = msg
		return nil
	})
}

// asSaved is each section of the Profile page as saved, in the Language of the page.
func (h Handlers) asSaved(ctx context.Context) (f profileForms, err error) {
	f.Language = LanguageForm{Language: web.LanguageOf(ctx)}
	times, err := h.Service.SlotTimes(ctx)
	if err != nil {
		return f, err
	}
	f.Times = shownTimes(ctx, times)
	targets, set, err := h.Service.Targets(ctx)
	if err != nil {
		return f, err
	}
	f.Targets = shownTargets(targets, set)
	mealOut, err := h.Service.MealOut(ctx)
	if err != nil {
		return f, err
	}
	f.MealOut = MealOutForm{Values: ingredients.ValuesOf(mealOut)}
	if f.Foods.Exclusions, f.Foods.Allergies, err = h.Service.Foods(ctx); err != nil {
		return f, err
	}
	if f.Appliances, err = h.Service.Appliances(ctx); err != nil {
		return f, err
	}
	f.Brief, err = h.Service.Brief(ctx)
	return f, err
}

// page is the Profile page showing `forms`.
func (h Handlers) page(ctx context.Context, forms profileForms) (templ.Component, error) {
	email, err := h.Email(ctx)
	return profilePage(h.Clock.Now(), email, forms), err
}

// answerSaved answers a `section` saved, with a Notice: as htmx the `section`, shown as
// saved, and as a plain post the Profile page it lands on.
func (h Handlers) answerSaved(w http.ResponseWriter, r *http.Request, section templ.Component) {
	web.Answer{RedirectTo: "/profile", Notice: web.T(r.Context(), msgSaved), Fragment: section}.Write(w, r, h.Log)
}

// answerRefused answers a section refused, which saved nothing, with 422 and `why`, by
// field in `errs`: the Profile page with each section as saved but the one `show` sets as
// typed, or as htmx the section `show` returns. The Language has no section of its own to
// swap, so a refused one answers with the page.
func (h Handlers) answerRefused(w http.ResponseWriter, r *http.Request, why httpx.Refusal, errs map[string]string, show func(*profileForms) templ.Component) {
	httpx.Refused(r, why, errs)
	forms, err := h.asSaved(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Profile", err)
		return
	}
	section := show(&forms)
	page, err := h.page(r.Context(), forms)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Profile", err)
		return
	}
	web.Answer{Status: http.StatusUnprocessableEntity, Page: page, Fragment: section}.Write(w, r, h.Log)
}

// shownTargets is `t` as the Targets form shows it, only its tolerance until it is set.
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
		h.answerSaved(w, r, targetsForm(shownTargets(t, true)))
		return
	}
	errs := map[string]string{}
	for field, problem := range problems {
		errs[field] = web.T(ctx, web.ProblemMessage(problem, numberProblems))
	}
	h.answerRefused(w, r, refusedTargets, errs, func(f *profileForms) templ.Component {
		f.Targets = TargetsForm{Values: typed, Errs: errs, Set: f.Targets.Set}
		return targetsForm(f.Targets)
	})
}

// shownTimes is `t` as the times-of-day form shows it, as the Language writes a time.
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
		h.answerSaved(w, r, timesForm(shownTimes(ctx, t)))
		return
	}
	errs := map[string]string{}
	for field, problem := range problems {
		errs[field] = web.T(ctx, web.ProblemMessage(problem, nil))
		if problem == domain.NotAfterTheOneBefore {
			errs[field] = web.Tf(ctx, msgAfter, web.T(ctx, web.SlotKindNames[domain.SlotKind(field).Before()]))
		}
	}
	h.answerRefused(w, r, refusedTimes, errs, func(f *profileForms) templ.Component {
		f.Times = TimesForm{Values: typed, Errs: errs}
		return timesForm(f.Times)
	})
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
		h.answerSaved(w, r, mealOutForm(MealOutForm{Values: ingredients.ValuesOf(n)}))
		return
	}
	errs := map[string]string{}
	for field, problem := range problems {
		errs[field] = web.T(ctx, web.ProblemMessage(problem, numberProblems))
	}
	h.answerRefused(w, r, refusedMealOut, errs, func(f *profileForms) templ.Component {
		f.MealOut = MealOutForm{Values: typed, Errs: errs}
		return mealOutForm(f.MealOut)
	})
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
	h.answerSaved(w, r, foodsForm(FoodsForm{Exclusions: exclusions, Allergies: allergies}))
}

// SaveAppliances saves the kinds of appliance ticked, each with its note, answering with
// their section as saved; a plain post lands on the Profile page.
func (h Handlers) SaveAppliances(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	notes := map[domain.ApplianceKind]string{}
	for _, k := range domain.ApplianceKinds {
		notes[k] = r.PostFormValue(noteField(k))
	}
	owned, err := h.Service.SetAppliances(ctx, r.PostForm["appliances"], notes)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "save the User's appliances", err)
		return
	}
	h.answerSaved(w, r, appliancesForm(owned))
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
	h.answerSaved(w, r, briefForm(brief))
}
