package domain_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

func TestTrailingSevenDayCaloriesMustBeWithinFivePercentOfTarget(t *testing.T) {
	// A 2000 kcal daily Target allows 13300–14700 kcal over seven days.
	tests := []struct {
		name  string
		eaten int
		want  bool
	}{
		{"exactly on Target", 14000, true},
		{"upper bound", 14700, true},
		{"just above upper bound", 14701, false},
		{"lower bound", 13300, true},
		{"just below lower bound", 13299, false},
		{"nothing eaten", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.CaloriesWithinTarget(2000, tt.eaten); got != tt.want {
				t.Errorf("CaloriesWithinTarget(2000, %d) = %v, want %v", tt.eaten, got, tt.want)
			}
		})
	}
}
