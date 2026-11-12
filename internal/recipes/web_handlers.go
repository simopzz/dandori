package recipes

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

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

// Library is the Recipe Library page, searched for its query q.
func (h Handlers) Library(w http.ResponseWriter, r *http.Request) {
	h.library(w, r, http.StatusOK, PasteForm{}, r.URL.Query().Get("q"))
}

// library answers with the Recipe Library page searched for q, its paste box as in
// paste, with status.
func (h Handlers) library(w http.ResponseWriter, r *http.Request, status int, paste PasteForm, q string) {
	ctx := r.Context()
	drafts, err := h.Service.Drafts(ctx)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Recipe Drafts", err)
		return
	}
	recipes, err := h.Service.Search(ctx, q)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "search the Recipes", err)
		return
	}
	web.Answer{Status: status, Page: library(h.Clock.Now(), paste, drafts, recipes, q)}.Write(w, r, h.Log)
}

// pasteLimit is how many characters a paste may have, trimmed.
const pasteLimit = 20000

// Paste starts a parse Run reading the pasted text into a Recipe Draft, and goes to the
// Run. A blank paste, or one over pasteLimit characters, is refused on its field.
func (h Handlers) Paste(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.PostForm.Get("text"))
	if text == "" {
		paste := PasteForm{Text: r.PostForm.Get("text"), Err: web.T(ctx, msgPasteRequired)}
		httpx.Refused(r, "nothing pasted", nil)
		h.library(w, r, http.StatusUnprocessableEntity, paste, "")
		return
	}
	if n := utf8.RuneCountInString(text); n > pasteLimit {
		p := message.NewPrinter(language.Make(string(web.LanguageOf(ctx))))
		paste := PasteForm{Text: r.PostForm.Get("text"), Err: web.Tf(ctx, msgPasteTooLong, p.Sprint(pasteLimit), p.Sprint(n))}
		httpx.Refused(r, "pasted text too long", nil)
		h.library(w, r, http.StatusUnprocessableEntity, paste, "")
		return
	}
	id, err := h.Service.StartParse(ctx, web.LanguageOf(ctx), text)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a parse Run", err)
		return
	}
	web.Answer{RedirectTo: "/runs/" + strconv.FormatInt(id, 10)}.Write(w, r, h.Log)
}

// ShowDraft is the Recipe form to check a Recipe Draft with, holding what was parsed.
func (h Handlers) ShowDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rec, err := h.Service.FindDraft(ctx, web.LanguageOf(ctx), id)
	if errors.Is(err, ErrNoDraft) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the Recipe Draft", err)
		return
	}
	known, err := h.Service.KnownIngredients(ctx)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, formPage(h.Clock.Now(), draftForm(ctx, id, rec, known)))
}

// Accept adds the Recipe posted, as checked from the Recipe Draft in the path, to the
// Recipe Library, and deletes the Recipe Draft.
func (h Handlers) Accept(w http.ResponseWriter, r *http.Request) {
	if id, ok := httpx.PathID(w, r); ok {
		h.save(w, r, 0, id)
	}
}

// Discard deletes the Recipe Draft in the path.
func (h Handlers) Discard(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	name, err := h.Service.Discard(ctx, id)
	if errors.Is(err, ErrNoDraft) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "discard a Recipe Draft", err)
		return
	}
	web.Answer{Notice: web.Tf(ctx, msgDiscarded, name), RedirectTo: "/recipes"}.Write(w, r, h.Log)
}

// DraftPage is the page of the Recipe Draft the signed-in User's parse Run runID left, or
// "" if it left none: a Run's page links to it.
func (h Handlers) DraftPage(ctx context.Context, runID int64) (string, error) {
	id, err := h.Service.DraftOf(ctx, runID)
	if errors.Is(err, ErrNoDraft) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return draftURL(id), nil
}

// ShowList is the list of Recipes alone, searched for its query q, which the search swaps
// in as the User types.
func (h Handlers) ShowList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	recipes, err := h.Service.Search(r.Context(), q)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "search the Recipes", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, recipeList(recipes, q))
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
// A row picking the Ingredient suggested for it is shown whole, its name changed.
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
	row.Confirming, row.Errs = false, nil // nothing is refused before a save
	if picking(query) {                   // the name changed, so the whole row
		httpx.Render(w, r, h.Log, http.StatusOK, ingredientRow(i, row))
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, rowDetails(i, row))
}

// ShowBlankRow is one more empty Ingredient row, numbered after the ones it is told of.
func (h Handlers) ShowBlankRow(w http.ResponseWriter, r *http.Request) {
	n := 0
	for r.URL.Query().Has(rowField("name", n)) {
		n++
	}
	httpx.Render(w, r, h.Log, http.StatusOK, ingredientRow(n, IngredientRow{Focus: true}))
}

// ShowBlankStep is one more empty step.
func (h Handlers) ShowBlankStep(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, h.Log, http.StatusOK, stepRow("", true))
}

// ShowEdit is the form to edit a Recipe with.
func (h Handlers) ShowEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rec, err := h.Service.Find(ctx, id)
	if errors.Is(err, ErrNoRecipe) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the Recipe", err)
		return
	}
	known, err := h.Service.KnownIngredients(ctx)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, formPage(h.Clock.Now(), formOf(ctx, rec, known)))
}

// Add saves the Recipe posted as a new one.
func (h Handlers) Add(w http.ResponseWriter, r *http.Request) { h.save(w, r, 0, 0) }

// Edit saves the Recipe posted over the Recipe in the path.
func (h Handlers) Edit(w http.ResponseWriter, r *http.Request) {
	if id, ok := httpx.PathID(w, r); ok {
		h.save(w, r, id, 0)
	}
}

// Delete deletes a Recipe, its Ingredients staying in the User's kitchen.
func (h Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	name, err := h.Service.Delete(ctx, id)
	if errors.Is(err, ErrNoRecipe) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "delete a Recipe", err)
		return
	}
	web.Answer{Notice: web.Tf(ctx, msgDeleted, name), RedirectTo: "/recipes"}.Write(w, r, h.Log)
}

// save saves the Recipe posted, with its new Ingredients, over the Recipe id, or as a new
// one if id is 0, accepting the Recipe Draft draftID unless that is 0. A form that adds a
// row, for a browser without htmx, comes back with one more and saves nothing. One the
// form refuses comes back as typed with why, a row of an unknown name opened to confirm.
func (h Handlers) save(w http.ResponseWriter, r *http.Request, id, draftID int64) {
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
	rec, f, ok := readForm(ctx, r.PostForm, id, draftID, known)
	switch r.PostForm.Get("add") {
	case "ingredients":
		httpx.Render(w, r, h.Log, http.StatusOK, formPage(h.Clock.Now(), withRow(f)))
		return
	case "steps":
		httpx.Render(w, r, h.Log, http.StatusOK, formPage(h.Clock.Now(), withStep(f)))
		return
	}
	if picking(r.PostForm) {
		httpx.Render(w, r, h.Log, http.StatusOK, formPage(h.Clock.Now(), unrefused(f)))
		return
	}
	refused := func(reason string, f RecipeForm) {
		httpx.Refused(r, reason, f.Errs)
		web.Answer{Status: http.StatusUnprocessableEntity, Page: formPage(h.Clock.Now(), f)}.Write(w, r, h.Log)
	}
	if !ok {
		refused("Recipe refused", f)
		return
	}
	notice := msgAdded
	switch {
	case draftID != 0:
		id, err = h.Service.Accept(ctx, web.LanguageOf(ctx), draftID, rec)
	case id == 0:
		id, err = h.Service.Add(ctx, web.LanguageOf(ctx), rec)
	default:
		notice = msgSaved
		err = h.Service.Edit(ctx, web.LanguageOf(ctx), id, rec)
	}
	if errors.Is(err, ErrNoRecipe) || errors.Is(err, ErrNoDraft) {
		http.NotFound(w, r)
		return
	}
	if taken, ok := errors.AsType[*NameTakenError](err); ok {
		f.Errs["name"], f.TakenID, f.TakenName = web.T(ctx, msgNameTaken), taken.ID, taken.Existing
		refused("Recipe name taken", f)
		return
	}
	if _, ok := errors.AsType[*ingredients.NameTakenError](err); ok {
		if known, err = h.Service.KnownIngredients(ctx); err != nil {
			httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
			return
		}
		_, f, _ = readForm(ctx, r.PostForm, id, draftID, known)
		refused("a new Ingredient's name was taken meanwhile", f)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "save a Recipe", err)
		return
	}
	web.Answer{Notice: web.Tf(ctx, notice, rec.Name), RedirectTo: recipeURL(id)}.Write(w, r, h.Log)
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

// draftURL is the page of the Recipe Draft id.
func draftURL(id int64) string { return "/recipe-drafts/" + strconv.FormatInt(id, 10) }
