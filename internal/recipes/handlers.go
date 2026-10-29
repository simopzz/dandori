package recipes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers serve the Recipe Library pages.
type Handlers struct {
	Service *Service
	Log     *slog.Logger
}

// Library lists the Recipe Library, or the Recipes a search found.
func (h Handlers) Library(w http.ResponseWriter, r *http.Request) {
	h.renderLibrary(w, r, RecipeLibrary{}, http.StatusOK)
}

func (h Handlers) renderLibrary(w http.ResponseWriter, r *http.Request, l RecipeLibrary, status int) {
	ctx := r.Context()
	var search Search
	l.Search, search = readSearch(r.URL.Query())
	if l.Search.Error != "" {
		status = http.StatusUnprocessableEntity
	}
	l.Searching = search.Any()
	var err error
	if l.Search.Appliances, err = h.Service.Appliances(ctx); err != nil {
		httpx.ServerError(w, h.Log, "load appliances", err)
		return
	}
	var found Results
	if l.Search.Error == "" {
		if found, err = h.Service.Search(ctx, search); err != nil {
			httpx.ServerError(w, h.Log, "search recipes", err)
			return
		}
	}
	l.Unranked, l.UnrankedFailed = found.Unranked, found.Failed
	for _, rec := range found.Recipes {
		l.Recipes = append(l.Recipes, RecipeSummary{
			ID:          rec.ID,
			Name:        rec.Name,
			Portions:    rec.Portions,
			PrepMinutes: rec.PrepMinutes,
			PerPortion:  PortionNutrition(rec.NutritionPerPortion()),
			Missing:     rec.Missing,
		})
	}
	templ.Handler(RecipeLibraryPage(l), templ.WithStatus(status)).ServeHTTP(w, r)
}

// Parse starts a Run parsing the pasted text, and sends the user to the
// draft it will produce.
func (h Handlers) Parse(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.PostForm.Get("text"))
	if text == "" {
		h.renderLibrary(w, r, RecipeLibrary{Error: "Paste a recipe to parse"}, http.StatusUnprocessableEntity)
		return
	}
	id, err := h.Service.StartParse(r.Context(), text)
	if err != nil {
		httpx.ServerError(w, h.Log, "start recipe parse", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/recipe-drafts/%d", id), http.StatusSeeOther)
}

// ShowDraft shows the draft a parse Run produced, ready to check and
// accept, or the Run's trace while there is none.
func (h Handlers) ShowDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	draft, found, err := h.Service.Draft(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, h.Log, "load draft", err)
		return
	}
	if !found {
		h.showDraftPending(w, r, id)
		return
	}
	known, err := h.Service.KnownIngredients(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load ingredients", err)
		return
	}
	_, f, _ := readRecipeForm(recipeFormValues(draft, known), known)
	templ.Handler(RecipeFormPage(draftForm(id, f))).ServeHTTP(w, r)
}

func (h Handlers) showDraftPending(w http.ResponseWriter, r *http.Request, id int64) {
	run, found, err := h.Service.ParseRun(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, h.Log, "load run", err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	templ.Handler(DraftPendingPage(DraftPending{Run: run, Page: fmt.Sprintf("/recipe-drafts/%d", id)})).ServeHTTP(w, r)
}

func draftForm(id int64, f RecipeForm) RecipeForm {
	f.Title = "Check the parsed Recipe"
	f.Action = fmt.Sprintf("/recipe-drafts/%d", id)
	f.Submit = "Accept Recipe"
	f.Discard = fmt.Sprintf("/recipe-drafts/%d/discard", id)
	return f
}

// AcceptDraft saves the draft, as the user corrected it, to the Recipe
// Library.
func (h Handlers) AcceptDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if found, err := h.Service.HasDraft(r.Context(), id); err != nil {
		httpx.ServerError(w, h.Log, "load draft", err)
		return
	} else if !found {
		http.NotFound(w, r)
		return
	}
	h.submit(w, r, func(f RecipeForm) RecipeForm { return draftForm(id, f) },
		func(ctx context.Context, rec domain.Recipe) (int64, error) {
			return h.Service.AcceptDraft(ctx, id, rec)
		})
}

func (h Handlers) DiscardDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := h.Service.Discard(r.Context(), id); err != nil && !errors.Is(err, ErrDraftGone) {
		httpx.ServerError(w, h.Log, "discard draft", err)
		return
	}
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}

// plannedDraftPath reads the Plan Proposal and the Recipe Draft of a
// planned draft's path, answering 404 if either isn't a number.
func plannedDraftPath(w http.ResponseWriter, r *http.Request) (proposal, id int64, ok bool) {
	proposal, err := strconv.ParseInt(r.PathValue("proposal"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, 0, false
	}
	id, ok = httpx.PathID(w, r)
	return proposal, id, ok
}

// proposalURL is the page of the Plan Proposal with id, where the user
// goes back to once a Recipe Draft it prepares is checked.
func proposalURL(id int64) string { return fmt.Sprintf("/plan-proposals/%d", id) }

func plannedDraftForm(proposal, id int64, f RecipeForm) RecipeForm {
	base := fmt.Sprintf("%s/recipe-drafts/%d", proposalURL(proposal), id)
	f.Title = "Check the drafted Recipe"
	f.Action = base
	f.Submit = "Accept Recipe"
	f.Discard = base + "/discard"
	return f
}

// ShowPlannedDraft shows a Recipe Draft the planner drafted for a Plan
// Proposal, ready to check and accept.
func (h Handlers) ShowPlannedDraft(w http.ResponseWriter, r *http.Request) {
	proposal, id, ok := plannedDraftPath(w, r)
	if !ok {
		return
	}
	draft, found, err := h.Service.DraftByID(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, h.Log, "load draft", err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	known, err := h.Service.KnownIngredients(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load ingredients", err)
		return
	}
	_, f, _ := readRecipeForm(recipeFormValues(draft, known), known)
	templ.Handler(RecipeFormPage(plannedDraftForm(proposal, id, f))).ServeHTTP(w, r)
}

// AcceptPlannedDraft saves a Recipe Draft the planner drafted, as the user
// corrected it, to the Recipe Library, and goes back to its Plan Proposal.
func (h Handlers) AcceptPlannedDraft(w http.ResponseWriter, r *http.Request) {
	proposal, id, ok := plannedDraftPath(w, r)
	if !ok {
		return
	}
	h.submitThen(w, r, func(f RecipeForm) RecipeForm { return plannedDraftForm(proposal, id, f) },
		func(ctx context.Context, rec domain.Recipe) (int64, error) {
			return h.Service.AcceptDraftByID(ctx, id, rec)
		},
		func(int64) string { return proposalURL(proposal) })
}

// DiscardPlannedDraft drops a Recipe Draft the planner drafted, and goes
// back to its Plan Proposal, which no longer applies.
func (h Handlers) DiscardPlannedDraft(w http.ResponseWriter, r *http.Request) {
	proposal, id, ok := plannedDraftPath(w, r)
	if !ok {
		return
	}
	if err := h.Service.DiscardByID(r.Context(), id); err != nil && !errors.Is(err, ErrDraftGone) {
		httpx.ServerError(w, h.Log, "discard draft", err)
		return
	}
	http.Redirect(w, r, proposalURL(proposal), http.StatusSeeOther)
}

func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.recipeFromPath(w, r)
	if !ok {
		return
	}
	h.renderRecipe(w, r, rec, "", http.StatusOK)
}

// renderRecipe shows rec, with problem saying why an action on it failed.
func (h Handlers) renderRecipe(w http.ResponseWriter, r *http.Request, rec Stored, problem string, status int) {
	view := RecipeView{
		Error:       problem,
		ID:          rec.ID,
		Name:        rec.Name,
		Portions:    rec.Portions,
		PrepMinutes: rec.PrepMinutes,
		Appliances:  rec.Appliances,
		Steps:       rec.Steps,
		PerPortion:  PortionNutrition(rec.NutritionPerPortion()),
	}
	for _, ri := range rec.Ingredients {
		view.Ingredients = append(view.Ingredients,
			fmt.Sprintf("%s %s %s", formatAmount(ri.Quantity), ri.Ingredient.Unit, ri.Ingredient.Name))
	}
	templ.Handler(RecipePage(view), templ.WithStatus(status)).ServeHTTP(w, r)
}

func (h Handlers) Edit(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.recipeFromPath(w, r)
	if !ok {
		return
	}
	known, err := h.Service.KnownIngredients(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load ingredients", err)
		return
	}
	_, f, _ := readRecipeForm(recipeFormValues(rec.Recipe, known), known)
	templ.Handler(RecipeFormPage(editForm(rec.ID, f))).ServeHTTP(w, r)
}

func editForm(id int64, f RecipeForm) RecipeForm {
	f.Title = "Edit Recipe"
	f.Action = fmt.Sprintf("/recipes/%d", id)
	f.Submit = "Save Recipe"
	return f
}

func (h Handlers) Update(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.recipeFromPath(w, r)
	if !ok {
		return
	}
	h.submit(w, r, func(f RecipeForm) RecipeForm { return editForm(rec.ID, f) },
		func(ctx context.Context, updated domain.Recipe) (int64, error) {
			return rec.ID, h.Service.Update(ctx, rec.ID, updated)
		})
}

func (h Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	switch err := h.Service.Delete(r.Context(), id); {
	case errors.Is(err, ErrHasPortions):
		rec, ok := h.recipeFromPath(w, r)
		if ok {
			h.renderRecipe(w, r, rec, "Portions of this Recipe are still kept, so it can't be deleted.", http.StatusConflict)
		}
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "delete recipe", err)
		return
	}
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}

// submit reads a submitted Recipe form and, when it is valid, saves it with
// save and shows the Recipe saved; otherwise it shows the form back, framed
// by frame.
func (h Handlers) submit(w http.ResponseWriter, r *http.Request,
	frame func(RecipeForm) RecipeForm,
	save func(context.Context, domain.Recipe) (int64, error),
) {
	h.submitThen(w, r, frame, save, func(id int64) string { return fmt.Sprintf("/recipes/%d", id) })
}

// submitThen is submit, going on to the page then gives for the Recipe
// saved.
func (h Handlers) submitThen(w http.ResponseWriter, r *http.Request,
	frame func(RecipeForm) RecipeForm,
	save func(context.Context, domain.Recipe) (int64, error),
	then func(id int64) string,
) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	known, err := h.Service.KnownIngredients(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load ingredients", err)
		return
	}
	rec, f, ok := readRecipeForm(r.PostForm, known)
	if !ok {
		templ.Handler(RecipeFormPage(frame(f)), templ.WithStatus(http.StatusUnprocessableEntity)).ServeHTTP(w, r)
		return
	}
	id, err := save(r.Context(), rec)
	if errors.Is(err, ErrDraftGone) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		// Keep what was typed rather than replacing the form with an error.
		h.Log.Error("store recipe", "err", err)
		f.Failed = true
		templ.Handler(RecipeFormPage(frame(f)), templ.WithStatus(http.StatusInternalServerError)).ServeHTTP(w, r)
		return
	}
	http.Redirect(w, r, then(id), http.StatusSeeOther)
}

// recipeFromPath loads the Recipe the path names, answering 404 if there
// is none.
func (h Handlers) recipeFromPath(w http.ResponseWriter, r *http.Request) (Stored, bool) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return Stored{}, false
	}
	rec, err := h.Service.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return Stored{}, false
	}
	if err != nil {
		httpx.ServerError(w, h.Log, "load recipe", err)
		return Stored{}, false
	}
	return rec, true
}
