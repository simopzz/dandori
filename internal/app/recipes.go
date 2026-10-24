package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

func (a *App) showRecipeLibrary(w http.ResponseWriter, r *http.Request) {
	a.renderRecipeLibrary(w, r, web.RecipeLibrary{}, http.StatusOK)
}

func (a *App) renderRecipeLibrary(w http.ResponseWriter, r *http.Request, l web.RecipeLibrary, status int) {
	recipes, err := a.loadRecipes(r.Context(), nil)
	if err != nil {
		a.serverError(w, "load recipes", err)
		return
	}
	for _, rec := range recipes {
		l.Recipes = append(l.Recipes, web.RecipeSummary{
			ID:          rec.id,
			Name:        rec.Name,
			Portions:    rec.Portions,
			PrepMinutes: rec.PrepMinutes,
			PerPortion:  web.PortionNutrition(rec.NutritionPerPortion()),
		})
	}
	templ.Handler(web.RecipeLibraryPage(l), templ.WithStatus(status)).ServeHTTP(w, r)
}

// parseRecipe starts a Run parsing the pasted text, and sends the user to
// the draft it will produce.
func (a *App) parseRecipe(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.PostForm.Get("text"))
	if text == "" {
		a.renderRecipeLibrary(w, r, web.RecipeLibrary{Error: "Paste a recipe to parse"}, http.StatusUnprocessableEntity)
		return
	}
	known, err := a.loadIngredients(r.Context())
	if err != nil {
		a.serverError(w, "load ingredients", err)
		return
	}
	id, err := a.startRun(r.Context(), recipeParseKind, a.recipeParseJob(text, known))
	if err != nil {
		a.serverError(w, "start recipe parse", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/recipe-drafts/%d", id), http.StatusSeeOther)
}

// showDraft shows the draft a parse Run produced, ready to check and
// accept, or the Run's trace while there is none.
func (a *App) showDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// While the Run goes on, the model may still change the draft.
	if live := a.runs.get(id); live != nil {
		a.showDraftPending(w, r, id)
		return
	}
	draft, found, err := a.loadDraft(r.Context(), id)
	if err != nil {
		a.serverError(w, "load draft", err)
		return
	}
	if !found {
		a.showDraftPending(w, r, id)
		return
	}
	known, err := a.loadIngredients(r.Context())
	if err != nil {
		a.serverError(w, "load ingredients", err)
		return
	}
	_, f, _ := readRecipeForm(recipeFormValues(draft.recipe(known), known), known)
	templ.Handler(web.RecipeFormPage(draftForm(id, f))).ServeHTTP(w, r)
}

func (a *App) showDraftPending(w http.ResponseWriter, r *http.Request, id int64) {
	run, found, err := a.loadRun(r.Context(), id)
	if err != nil {
		a.serverError(w, "load run", err)
		return
	}
	if !found || run.Kind != recipeParseKind {
		http.NotFound(w, r)
		return
	}
	run.Events, err = a.loadEvents(r.Context(), id, 0)
	if err != nil {
		a.serverError(w, "load trace", err)
		return
	}
	templ.Handler(web.DraftPendingPage(web.DraftPending{Run: run})).ServeHTTP(w, r)
}

func draftForm(id int64, f web.RecipeForm) web.RecipeForm {
	f.Title = "Check the parsed Recipe"
	f.Action = fmt.Sprintf("/recipe-drafts/%d", id)
	f.Submit = "Accept Recipe"
	f.Discard = fmt.Sprintf("/recipe-drafts/%d/discard", id)
	return f
}

// acceptDraft saves the draft, as the user corrected it, to the Recipe
// Library.
func (a *App) acceptDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, found, err := a.loadDraft(r.Context(), id); err != nil {
		a.serverError(w, "load draft", err)
		return
	} else if !found {
		http.NotFound(w, r)
		return
	}
	a.submitRecipe(w, r, func(f web.RecipeForm) web.RecipeForm { return draftForm(id, f) },
		func(ctx context.Context, tx pgx.Tx, rec domain.Recipe) (int64, error) {
			// Deleting the draft first locks it, so it is accepted only once.
			if err := deleteDraft(ctx, tx, id); err != nil {
				return 0, err
			}
			return storeRecipe(ctx, tx, 0, rec)
		})
}

func (a *App) discardDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := deleteDraft(r.Context(), a.pool, id); err != nil && !errors.Is(err, errNoDraft) {
		a.serverError(w, "discard draft", err)
		return
	}
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}

func (a *App) showRecipe(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.recipeFromPath(w, r)
	if !ok {
		return
	}
	view := web.RecipeView{
		ID:          rec.id,
		Name:        rec.Name,
		Portions:    rec.Portions,
		PrepMinutes: rec.PrepMinutes,
		Appliances:  rec.Appliances,
		Steps:       rec.Steps,
		PerPortion:  web.PortionNutrition(rec.NutritionPerPortion()),
	}
	for _, ri := range rec.Ingredients {
		view.Ingredients = append(view.Ingredients,
			fmt.Sprintf("%s %s %s", formatAmount(ri.Quantity), ri.Ingredient.Unit, ri.Ingredient.Name))
	}
	templ.Handler(web.RecipePage(view)).ServeHTTP(w, r)
}

func (a *App) editRecipe(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.recipeFromPath(w, r)
	if !ok {
		return
	}
	known, err := a.loadIngredients(r.Context())
	if err != nil {
		a.serverError(w, "load ingredients", err)
		return
	}
	_, f, _ := readRecipeForm(recipeFormValues(rec.Recipe, known), known)
	templ.Handler(web.RecipeFormPage(editForm(rec.id, f))).ServeHTTP(w, r)
}

func editForm(id int64, f web.RecipeForm) web.RecipeForm {
	f.Title = "Edit Recipe"
	f.Action = fmt.Sprintf("/recipes/%d", id)
	f.Submit = "Save Recipe"
	return f
}

func (a *App) updateRecipe(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.recipeFromPath(w, r)
	if !ok {
		return
	}
	a.submitRecipe(w, r, func(f web.RecipeForm) web.RecipeForm { return editForm(rec.id, f) },
		func(ctx context.Context, tx pgx.Tx, updated domain.Recipe) (int64, error) {
			return storeRecipe(ctx, tx, rec.id, updated)
		})
}

func (a *App) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := a.pool.Exec(r.Context(), `DELETE FROM recipe WHERE id = $1`, id); err != nil {
		a.serverError(w, "delete recipe", err)
		return
	}
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}

// submitRecipe reads a submitted Recipe form and, when it is valid, stores
// it with store; otherwise it shows the form back, framed by frame.
func (a *App) submitRecipe(w http.ResponseWriter, r *http.Request,
	frame func(web.RecipeForm) web.RecipeForm,
	store func(context.Context, pgx.Tx, domain.Recipe) (int64, error),
) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	known, err := a.loadIngredients(r.Context())
	if err != nil {
		a.serverError(w, "load ingredients", err)
		return
	}
	rec, f, ok := readRecipeForm(r.PostForm, known)
	if !ok {
		templ.Handler(web.RecipeFormPage(frame(f)), templ.WithStatus(http.StatusUnprocessableEntity)).ServeHTTP(w, r)
		return
	}
	var id int64
	err = pgx.BeginFunc(r.Context(), a.pool, func(tx pgx.Tx) error {
		id, err = store(r.Context(), tx, rec)
		return err
	})
	if errors.Is(err, errNoDraft) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		// Keep what was typed rather than replacing the form with an error.
		a.log.Error("store recipe", "err", err)
		f.Failed = true
		templ.Handler(web.RecipeFormPage(frame(f)), templ.WithStatus(http.StatusInternalServerError)).ServeHTTP(w, r)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/recipes/%d", id), http.StatusSeeOther)
}

// recipeFromPath loads the Recipe the path names, answering 404 if there
// is none.
func (a *App) recipeFromPath(w http.ResponseWriter, r *http.Request) (storedRecipe, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return storedRecipe{}, false
	}
	recipes, err := a.loadRecipes(r.Context(), &id)
	if err != nil {
		a.serverError(w, "load recipe", err)
		return storedRecipe{}, false
	}
	if len(recipes) == 0 {
		http.NotFound(w, r)
		return storedRecipe{}, false
	}
	return recipes[0], true
}

// recipeFormValues is the form a Recipe is shown in, marking which of its
// Ingredients are new.
func recipeFormValues(rec domain.Recipe, known map[string]domain.Ingredient) url.Values {
	v := url.Values{
		"name":         {rec.Name},
		"portions":     {strconv.Itoa(rec.Portions)},
		"prep_minutes": {strconv.Itoa(rec.PrepMinutes)},
		"appliances":   {strings.Join(rec.Appliances, "\n")},
		"steps":        rec.Steps,
	}
	for i, ri := range rec.Ingredients {
		ing := ri.Ingredient
		_, isKnown := known[ing.Name]
		field := func(name string) string { return fmt.Sprintf("ing_%s_%d", name, i) }
		v.Set(field("name"), ing.Name)
		v.Set(field("quantity"), formatAmount(ri.Quantity))
		v.Set(field("unit"), string(ing.Unit))
		v.Set(field("new"), strconv.FormatBool(!isKnown))
		if !isKnown {
			v.Set(field("kcal"), formatAmount(ing.Nutrition.Kcal))
			v.Set(field("protein_g"), formatAmount(ing.Nutrition.ProteinG))
			v.Set(field("carbs_g"), formatAmount(ing.Nutrition.CarbsG))
			v.Set(field("fat_g"), formatAmount(ing.Nutrition.FatG))
		}
	}
	return v
}

// readRecipeForm reads a Recipe form, matching its Ingredients by name to
// the known ones. It returns the form to show back, and whether the Recipe
// can be saved: it is valid, and no Ingredient turned from known to new or
// back since the form was shown, so every new Ingredient's nutritional
// values have been seen.
func readRecipeForm(form url.Values, known map[string]domain.Ingredient) (domain.Recipe, web.RecipeForm, bool) {
	get := func(name string) string { return strings.TrimSpace(form.Get(name)) }
	f := web.RecipeForm{Values: map[string]string{}, Errors: map[string]string{}}
	for _, name := range []string{"name", "portions", "prep_minutes", "appliances"} {
		f.Values[name] = get(name)
	}
	f.Steps = nonBlank(form["steps"])
	rec := domain.Recipe{
		Name:       get("name"),
		Appliances: domain.FoodTerms(get("appliances")),
		Steps:      f.Steps,
	}
	rec.Portions = readInt(f, "portions")
	rec.PrepMinutes = readInt(f, "prep_minutes")

	for i := 0; form.Has(fmt.Sprintf("ing_name_%d", i)); i++ {
		in := func(name string) string { return get(fmt.Sprintf("ing_%s_%d", name, i)) }
		row := web.IngredientRow{
			Name:     domain.IngredientName(in("name")),
			Quantity: in("quantity"),
			Unit:     in("unit"),
			Kcal:     in("kcal"),
			ProteinG: in("protein_g"),
			CarbsG:   in("carbs_g"),
			FatG:     in("fat_g"),
		}
		if row.Name == "" && row.Quantity == "" {
			continue // a blank row, or one cleared to remove it
		}
		// Rows are renumbered without the blank ones, as the domain numbers them.
		at := len(f.Ingredients)
		out := func(name string) string { return fmt.Sprintf("ing_%s_%d", name, at) }

		ing, isKnown := known[row.Name]
		if row.Name != "" && in("new") != strconv.FormatBool(!isKnown) {
			f.Rematched = true
		}
		// A known Ingredient in another unit, say after a rename: the
		// quantity means nothing in its unit, so it must be given again.
		requantify := isKnown && row.Unit != "" && row.Unit != string(ing.Unit)
		if isKnown {
			row.Unit = string(ing.Unit)
			row.Kcal, row.ProteinG, row.CarbsG, row.FatG = "", "", "", ""
		} else {
			row.New = true
			ing = domain.Ingredient{Name: row.Name, Unit: domain.Unit(row.Unit)}
			ing.Nutrition.Kcal = readAmount(f, out("kcal"), row.Kcal)
			ing.Nutrition.ProteinG = readAmount(f, out("protein_g"), row.ProteinG)
			ing.Nutrition.CarbsG = readAmount(f, out("carbs_g"), row.CarbsG)
			ing.Nutrition.FatG = readAmount(f, out("fat_g"), row.FatG)
		}
		quantity := readAmount(f, out("quantity"), row.Quantity)
		if requantify {
			row.Quantity = ""
			f.Errors[out("quantity")] = fmt.Sprintf("%s is measured in %s; give the quantity in %s", ing.Name, ing.Unit, ing.Unit)
		}
		rec.Ingredients = append(rec.Ingredients, domain.RecipeIngredient{Ingredient: ing, Quantity: quantity})
		f.Ingredients = append(f.Ingredients, row)
	}

	for path, msg := range rec.Validate() {
		field := recipeField(path)
		if f.Errors[field] == "" {
			f.Errors[field] = msg
		}
	}
	ok := len(f.Errors) == 0 && !f.Rematched
	if len(f.Errors) == 0 {
		n := web.PortionNutrition(rec.NutritionPerPortion())
		f.PerPortion = &n
	}
	return rec, f, ok
}

// recipeField is the form field a domain field path such as
// "Ingredients.2.Quantity" is shown at.
func recipeField(path string) string {
	switch path {
	case "Name":
		return "name"
	case "Portions":
		return "portions"
	case "PrepMinutes":
		return "prep_minutes"
	case "Ingredients":
		return "ingredients"
	}
	var i int
	var field string
	if _, err := fmt.Sscanf(strings.ReplaceAll(path, ".", " "), "Ingredients %d %s", &i, &field); err == nil {
		return fmt.Sprintf("ing_%s_%d", strings.ToLower(field), i)
	}
	return path
}

func readInt(f web.RecipeForm, name string) int {
	raw := f.Values[name]
	if raw == "" {
		f.Errors[name] = "Required"
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	switch {
	case errors.Is(err, strconv.ErrRange):
		f.Errors[name] = "Too large"
	case err != nil:
		f.Errors[name] = "Must be a whole number"
	}
	return int(v)
}

// readAmount reads a decimal amount, taking a comma as the decimal point.
func readAmount(f web.RecipeForm, name, raw string) float64 {
	if raw == "" {
		f.Errors[name] = "Required"
		return 0
	}
	v, err := strconv.ParseFloat(strings.Replace(raw, ",", ".", 1), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		f.Errors[name] = "Must be a number"
	}
	return v
}

// formatAmount shows an amount without trailing zeros.
func formatAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// storedRecipe is a Recipe in the Recipe Library.
type storedRecipe struct {
	id int64
	domain.Recipe
}

// loadRecipes loads the Recipe with id, or every Recipe when id is nil,
// newest first, with their Ingredients. Both queries read one snapshot, so
// a Recipe saved or deleted meanwhile is either wholly there or not at all.
func (a *App) loadRecipes(ctx context.Context, id *int64) ([]storedRecipe, error) {
	var recipes []storedRecipe
	snapshot := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := pgx.BeginTxFunc(ctx, a.pool, snapshot, func(tx pgx.Tx) error {
		rows, _ := tx.Query(ctx, `
			SELECT id, name, portions, prep_minutes, appliances, steps FROM recipe
			WHERE $1::bigint IS NULL OR id = $1 ORDER BY id DESC`, id)
		var err error
		recipes, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedRecipe, error) {
			var r storedRecipe
			err := row.Scan(&r.id, &r.Name, &r.Portions, &r.PrepMinutes, &r.Appliances, &r.Steps)
			return r, err
		})
		if err != nil || len(recipes) == 0 {
			return err
		}
		byID := map[int64]*storedRecipe{}
		for i := range recipes {
			byID[recipes[i].id] = &recipes[i]
		}
		rows, _ = tx.Query(ctx, `
			SELECT ri.recipe_id, ri.quantity, i.name, i.unit, i.kcal, i.protein_g, i.carbs_g, i.fat_g
			FROM recipe_ingredient ri JOIN ingredient i ON i.id = ri.ingredient_id
			WHERE $1::bigint IS NULL OR ri.recipe_id = $1
			ORDER BY ri.recipe_id, ri.position`, id)
		var recipeID int64
		var ri domain.RecipeIngredient
		n := &ri.Ingredient.Nutrition
		_, err = pgx.ForEachRow(rows, []any{&recipeID, &ri.Quantity, &ri.Ingredient.Name, &ri.Ingredient.Unit, &n.Kcal, &n.ProteinG, &n.CarbsG, &n.FatG}, func() error {
			r := byID[recipeID]
			r.Ingredients = append(r.Ingredients, ri)
			return nil
		})
		return err
	})
	return recipes, err
}

// loadIngredients loads every Ingredient, by name.
func (a *App) loadIngredients(ctx context.Context) (map[string]domain.Ingredient, error) {
	rows, _ := a.pool.Query(ctx, `SELECT name, unit, kcal, protein_g, carbs_g, fat_g FROM ingredient`)
	known := map[string]domain.Ingredient{}
	var ing domain.Ingredient
	n := &ing.Nutrition
	_, err := pgx.ForEachRow(rows, []any{&ing.Name, &ing.Unit, &n.Kcal, &n.ProteinG, &n.CarbsG, &n.FatG}, func() error {
		known[ing.Name] = ing
		return nil
	})
	return known, err
}

func (a *App) loadDraft(ctx context.Context, runID int64) (parsedRecipe, bool, error) {
	var raw []byte
	err := a.pool.QueryRow(ctx, `SELECT recipe FROM recipe_draft WHERE run_id = $1`, runID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return parsedRecipe{}, false, nil
	}
	if err != nil {
		return parsedRecipe{}, false, err
	}
	var p parsedRecipe
	if err := json.Unmarshal(raw, &p); err != nil {
		return parsedRecipe{}, false, fmt.Errorf("read draft of run %d: %w", runID, err)
	}
	return p, true, nil
}

// errNoDraft means the draft was already accepted or discarded.
var errNoDraft = errors.New("no such draft")

func deleteDraft(ctx context.Context, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, runID int64) error {
	tag, err := db.Exec(ctx, `DELETE FROM recipe_draft WHERE run_id = $1`, runID)
	if err == nil && tag.RowsAffected() == 0 {
		err = errNoDraft
	}
	return err
}

// storeRecipe stores rec as the Recipe with id, or as a new Recipe when id
// is 0, adding its new Ingredients. It returns the Recipe's id.
func storeRecipe(ctx context.Context, tx pgx.Tx, id int64, rec domain.Recipe) (int64, error) {
	var err error
	if id == 0 {
		err = tx.QueryRow(ctx, `
			INSERT INTO recipe (name, portions, prep_minutes, appliances, steps)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			rec.Name, rec.Portions, rec.PrepMinutes, rec.Appliances, rec.Steps).Scan(&id)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE recipe SET name = $2, portions = $3, prep_minutes = $4, appliances = $5, steps = $6, updated_at = now()
			WHERE id = $1`,
			id, rec.Name, rec.Portions, rec.PrepMinutes, rec.Appliances, rec.Steps)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM recipe_ingredient WHERE recipe_id = $1`, id)
		}
	}
	if err != nil {
		return 0, err
	}
	for i, ri := range rec.Ingredients {
		ing, n := ri.Ingredient, ri.Ingredient.Nutrition
		// A known Ingredient keeps its values: only new ones are inserted.
		if _, err := tx.Exec(ctx, `
			INSERT INTO ingredient (name, unit, kcal, protein_g, carbs_g, fat_g)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			ing.Name, ing.Unit, n.Kcal, n.ProteinG, n.CarbsG, n.FatG); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recipe_ingredient (recipe_id, position, ingredient_id, quantity)
			SELECT $1, $2, id, $4 FROM ingredient WHERE name = $3`,
			id, i, ing.Name, ri.Quantity); err != nil {
			return 0, err
		}
	}
	return id, nil
}
