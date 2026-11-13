package recipes

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/web"
)

// IngredientRow is one row of the Recipe form as shown: what was typed, and for an
// Ingredient not known the rest of what makes it one, to confirm.
type IngredientRow struct {
	Name, Quantity, Unit, Usual string
	Values                      map[string]string
	Staple                      bool
	// New says the name is no Ingredient of the User's. Confirming says its fields are
	// only now shown, so nothing about them is refused yet.
	New, Confirming bool
	// Focus says the row takes focus as it arrives.
	Focus bool
	Errs  map[string]string
	// Suggested is the known Ingredient a new one's name is close to. UnitChanged says
	// why the quantity was cleared: the row became a known Ingredient in another unit.
	Suggested, UnitChanged string
}

// RecipeForm is the Recipe form as shown: what was typed, why it was refused by field,
// and the Ingredients the User has, to pick from.
type RecipeForm struct {
	// ID is the Recipe being edited, or 0 for a new one. DraftID is the Recipe Draft the
	// new one is checked from, or 0.
	ID, DraftID            int64
	Action                 string
	Title, Heading, Submit web.Message
	// RemoveAction deletes the Recipe or discards the Recipe Draft, with the button
	// Remove, or is "" for a new Recipe.
	RemoveAction                  string
	Remove                        web.Message
	Name, Portions, Active, Total string
	// Slots and Appliances are the codes of the kinds ticked.
	Slots, Appliances []string
	Steps             []string
	Ingredients       []IngredientRow
	Known             []domain.Ingredient
	Errs              map[string]string
	// Taken is the Recipe the name typed is already.
	TakenID   int64
	TakenName string
	// Confirm says some Ingredient's fields are shown for the first time.
	Confirm bool
}

// blankForm is the form to write a new Recipe with, with an empty row and step to start.
func blankForm(known []domain.Ingredient) RecipeForm {
	return newForm(RecipeForm{Known: known, Errs: map[string]string{}})
}

// formOf is the form to edit rec with, holding it as it is.
func formOf(ctx context.Context, rec domain.Recipe, known []domain.Ingredient) RecipeForm {
	f := RecipeForm{
		ID: rec.ID, Name: rec.Name, Portions: strconv.Itoa(rec.Portions), Active: strconv.Itoa(rec.ActiveMinutes),
		Total: strconv.Itoa(rec.TotalMinutes), Appliances: codesOf(rec.Appliances), Steps: rec.Steps,
		Slots: codesOf(rec.Kinds), Known: known, Errs: map[string]string{},
	}
	for _, ri := range rec.Ingredients {
		row := IngredientRow{
			Name: ri.Ingredient.Name, Quantity: web.Number(ctx, ri.Quantity.Number()), Unit: string(ri.Ingredient.Unit),
		}
		if ri.Ingredient.ID == 0 { // a Recipe Draft's new Ingredient, its proposed values shown to confirm
			row.New, row.Usual, row.Staple = true, string(ri.Ingredient.Usual), ri.Ingredient.Staple
			row.Values = ingredients.ValuesOf(ri.Ingredient.Nutrition)
			row.Suggested, _ = domain.ClosestIngredient(web.LanguageOf(ctx), ri.Ingredient.Name, namesOf(known))
		} else if ri.Quantity == 0 { // a Recipe Draft's row known since in another unit
			row.Quantity, row.UnitChanged = "", unitChanged(ctx, ri.Ingredient.Unit)
		}
		f.Ingredients = append(f.Ingredients, row)
	}
	return newForm(f)
}

// draftForm is the form to check the Recipe Draft id with, holding rec as it was parsed.
func draftForm(ctx context.Context, id int64, rec domain.Recipe, known []domain.Ingredient) RecipeForm {
	f := formOf(ctx, rec, known)
	f.DraftID = id
	return newForm(f)
}

// newForm is f as the form to post, with a row and a step to type in.
func newForm(f RecipeForm) RecipeForm {
	f.Action, f.Title, f.Heading, f.Submit = "/recipes", msgWriteTitle, msgWrite, msgSave
	switch {
	case f.ID != 0:
		f.Action, f.Title, f.Heading = recipeURL(f.ID), msgEditTitle, msgEditHeading
		f.RemoveAction, f.Remove = recipeURL(f.ID)+"/delete", msgDelete
	case f.DraftID != 0:
		f.Action, f.Title, f.Heading, f.Submit = draftURL(f.DraftID), msgDraftTitle, DraftName, msgAccept
		f.RemoveAction, f.Remove = draftURL(f.DraftID)+"/discard", msgDiscard
	}
	if len(f.Ingredients) == 0 {
		f.Ingredients = []IngredientRow{{}}
	}
	if len(f.Steps) == 0 {
		f.Steps = []string{""}
	}
	return f
}

// byName is known by the key that identifies each for a User speaking l.
func byName(l domain.Language, known []domain.Ingredient) map[string]domain.Ingredient {
	m := make(map[string]domain.Ingredient, len(known))
	for _, ing := range known {
		m[domain.NormaliseIngredientName(l, ing.Name)] = ing
	}
	return m
}

// namesOf is known's names.
func namesOf(known []domain.Ingredient) []string {
	names := make([]string, len(known))
	for i, ing := range known {
		names[i] = ing.Name
	}
	return names
}

// unitChanged says to give a quantity again in unit, which Dandori won't convert to.
func unitChanged(ctx context.Context, unit domain.Unit) string {
	return web.Tf(ctx, msgUnitChanged, web.T(ctx, ingredients.RecordedIn[unit]))
}

// rowField is the form field of field on the row at index i.
func rowField(field string, i int) string { return fmt.Sprintf("ing_%s_%d", field, i) }

// readRow reads the row at index i of form, matching its name to known, as an Ingredient
// (one without an ID being new) and the row as shown. settled says the User has seen
// the row for what it is now: a new one's fields, or a known one as known. A row picked
// takes the name of the known Ingredient picked, if the User still has it. A row
// shown in a unit that becomes a known Ingredient in another has its quantity
// cleared, never converted.
func readRow(ctx context.Context, form url.Values, i int, known map[string]domain.Ingredient) (ing domain.Ingredient, row IngredientRow, settled bool) {
	l := web.LanguageOf(ctx)
	in := func(field string) string { return strings.TrimSpace(form.Get(rowField(field, i))) }
	row = IngredientRow{Name: in("name"), Quantity: in("quantity"), Errs: map[string]string{}}
	names := namesOf(slices.Collect(maps.Values(known)))
	if _, ok := known[domain.NormaliseIngredientName(l, in("pick"))]; ok {
		row.Name = in("pick")
	}
	seen := in("new")
	ing, isKnown := known[domain.NormaliseIngredientName(l, row.Name)]
	if isKnown {
		row.Name, row.Unit = ing.Name, string(ing.Unit)
		if shown := in("unit"); seen == "true" && shown != "" && shown != row.Unit {
			row.Quantity, row.UnitChanged = "", unitChanged(ctx, ing.Unit)
		}
		return ing, row, seen != "true"
	}
	if row.Name == "" {
		return ing, row, true
	}
	row.New, row.Confirming = true, seen != "true"
	row.Suggested, _ = domain.ClosestIngredient(l, row.Name, names)
	row.Unit, row.Usual, row.Staple = in("unit"), in("usual"), in("staple") != ""
	row.Values = map[string]string{}
	for _, field := range []string{"kcal", "protein", "carbs", "fat", "fibre", "sugar"} {
		row.Values[field] = in(field)
	}
	nutrition, problems := domain.ParseNutrition(row.Values)
	ing = domain.Ingredient{
		Name: row.Name, Unit: domain.Unit(row.Unit), Usual: domain.StorageLocation(row.Usual), Staple: row.Staple,
		Nutrition: nutrition,
	}
	if !row.Confirming {
		row.Errs = ingredients.NutritionErrs(ctx, problems)
	}
	return ing, row, !row.Confirming
}

// readForm reads a posted form for the Recipe id, 0 for a new one, checked from the
// Recipe Draft draftID unless that is 0, matching its rows to known. It is the Recipe,
// the form to show back, and whether the Recipe can be saved: it is valid and the User
// has seen every new Ingredient's fields.
func readForm(ctx context.Context, form url.Values, id, draftID int64, known []domain.Ingredient) (domain.Recipe, RecipeForm, bool) {
	l := web.LanguageOf(ctx)
	get := func(field string) string { return strings.TrimSpace(form.Get(field)) }
	f := newForm(RecipeForm{
		ID: id, DraftID: draftID, Name: get("name"), Portions: get("portions"), Active: get("active"), Total: get("total"),
		Steps: nonBlank(form["steps"]), Slots: form["slots"], Appliances: form["appliances"], Known: known, Errs: map[string]string{},
	})
	rec := domain.Recipe{Name: f.Name, Steps: nonBlank(form["steps"])}
	for _, kind := range domain.SlotKinds {
		if slices.Contains(f.Slots, string(kind)) {
			rec.Kinds = append(rec.Kinds, kind)
		}
	}
	for _, kind := range domain.ApplianceKinds {
		if slices.Contains(f.Appliances, string(kind)) {
			rec.Appliances = append(rec.Appliances, kind)
		}
	}
	refuse := func(field string, problem domain.Problem) {
		if _, refused := f.Errs[field]; !refused && problem != "" {
			f.Errs[field] = web.T(ctx, fieldMessage(field, problem))
		}
	}
	var problem domain.Problem
	rec.Portions, problem = domain.ParseHowMany(f.Portions)
	refuse("portions", problem)
	rec.ActiveMinutes, problem = domain.ParseMinutes(f.Active)
	refuse("active", problem)
	rec.TotalMinutes, problem = domain.ParseMinutes(f.Total)
	refuse("total", problem)

	names := byName(l, known)
	var rows []IngredientRow
	for i := 0; form.Has(rowField("name", i)); i++ {
		ing, row, settled := readRow(ctx, form, i, names)
		if row.Name == "" && row.Quantity == "" {
			continue // a blank row, or one cleared to remove it
		}
		f.Confirm = f.Confirm || !settled
		unit := ing.Unit
		if !slices.Contains(domain.Units, unit) {
			unit = domain.Pieces // not chosen yet, so any quantity the unit may allow
		}
		quantity, qp := domain.ParseQuantity(unit, row.Quantity)
		if qp != "" && row.UnitChanged == "" {
			row.Errs["quantity"] = web.T(ctx, web.ProblemMessage(qp, quantityMessages))
		}
		rec.Ingredients = append(rec.Ingredients, domain.RecipeIngredient{Ingredient: ing, Quantity: quantity})
		rows = append(rows, row)
	}
	recipeProblems, rowProblems := rec.Problems(l)
	for field, problem := range recipeProblems {
		refuse(field, problem)
	}
	ok := true
	for i, problems := range rowProblems {
		for field, problem := range problems {
			own := field == "name" || field == "quantity"
			if _, refused := rows[i].Errs[field]; refused || (!own && (rows[i].Confirming || rows[i].Name == "")) {
				continue
			}
			rows[i].Errs[field] = rowProblem(ctx, field, problem)
		}
		ok = ok && len(rows[i].Errs) == 0 && rows[i].UnitChanged == ""
	}
	f.Ingredients = rows
	f = newForm(f)
	if msg := f.Errs["ingredients"]; msg != "" { // on the row left to fill in, to focus it
		f.Ingredients[0].Errs = map[string]string{"name": msg}
		delete(f.Errs, "ingredients")
		ok = false
	}
	return rec, f, ok && len(f.Errs) == 0 && !f.Confirm
}

// rowProblem says why field of a row is refused.
func rowProblem(ctx context.Context, field string, problem domain.Problem) string {
	switch field {
	case "name":
		return web.T(ctx, web.ProblemMessage(problem, rowNameMessages))
	case "quantity":
		return web.T(ctx, web.ProblemMessage(problem, quantityMessages))
	}
	return web.T(ctx, ingredients.ProblemMessage(field, problem))
}

// typed is row as the fields confirming its new Ingredient show it.
func (row IngredientRow) typed() ingredients.TypedIngredient {
	return ingredients.TypedIngredient{
		Unit: row.Unit, Usual: row.Usual, Staple: row.Staple, Values: row.Values, Errs: row.Errs, Focus: row.Confirming,
	}
}

// nonBlank is lines trimmed, without the blank ones.
func nonBlank(lines []string) []string {
	out := []string{}
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// picking reports whether form picks a suggestion for one of its rows.
func picking(form url.Values) bool {
	for field := range form {
		if strings.HasPrefix(field, "ing_pick_") {
			return true
		}
	}
	return false
}

// rowIndex is the row a fragment's query is about, from its name field's number.
func rowIndex(query url.Values) (int, bool) {
	for field := range query {
		if n, ok := strings.CutPrefix(field, "ing_name_"); ok {
			i, err := strconv.Atoi(n)
			return i, err == nil && i >= 0
		}
	}
	return 0, false
}
