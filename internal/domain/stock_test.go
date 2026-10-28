package domain_test

import (
	"math"
	"slices"
	"testing"
	"time"

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

func TestRecipeIsCoveredByEnoughUnexpiredStockOfEveryIngredient(t *testing.T) {
	today := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	yesterday, tomorrow := today.AddDate(0, 0, -1), today.AddDate(0, 0, 1)
	oats := domain.Ingredient{Name: "rolled oats", Unit: domain.Grams}
	milk := domain.Ingredient{Name: "milk", Unit: domain.Millilitres}
	porridge := domain.Recipe{Name: "Porridge", Portions: 1, Ingredients: []domain.RecipeIngredient{
		{Ingredient: oats, Quantity: 80},
		{Ingredient: milk, Quantity: 250},
	}}
	item := func(ing domain.Ingredient, quantity float64, loc domain.StorageLocation, expires *time.Time) domain.StockItem {
		return domain.StockItem{Ingredient: ing, Quantity: quantity, Location: loc, ExpiresOn: expires}
	}
	tests := []struct {
		name  string
		stock []domain.StockItem
		want  []string
	}{
		{"nothing in stock", nil, []string{"rolled oats", "milk"}},
		{"enough of both", []domain.StockItem{item(oats, 500, domain.Pantry, nil), item(milk, 250, domain.Fridge, &tomorrow)}, nil},
		{"too little milk", []domain.StockItem{item(oats, 500, domain.Pantry, nil), item(milk, 200, domain.Fridge, nil)}, []string{"milk"}},
		{"added up across Storage Locations", []domain.StockItem{
			item(oats, 50, domain.Pantry, nil), item(oats, 30, domain.Freezer, nil), item(milk, 1000, domain.Fridge, nil),
		}, nil},
		{"expiring today still counts", []domain.StockItem{item(oats, 80, domain.Pantry, nil), item(milk, 250, domain.Fridge, &today)}, nil},
		{"expired milk doesn't count", []domain.StockItem{
			item(oats, 80, domain.Pantry, nil), item(milk, 1000, domain.Fridge, &yesterday), item(milk, 100, domain.Fridge, nil),
		}, []string{"milk"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := porridge.MissingFrom(tt.stock, today); !slices.Equal(got, tt.want) {
				t.Errorf("MissingFrom = %q, want %q", got, tt.want)
			}
		})
	}
}
