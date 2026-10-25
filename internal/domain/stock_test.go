package domain_test

import (
	"math"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

func TestStockItemQuantityMustBeAPositiveNumber(t *testing.T) {
	tests := []struct {
		name     string
		quantity float64
		want     string
	}{
		{"positive", 0.5, ""},
		{"zero", 0, "Must be more than 0"},
		{"negative", -1, "Must be more than 0"},
		{"not a number", math.NaN(), "Must be a number"},
		{"infinite", math.Inf(1), "Must be a number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := domain.StockItem{Quantity: tt.quantity, Location: domain.Fridge}
			if got := item.Validate()["Quantity"]; got != tt.want {
				t.Errorf("Validate()[Quantity] = %q, want %q", got, tt.want)
			}
		})
	}
}
