package domain_test

import (
	"maps"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

func TestParseMealOut(t *testing.T) {
	valid := map[string]string{"kcal": "1000", "protein": "35", "carbs": "110", "fat": "45", "fibre": "6", "sugar": "12"}
	with := func(field, typed string) map[string]string {
		changed := maps.Clone(valid)
		changed[field] = typed
		return changed
	}

	got, problems := domain.ParseMealOut(with("kcal", " 1000 "))
	want := domain.Nutrition{
		Kcal: domain.Known(1000), Protein: domain.Known(35), Carbs: domain.Known(110), Fat: domain.Known(45),
		Fibre: domain.Known(6), Sugar: domain.Known(12),
	}
	if got != want || len(problems) != 0 {
		t.Errorf("ParseMealOut = %+v, %v; want %+v and no problems", got, problems, want)
	}

	// Fibre and sugar left blank are unknown, never 0; 0 typed is 0.
	got, problems = domain.ParseMealOut(map[string]string{"kcal": "0", "protein": "0", "carbs": "0", "fat": "0", "fibre": " ", "sugar": ""})
	want = domain.Nutrition{Kcal: domain.Known(0), Protein: domain.Known(0), Carbs: domain.Known(0), Fat: domain.Known(0)}
	if got != want || len(problems) != 0 {
		t.Errorf("ParseMealOut with fibre and sugar blank = %+v, %v; want %+v and no problems", got, problems, want)
	}

	cases := []struct {
		field, typed string
		problem      domain.Problem
	}{
		{"kcal", "", domain.Required},
		{"protein", " ", domain.Required},
		{"carbs", "-1", domain.Negative},
		{"fat", "4.5", domain.NotWhole},
		{"kcal", "lots", domain.NotWhole},
		{"kcal", "10000", domain.TooLarge},
		{"fat", "99999999999999999999", domain.TooLarge},
		{"fibre", "-2", domain.Negative},
		{"fibre", "2.5", domain.NotWhole},
		{"sugar", "10000", domain.TooLarge},
	}
	for _, c := range cases {
		_, problems := domain.ParseMealOut(with(c.field, c.typed))
		if want := map[string]domain.Problem{c.field: c.problem}; !maps.Equal(problems, want) {
			t.Errorf("ParseMealOut with %s %q: problems %v; want %v", c.field, c.typed, problems, want)
		}
	}
}
