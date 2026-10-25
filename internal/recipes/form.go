package recipes

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/simopzz/dandori/internal/domain"
)

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
func readRecipeForm(form url.Values, known map[string]domain.Ingredient) (domain.Recipe, RecipeForm, bool) {
	get := func(name string) string { return strings.TrimSpace(form.Get(name)) }
	f := RecipeForm{Values: map[string]string{}, Errors: map[string]string{}}
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
		row := IngredientRow{
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
		n := PortionNutrition(rec.NutritionPerPortion())
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

func readInt(f RecipeForm, name string) int {
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
func readAmount(f RecipeForm, name, raw string) float64 {
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

func nonBlank(lines []string) []string {
	out := []string{}
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
