package domain_test

import (
	"slices"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// A typed list of foods, the Profile's exclusions or allergies, is normalised as in v1.
func TestATypedListOfFoodsIsNormalised(t *testing.T) {
	tests := []struct {
		name, typed string
		want        []string
	}{
		{name: "empty", typed: "", want: []string{}},
		{name: "only separators and spaces", typed: " ,\n;\r\n ", want: []string{}},
		{name: "one per line", typed: "peanuts\r\nshellfish\n", want: []string{"peanuts", "shellfish"}},
		{name: "commas and semicolons", typed: "peanuts, shellfish;sesame", want: []string{"peanuts", "shellfish", "sesame"}},
		{name: "case and inner space", typed: "  Sesame   SEEDS ", want: []string{"sesame seeds"}},
		{name: "typographic apostrophes", typed: "fiocchi d’avena\nfiocchi d‘avena", want: []string{"fiocchi d'avena"}},
		{name: "repeats keep the first", typed: "shellfish\nPeanuts\npeanuts\nshellfish", want: []string{"shellfish", "peanuts"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.NormaliseFoods(tt.typed); !slices.Equal(got, tt.want) || got == nil {
				t.Errorf("NormaliseFoods(%q) = %#v, want %#v", tt.typed, got, tt.want)
			}
		})
	}
}
