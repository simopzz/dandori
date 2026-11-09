package recipes

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/web"
)

// Handlers serve the Recipe Library and the Recipe form.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// Library is the Recipe Library page.
func (h Handlers) Library(w http.ResponseWriter, r *http.Request) {
	recipes, err := h.Service.List(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Recipes", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, library(h.Clock.Now(), recipes))
}

// Show is the Recipe's own page.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	rec, err := h.Service.Find(r.Context(), id)
	if errors.Is(err, ErrNoRecipe) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the Recipe", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, recipePage(h.Clock.Now(), rec))
}

// ShowNew is the form to write a Recipe by hand.
func (h Handlers) ShowNew(w http.ResponseWriter, r *http.Request) {
	known, err := h.Service.KnownIngredients(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, formPage(h.Clock.Now(), blankForm(known)))
}

// ShowRow is what a row of the form shows beneath its name as the User leaves it: the
// unit of a known Ingredient, or the fields confirming a new one, keeping what was typed.
func (h Handlers) ShowRow(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	i, ok := rowIndex(query)
	if !ok {
		http.Error(w, "no row", http.StatusBadRequest)
		return
	}
	known, err := h.Service.KnownIngredients(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
		return
	}
	_, row, _ := readRow(r.Context(), query, i, byName(web.LanguageOf(r.Context()), known))
	row.Confirming = false
	httpx.Render(w, r, h.Log, http.StatusOK, rowDetails(i, row))
}

// Add saves the Recipe posted, with its new Ingredients. A form that adds a row, for a
// browser without the form's script, comes back with one more and saves nothing. One the
// form refuses comes back as typed with why, a row of an unknown name opened to confirm.
func (h Handlers) Add(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	known, err := h.Service.KnownIngredients(ctx)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
		return
	}
	rec, f, ok := readForm(ctx, r.PostForm, known)
	switch r.PostForm.Get("add") {
	case "ingredients":
		httpx.Render(w, r, h.Log, http.StatusOK, formPage(h.Clock.Now(), withRow(f)))
		return
	case "steps":
		httpx.Render(w, r, h.Log, http.StatusOK, formPage(h.Clock.Now(), withStep(f)))
		return
	}
	refused := func(f RecipeForm) {
		web.Answer{Status: http.StatusUnprocessableEntity, Page: formPage(h.Clock.Now(), f)}.Write(w, r, h.Log)
	}
	if !ok {
		refused(f)
		return
	}
	id, err := h.Service.Add(ctx, web.LanguageOf(ctx), rec)
	if taken, ok := errors.AsType[*NameTakenError](err); ok {
		f.Errs["name"], f.TakenID, f.TakenName = web.T(ctx, msgNameTaken), taken.ID, taken.Existing
		refused(f)
		return
	}
	if _, ok := errors.AsType[*ingredients.NameTakenError](err); ok {
		if known, err = h.Service.KnownIngredients(ctx); err != nil {
			httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
			return
		}
		_, f, _ = readForm(ctx, r.PostForm, known)
		refused(f)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add a Recipe", err)
		return
	}
	web.Answer{Notice: web.Tf(ctx, msgAdded, rec.Name), RedirectTo: recipeURL(id)}.Write(w, r, h.Log)
}

// withRow is f with one more Ingredient row, showing nothing refused yet.
func withRow(f RecipeForm) RecipeForm {
	f = unrefused(f)
	f.Ingredients = append(f.Ingredients, IngredientRow{})
	return f
}

// withStep is f with one more step, showing nothing refused yet.
func withStep(f RecipeForm) RecipeForm {
	f = unrefused(f)
	f.Steps = append(f.Steps, "")
	return f
}

// unrefused is f before anything was refused.
func unrefused(f RecipeForm) RecipeForm {
	f.Errs, f.Confirm = map[string]string{}, false
	for i := range f.Ingredients {
		f.Ingredients[i].Errs = nil
	}
	return f
}

// recipeURL is the page of the Recipe id.
func recipeURL(id int64) string { return "/recipes/" + strconv.FormatInt(id, 10) }
