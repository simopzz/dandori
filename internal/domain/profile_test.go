package domain_test

import (
	"testing"

	"github.com/simopzz/mealops/internal/domain"
)

func TestFoodTermsAreNormalisedAndDeduplicated(t *testing.T) {
	got := domain.FoodTerms(" Peanuts ,\n\nshellfish; PEANUTS\r\n  Sesame   Seeds ")
	want := []string{"peanuts", "shellfish", "sesame seeds"}
	if len(got) != len(want) {
		t.Fatalf("FoodTerms = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FoodTerms = %q, want %q", got, want)
		}
	}
}

func TestProfileValidation(t *testing.T) {
	valid := domain.Profile{
		Targets:            domain.Nutrition{Kcal: 2000, ProteinG: 150, CarbsG: 200, FatG: 70},
		MealOut:            domain.Nutrition{Kcal: 900, ProteinG: 35, CarbsG: 90, FatG: 40},
		UntrackedAllowance: domain.Nutrition{Kcal: 150, ProteinG: 5, CarbsG: 15, FatG: 5},
	}
	tests := []struct {
		name   string
		change func(*domain.Profile)
		want   map[string]string
	}{
		{"valid", func(*domain.Profile) {}, map[string]string{}},
		{"zero calorie Target", func(p *domain.Profile) { p.Targets.Kcal = 0 }, map[string]string{"Targets.Kcal": "Must be more than 0"}},
		{"zero macro Targets are allowed", func(p *domain.Profile) { p.Targets.CarbsG = 0 }, map[string]string{}},
		{"negative Meal Out protein", func(p *domain.Profile) { p.MealOut.ProteinG = -1 }, map[string]string{"MealOut.ProteinG": "Can't be negative"}},
		{"Untracked Allowance just under the calorie Target", func(p *domain.Profile) { p.UntrackedAllowance.Kcal = 1999 }, map[string]string{}},
		{"Untracked Allowance equal to the calorie Target", func(p *domain.Profile) { p.UntrackedAllowance.Kcal = 2000 }, map[string]string{"UntrackedAllowance.Kcal": "Must be less than the daily calorie Target"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.change(&p)
			got := p.Validate()
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
