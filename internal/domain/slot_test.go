package domain_test

import (
	"testing"

	"github.com/simopzz/mealops/internal/domain"
)

func TestOnlyLunchAndDinnerSlotsAreFilledByPortions(t *testing.T) {
	tests := []struct {
		kind domain.SlotKind
		want bool
	}{
		{domain.Breakfast, false},
		{domain.Lunch, true},
		{domain.Dinner, true},
		{domain.Snack, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if got := tt.kind.FilledByPortion(); got != tt.want {
				t.Errorf("%s Slot filled by Portion = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}
