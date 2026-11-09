package domain_test

import (
	"maps"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

func nutrition(kcal, protein, carbs, fat float64, fibre, sugar domain.Amount) domain.Nutrition {
	return domain.Nutrition{
		Kcal: domain.Known(kcal), Protein: domain.Known(protein), Carbs: domain.Known(carbs), Fat: domain.Known(fat),
		Fibre: fibre, Sugar: sugar,
	}
}

var (
	chicken = domain.Ingredient{ID: 1, Name: "chicken breast", Unit: domain.Grams,
		Nutrition: nutrition(110, 23, 0, 1.5, domain.Known(0), domain.Known(0))}
	oil = domain.Ingredient{ID: 2, Name: "olive oil", Unit: domain.Millilitres,
		Nutrition: nutrition(820, 0, 0, 91, domain.Unknown, domain.Unknown)}
	egg = domain.Ingredient{ID: 3, Name: "egg", Unit: domain.Pieces,
		Nutrition: nutrition(70, 6.3, 0.4, 4.8, domain.Known(0), domain.Unknown)}
	rice = domain.Ingredient{ID: 4, Name: "basmati rice", Unit: domain.Grams,
		Nutrition: nutrition(350, 7.5, 78, 0.9, domain.Known(1.3), domain.Known(0.2))}
)

func grams(v float64) domain.Quantity { return domain.QuantityOf(v) }

func TestRecipeNutritionPerPortionIsDerivedFromItsIngredients(t *testing.T) {
	// 600 g chicken + 30 ml oil + 3 eggs + 240 g rice, over three Portions:
	// kcal 660+246+210+840 = 1956 → 652; protein 138+0+18.9+18 = 174.9 → 58;
	// carbs 0+0+1.2+187.2 = 188.4 → 63; fat 9+27.3+14.4+2.16 = 52.86 → 18.
	r := domain.Recipe{Portions: 3, Ingredients: []domain.RecipeIngredient{
		{Ingredient: chicken, Quantity: grams(600)},
		{Ingredient: oil, Quantity: grams(30)},
		{Ingredient: egg, Quantity: grams(3)},
		{Ingredient: rice, Quantity: grams(240)},
	}}
	got := r.NutritionPerPortion()
	for name, c := range map[string]struct{ got, want float64 }{
		"kcal": {got.Kcal.Value, 652}, "protein": {got.Protein.Value, 58},
		"carbs": {got.Carbs.Value, 63}, "fat": {got.Fat.Value, 18},
	} {
		if c.got != c.want {
			t.Errorf("%s per Portion = %v, want %v", name, c.got, c.want)
		}
	}
}

// Fibre and sugar an Ingredient doesn't know count as nothing, so what is added up is
// only a floor: at least that much. Every Ingredient knowing it is exact, and none
// knowing it is unknown, never 0.
func TestRecipeNutritionPerPortionIsAFloorWhenAValueIsUnknown(t *testing.T) {
	tests := []struct {
		name        string
		ingredients []domain.RecipeIngredient
		want        domain.Amount
		floor       bool
	}{
		{"every Ingredient knows it", []domain.RecipeIngredient{{Ingredient: rice, Quantity: grams(200)}}, domain.Known(3), false},
		{"some Ingredients know it", []domain.RecipeIngredient{
			{Ingredient: rice, Quantity: grams(200)}, {Ingredient: oil, Quantity: grams(30)},
		}, domain.Known(3), true},
		{"no Ingredient knows it", []domain.RecipeIngredient{{Ingredient: oil, Quantity: grams(30)}}, domain.Unknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.Recipe{Portions: 1, Ingredients: tt.ingredients}.NutritionPerPortion()
			if got.Fibre != tt.want || got.FibreFloor != tt.floor {
				t.Errorf("fibre = %+v, floor %v; want %+v, floor %v", got.Fibre, got.FibreFloor, tt.want, tt.floor)
			}
		})
	}
}

// Fibre and sugar are each their own floor.
func TestRecipeNutritionPerPortionFloorsFibreAndSugarApart(t *testing.T) {
	got := domain.Recipe{Portions: 1, Ingredients: []domain.RecipeIngredient{
		{Ingredient: egg, Quantity: grams(2)}, {Ingredient: rice, Quantity: grams(100)},
	}}.NutritionPerPortion()
	if got.FibreFloor || !got.SugarFloor {
		t.Errorf("FibreFloor = %v, SugarFloor = %v; want false, true", got.FibreFloor, got.SugarFloor)
	}
}

func TestRecipeValidation(t *testing.T) {
	valid := func() domain.Recipe {
		return domain.Recipe{
			Name: "Chicken and rice", Portions: 3, ActiveMinutes: 20, TotalMinutes: 40,
			Kinds: []domain.SlotKind{domain.Dinner},
			Ingredients: []domain.RecipeIngredient{
				{Ingredient: chicken, Quantity: grams(600)},
				{Ingredient: rice, Quantity: grams(240)},
			},
		}
	}
	type problems = map[string]domain.Problem
	tests := []struct {
		name   string
		change func(*domain.Recipe)
		want   problems
		rows   map[int]problems
	}{
		{name: "valid", change: func(*domain.Recipe) {}},
		{name: "no steps or appliances are fine", change: func(r *domain.Recipe) { r.Steps, r.Appliances = nil, nil }},
		{name: "no active minutes are fine", change: func(r *domain.Recipe) { r.ActiveMinutes = 0 }},
		{name: "active minutes equal to total are fine", change: func(r *domain.Recipe) { r.ActiveMinutes = 40 }},
		{name: "no name", change: func(r *domain.Recipe) { r.Name = "  " }, want: problems{"name": domain.Required}},
		{name: "no Ingredient", change: func(r *domain.Recipe) { r.Ingredients = nil }, want: problems{"ingredients": domain.Required}},
		{name: "no kind of Slot", change: func(r *domain.Recipe) { r.Kinds = nil }, want: problems{"slots": domain.Required}},
		{name: "no Portion as written", change: func(r *domain.Recipe) { r.Portions = 0 }, want: problems{"portions": domain.NotWhole}},
		{name: "total minutes of 0", change: func(r *domain.Recipe) { r.TotalMinutes, r.ActiveMinutes = 0, 0 }, want: problems{"total": domain.NotAboveZero}},
		{name: "active minutes above total", change: func(r *domain.Recipe) { r.ActiveMinutes = 41 }, want: problems{"active": domain.AboveTotal}},
		{name: "a quantity of 0", change: func(r *domain.Recipe) { r.Ingredients[1].Quantity = 0 }, rows: map[int]problems{1: {"quantity": domain.NotAboveZero}}},
		{name: "an Ingredient twice", change: func(r *domain.Recipe) {
			r.Ingredients = append(r.Ingredients, domain.RecipeIngredient{Ingredient: chicken, Quantity: grams(100)})
		}, rows: map[int]problems{2: {"name": domain.ListedTwice}}},
		{name: "an Ingredient twice under names that are one food", change: func(r *domain.Recipe) {
			r.Ingredients[0].Ingredient = egg
			large := egg
			large.Name = " Large  Egg"
			r.Ingredients[1].Ingredient = large
		}, rows: map[int]problems{1: {"name": domain.ListedTwice}}},
		{name: "a new Ingredient missing what it needs", change: func(r *domain.Recipe) {
			r.Ingredients[0].Ingredient = domain.Ingredient{Name: "tofu", Unit: domain.Grams, Usual: domain.Fridge}
		}, rows: map[int]problems{0: {"kcal": domain.Required, "protein": domain.Required, "carbs": domain.Required, "fat": domain.Required}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid()
			tt.change(&r)
			got, rows := r.Problems(domain.English)
			if !maps.Equal(got, tt.want) && len(got)+len(tt.want) > 0 {
				t.Errorf("Problems() = %v, want %v", got, tt.want)
			}
			if len(rows) != len(r.Ingredients) {
				t.Fatalf("%d rows of problems for %d Ingredients", len(rows), len(r.Ingredients))
			}
			for i, row := range rows {
				if !maps.Equal(row, tt.rows[i]) && len(row)+len(tt.rows[i]) > 0 {
					t.Errorf("row %d = %v, want %v", i, row, tt.rows[i])
				}
			}
		})
	}
}

func TestParseMinutes(t *testing.T) {
	tests := []struct {
		typed   string
		want    int
		problem domain.Problem
	}{
		{"45", 45, ""}, {" 0 ", 0, ""}, {"", 0, domain.Required}, {"-1", 0, domain.NotWhole},
		{"1.5", 0, domain.NotWhole}, {"soon", 0, domain.NotWhole}, {"10080", 10080, ""}, {"10081", 0, domain.TooLarge},
	}
	for _, tt := range tests {
		if got, problem := domain.ParseMinutes(tt.typed); got != tt.want || problem != tt.problem {
			t.Errorf("ParseMinutes(%q) = %d, %q; want %d, %q", tt.typed, got, problem, tt.want, tt.problem)
		}
	}
}

// Two names that differ only by case and surrounding space are one Recipe's name.
func TestNormaliseRecipeNameIgnoresCaseAndSurroundingSpace(t *testing.T) {
	if a, b := domain.NormaliseRecipeName("  Lasagne al Forno "), domain.NormaliseRecipeName("lasagne al forno"); a != b {
		t.Errorf("normalised to %q and %q", a, b)
	}
}
