package domain_test

import (
	"math"
	"slices"
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

func TestRecipeIsCoveredByEnoughStockOfEveryIngredient(t *testing.T) {
	oats := domain.Ingredient{Name: "rolled oats", Unit: domain.Grams}
	milk := domain.Ingredient{Name: "milk", Unit: domain.Millilitres}
	porridge := domain.Recipe{Name: "Porridge", Portions: 1, Ingredients: []domain.RecipeIngredient{
		{Ingredient: oats, Quantity: 80},
		{Ingredient: milk, Quantity: 250},
	}}
	item := func(ing domain.Ingredient, quantity float64, loc domain.StorageLocation) domain.StockItem {
		return domain.StockItem{Ingredient: ing, Quantity: quantity, Location: loc}
	}
	tests := []struct {
		name  string
		stock []domain.StockItem
		want  []string
	}{
		{"nothing in stock", nil, []string{"rolled oats", "milk"}},
		{"enough of both", []domain.StockItem{item(oats, 500, domain.Pantry), item(milk, 250, domain.Fridge)}, nil},
		{"too little milk", []domain.StockItem{item(oats, 500, domain.Pantry), item(milk, 200, domain.Fridge)}, []string{"milk"}},
		{"added up across Storage Locations", []domain.StockItem{
			item(oats, 50, domain.Pantry), item(oats, 30, domain.Freezer), item(milk, 1000, domain.Fridge),
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := porridge.MissingFrom(tt.stock); !slices.Equal(got, tt.want) {
				t.Errorf("MissingFrom = %q, want %q", got, tt.want)
			}
		})
	}
}
