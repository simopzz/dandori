package domain_test

import (
	"slices"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// A near miss suggests the known Ingredient it most likely meant: a letter or two off,
// the words in another order, or a plural. Another food that shares a word is not one.
func TestClosestIngredientSuggestsTheKnownIngredientANearMissMeant(t *testing.T) {
	known := []string{"fiocchi d'avena", "petto di pollo", "riso basmati", "olio extravergine d'oliva"}
	tests := []struct {
		name      string
		typed     string
		want      string
		suggested bool
	}{
		{"a letter missing", "fiocci d'avena", "fiocchi d'avena", true},
		{"a letter missing at the end of a word", "petto di polo", "petto di pollo", true},
		{"words swapped", "basmati riso", "riso basmati", true},
		{"a size word and a typo", "Petto di polo grande", "petto di pollo", true},
		{"a grade and a typo", "olio extra vergine d'olva", "olio extravergine d'oliva", true},
		{"another cut of another meat", "petto di tacchino", "", false},
		{"another food altogether", "zucchine", "", false},
		{"sharing one word", "riso integrale", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := domain.ClosestIngredient(domain.Italian, tt.typed, known)
			if got != tt.want || ok != tt.suggested {
				t.Errorf("ClosestIngredient(%q) = %q, %v; want %q, %v", tt.typed, got, ok, tt.want, tt.suggested)
			}
		})
	}
	if got, ok := domain.ClosestIngredient(domain.Italian, "petto di polo", nil); ok {
		t.Errorf("ClosestIngredient with no known Ingredients = %q, want none", got)
	}
}

// In English, a plural suggests its singular, which normalising keeps apart, but a food
// that only begins with another's name does not.
func TestClosestIngredientInEnglish(t *testing.T) {
	known := []string{"egg", "chicken breast"}
	tests := []struct {
		typed     string
		want      string
		suggested bool
	}{
		{"eggs", "egg", true},
		{"Large eggs", "egg", true},
		{"chiken breast", "chicken breast", true},
		{"egg white", "", false},
	}
	for _, tt := range tests {
		got, ok := domain.ClosestIngredient(domain.English, tt.typed, known)
		if got != tt.want || ok != tt.suggested {
			t.Errorf("ClosestIngredient(%q) = %q, %v; want %q, %v", tt.typed, got, ok, tt.want, tt.suggested)
		}
	}
}

// Of names equally alike, the one first alphabetically is suggested.
func TestClosestIngredientBreaksATieByName(t *testing.T) {
	got, ok := domain.ClosestIngredient(domain.Italian, "basmati", []string{"basmatu", "basmato"})
	if !ok || got != "basmato" {
		t.Errorf(`ClosestIngredient("basmati") = %q, %v; want "basmato", true`, got, ok)
	}
}

// One slip from a known name suggests it, however short the name, but a name of fewer
// than three letters is too short to tell a slip from another food.
func TestClosestIngredientSuggestsANameOneSlipAway(t *testing.T) {
	tests := []struct {
		name, typed, want string
		suggested         bool
	}{
		{"a letter dropped", "aglo", "aglio", true},
		{"a letter added", "agglio", "aglio", true},
		{"a letter changed", "agkio", "aglio", true},
		{"two letters swapped", "algio", "aglio", true},
		{"two slips", "agkao", "", false},
		{"too short", "ag", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := domain.ClosestIngredient(domain.Italian, tt.typed, []string{"aglio", "zucca"})
			if got != tt.want || ok != tt.suggested {
				t.Errorf("ClosestIngredient(%q) = %q, %v; want %q, %v", tt.typed, got, ok, tt.want, tt.suggested)
			}
		})
	}
}

// A name that is a known name, as it normalises, is that one, not its neighbour a slip
// away, so the Service can refuse it as taken.
func TestClosestIngredientPrefersTheSameNameToOneASlipAway(t *testing.T) {
	got, ok := domain.ClosestIngredient(domain.Italian, "Mele", []string{"mela", "mele"})
	if !ok || got != "mele" {
		t.Errorf(`ClosestIngredient("Mele") = %q, %v; want "mele", true`, got, ok)
	}
}

// A search finds the names that contain what was typed, or spelled alike, whole or in a
// run of their words, most alike first; a blank one finds every name, in order.
func TestSearchingIngredientsByName(t *testing.T) {
	names := []string{"egg", "egg white", "fiocchi d'avena", "petto di pollo", "polenta", "riso basmati", "zucchine"}
	tests := []struct {
		name, query string
		want        []string
	}{
		{"blank", "  ", names},
		{"a whole name", "egg", []string{"egg", "egg white"}},
		{"the start of a word, nearer a short one", "pol", []string{"petto di pollo", "polenta"}},
		{"a typo in one word", "polo", []string{"petto di pollo", "polenta"}},
		{"a typo in a whole name", "fiocci d’avena", []string{"fiocchi d'avena"}},
		{"case", "RISO", []string{"riso basmati"}},
		{"nothing alike", "salmone", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, i := range domain.SearchIngredients(tt.query, names) {
				got = append(got, names[i])
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("SearchIngredients(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}
