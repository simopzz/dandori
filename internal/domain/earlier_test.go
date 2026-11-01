package domain_test

import (
	"slices"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

func TestTheTrailingSevenDaysCountWhatEarlierSlotsEat(t *testing.T) {
	// The Cycle starts at breakfast on Sun 8 Nov: a trailing seven days of
	// it reaches back to Mon 2 Nov. It is planned from dinner on Tue 10 Nov.
	c := domain.Cycle{Start: *at("2026-11-08", domain.Breakfast), EndsOn: day("2026-11-14")}
	from := *at("2026-11-10", domain.Dinner)
	// Lemons hold 20 kcal a Portion.
	lemon := domain.RecipeIngredient{Ingredient: domain.Ingredient{Name: "lemon", Unit: domain.Pieces, Nutrition: domain.IngredientNutrition{Kcal: 20}}, Quantity: 1}
	eaten := func(on string, kind domain.SlotKind) domain.PrepSession {
		return domain.PrepSession{At: *at(on, kind), Ingredients: []domain.RecipeIngredient{lemon}, Portions: []domain.PlannedPortion{{EatAt: at(on, kind)}}}
	}
	tests := []struct {
		name       string
		eaten      domain.PrepSession
		exceptions []domain.Exception
		want       []string
	}{
		{"six days before the Cycle's first day is reached", eaten("2026-11-02", domain.Dinner), nil, []string{"2026-11-02"}},
		{"seven days before is not", eaten("2026-11-01", domain.Dinner), nil, nil},
		{"a Slot of an earlier Cycle counts", eaten("2026-11-05", domain.Lunch), nil, []string{"2026-11-05"}},
		{
			"a Portion an earlier Cycle prepared counts in the Cycle's Slot it fills",
			domain.PrepSession{At: *at("2026-11-05", domain.Dinner), Ingredients: []domain.RecipeIngredient{lemon}, Portions: []domain.PlannedPortion{{EatAt: at("2026-11-09", domain.Lunch)}}},
			nil,
			[]string{"2026-11-09"},
		},
		{"the Cycle's own Slot before from counts", eaten("2026-11-10", domain.Snack), nil, []string{"2026-11-10"}},
		{"the Slot at from is the Plan's, not earlier", eaten("2026-11-10", domain.Dinner), nil, nil},
		{
			"an excepted Slot's Portion isn't eaten",
			eaten("2026-11-05", domain.Lunch),
			[]domain.Exception{{Slot: *at("2026-11-05", domain.Lunch), Kind: domain.MealOut}},
			nil,
		},
		{
			"an Exception on another Slot of the day leaves it eaten",
			eaten("2026-11-05", domain.Lunch),
			[]domain.Exception{{Slot: *at("2026-11-05", domain.Dinner), Kind: domain.MealOut}},
			[]string{"2026-11-05"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, d := range domain.Earlier(c, from, []domain.PrepSession{tt.eaten}, tt.exceptions) {
				if d.Nutrition.Kcal != 20 {
					t.Errorf("%s eats %d kcal, want a lemon's 20", d.On.Format("2006-01-02"), d.Nutrition.Kcal)
				}
				got = append(got, d.On.Format("2006-01-02"))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Earlier eats on %v, want %v", got, tt.want)
			}
		})
	}
}
