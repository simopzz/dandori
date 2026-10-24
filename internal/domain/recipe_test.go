package domain_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

var (
	chickenBreast = domain.Ingredient{Name: "chicken breast", Unit: domain.Grams,
		Nutrition: domain.IngredientNutrition{Kcal: 110, ProteinG: 23, CarbsG: 0, FatG: 1.5}}
	oliveOil = domain.Ingredient{Name: "olive oil", Unit: domain.Millilitres,
		Nutrition: domain.IngredientNutrition{Kcal: 820, ProteinG: 0, CarbsG: 0, FatG: 91}}
	egg = domain.Ingredient{Name: "egg", Unit: domain.Pieces,
		Nutrition: domain.IngredientNutrition{Kcal: 70, ProteinG: 6.3, CarbsG: 0.4, FatG: 4.8}}
	rice = domain.Ingredient{Name: "basmati rice", Unit: domain.Grams,
		Nutrition: domain.IngredientNutrition{Kcal: 350, ProteinG: 7.5, CarbsG: 78, FatG: 0.9}}
)

func TestRecipeNutritionPerPortionIsDerivedFromItsIngredients(t *testing.T) {
	tests := []struct {
		name   string
		recipe domain.Recipe
		want   domain.Nutrition
	}{
		{
			// Nutrition is per 100 g: 600 g of chicken is six times it.
			name: "grams, one Portion",
			recipe: domain.Recipe{Portions: 1, Ingredients: []domain.RecipeIngredient{
				{Ingredient: chickenBreast, Quantity: 600},
			}},
			want: domain.Nutrition{Kcal: 660, ProteinG: 138, CarbsG: 0, FatG: 9},
		},
		{
			// 600 g chicken + 30 ml oil + 3 eggs + 240 g rice, over three Portions:
			// kcal 660+246+210+840 = 1956 → 652; protein 138+0+18.9+18 = 174.9 → 58.3;
			// carbs 0+0+1.2+187.2 = 188.4 → 62.8; fat 9+27.3+14.4+2.16 = 52.86 → 17.62.
			name: "grams, millilitres and pieces over three Portions",
			recipe: domain.Recipe{Portions: 3, Ingredients: []domain.RecipeIngredient{
				{Ingredient: chickenBreast, Quantity: 600},
				{Ingredient: oliveOil, Quantity: 30},
				{Ingredient: egg, Quantity: 3},
				{Ingredient: rice, Quantity: 240},
			}},
			want: domain.Nutrition{Kcal: 652, ProteinG: 58, CarbsG: 63, FatG: 18},
		},
		{
			name:   "no Ingredients",
			recipe: domain.Recipe{Portions: 2},
			want:   domain.Nutrition{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.recipe.NutritionPerPortion(); got != tt.want {
				t.Errorf("NutritionPerPortion() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRecipeValidation(t *testing.T) {
	valid := func() domain.Recipe {
		return domain.Recipe{
			Name:        "Chicken and rice",
			Portions:    3,
			PrepMinutes: 40,
			Ingredients: []domain.RecipeIngredient{
				{Ingredient: chickenBreast, Quantity: 600},
				{Ingredient: rice, Quantity: 240},
			},
		}
	}
	tests := []struct {
		name   string
		change func(*domain.Recipe)
		want   map[string]string
	}{
		{"valid", func(*domain.Recipe) {}, map[string]string{}},
		{"no prep time is fine", func(r *domain.Recipe) { r.PrepMinutes = 0 }, map[string]string{}},
		{"missing name", func(r *domain.Recipe) { r.Name = "  " }, map[string]string{"Name": "Required"}},
		{"no Portions", func(r *domain.Recipe) { r.Portions = 0 }, map[string]string{"Portions": "Must be at least 1"}},
		{"negative prep time", func(r *domain.Recipe) { r.PrepMinutes = -5 }, map[string]string{"PrepMinutes": "Can't be negative"}},
		{"no Ingredients", func(r *domain.Recipe) { r.Ingredients = nil }, map[string]string{"Ingredients": "A Recipe needs at least one Ingredient"}},
		{"zero quantity", func(r *domain.Recipe) { r.Ingredients[1].Quantity = 0 }, map[string]string{"Ingredients.1.Quantity": "Must be more than 0"}},
		{"unknown unit", func(r *domain.Recipe) { r.Ingredients[0].Ingredient.Unit = "cup" }, map[string]string{"Ingredients.0.Unit": "Must be g, ml or piece"}},
		{"missing Ingredient name", func(r *domain.Recipe) { r.Ingredients[0].Ingredient.Name = "" }, map[string]string{"Ingredients.0.Name": "Required"}},
		{"negative nutritional value", func(r *domain.Recipe) { r.Ingredients[1].Ingredient.Nutrition.FatG = -1 }, map[string]string{"Ingredients.1.Nutrition": "Can't be negative"}},
		{"Ingredient listed twice", func(r *domain.Recipe) {
			r.Ingredients = append(r.Ingredients, domain.RecipeIngredient{Ingredient: chickenBreast, Quantity: 100})
		}, map[string]string{"Ingredients.2.Name": "Listed twice"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid()
			tt.change(&r)
			got := r.Validate()
			if len(got) != len(tt.want) {
				t.Fatalf("Validate() = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("Validate()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
