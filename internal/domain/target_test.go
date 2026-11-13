package domain_test

import (
	"maps"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

func TestParseTargets(t *testing.T) {
	valid := map[string]string{"kcal": "2200", "protein": "130", "fibre": "30", "sugar": "50", "tolerance": "5"}
	with := func(field, typed string) map[string]string {
		changed := maps.Clone(valid)
		changed[field] = typed
		return changed
	}

	got, problems := domain.ParseTargets(map[string]string{"kcal": " 2200 ", "protein": "130", "fibre": "30", "sugar": "50", "tolerance": "8"})
	want := domain.Targets{Kcal: 2200, Protein: 130, Fibre: 30, Sugar: 50, Tolerance: 8}
	if got != want || len(problems) != 0 {
		t.Errorf("ParseTargets = %+v, %v; want %+v and no problems", got, problems, want)
	}

	cases := []struct {
		field, typed string
		problem      domain.Problem
	}{
		{"kcal", "", domain.Required},
		{"protein", "0", domain.NotAboveZero},
		{"fibre", "-3", domain.NotAboveZero},
		{"sugar", "12.5", domain.NotWhole},
		{"sugar", "lots", domain.NotWhole},
		{"kcal", "10000", domain.TooLarge},
		{"kcal", "99999999999999999999", domain.TooLarge},
		{"tolerance", "", domain.Required},
		{"tolerance", "0", domain.OutOfRange},
		{"tolerance", "21", domain.OutOfRange},
		{"tolerance", "-1", domain.OutOfRange},
		{"tolerance", "2.5", domain.NotWhole},
	}
	for _, c := range cases {
		_, problems := domain.ParseTargets(with(c.field, c.typed))
		if want := map[string]domain.Problem{c.field: c.problem}; !maps.Equal(problems, want) {
			t.Errorf("ParseTargets with %s %q: problems %v; want %v", c.field, c.typed, problems, want)
		}
	}

	for _, edge := range []string{"1", "20"} {
		if _, problems := domain.ParseTargets(with("tolerance", edge)); len(problems) != 0 {
			t.Errorf("ParseTargets with tolerance %q: problems %v; want none", edge, problems)
		}
	}

	// One problem per field, each on its own: no cross-checks.
	_, problems = domain.ParseTargets(map[string]string{"kcal": "", "protein": "0", "fibre": "x", "sugar": "40", "tolerance": "30"})
	if want := map[string]domain.Problem{"kcal": domain.Required, "protein": domain.NotAboveZero, "fibre": domain.NotWhole, "tolerance": domain.OutOfRange}; !maps.Equal(problems, want) {
		t.Errorf("ParseTargets with four refused: problems %v; want %v", problems, want)
	}
}
