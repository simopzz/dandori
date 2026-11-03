package domain_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// A User's Language can change until their kitchen holds a food term
// (ADR-0015). Each slice that brings a food term adds its row here.
func TestTheLanguageCanChangeUntilTheKitchenHoldsAFoodTerm(t *testing.T) {
	tests := []struct {
		name    string
		kitchen domain.Kitchen
		want    bool
	}{
		{name: "an empty kitchen", kitchen: domain.Kitchen{}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.LanguageCanChange(tt.kitchen); got != tt.want {
				t.Errorf("LanguageCanChange(%+v) = %v, want %v", tt.kitchen, got, tt.want)
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
