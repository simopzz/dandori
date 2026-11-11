package domain_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// A User's Language can change until they have written a Food Term (ADR-0015). Each slice
// that brings a Food Term adds its row here.
func TestTheLanguageCanChangeUntilAFoodTermIsWritten(t *testing.T) {
	tests := []struct {
		name  string
		terms domain.FoodTerms
		want  bool
	}{
		{name: "no Food Term", terms: domain.FoodTerms{}, want: true},
		{name: "an Ingredient", terms: domain.FoodTerms{Ingredients: 1}, want: false},
		{name: "a Recipe", terms: domain.FoodTerms{Recipes: 1}, want: false},
		{name: "a Recipe Draft", terms: domain.FoodTerms{Drafts: 1}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.LanguageCanChange(tt.terms); got != tt.want {
				t.Errorf("LanguageCanChange(%+v) = %v, want %v", tt.terms, got, tt.want)
			}
		})
	}
}

func TestParseLanguage(t *testing.T) {
	tests := []struct {
		code   string
		want   domain.Language
		wantOK bool
	}{
		{code: "it", want: domain.Italian, wantOK: true},
		{code: "en", want: domain.English, wantOK: true},
		{code: "fr", wantOK: false},
		{code: "", wantOK: false},
	}
	for _, tt := range tests {
		got, ok := domain.ParseLanguage(tt.code)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("ParseLanguage(%q) = %q, %v, want %q, %v", tt.code, got, ok, tt.want, tt.wantOK)
		}
	}
}
