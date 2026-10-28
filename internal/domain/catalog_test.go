package domain_test

import (
	"maps"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// lentils is a Catalog Product as the seed stores it from Esselunga.
func lentils() domain.CatalogProduct {
	return domain.CatalogProduct{
		Code:         "412106",
		Name:         "Esselunga Lenticchie 400 g",
		PackQuantity: 400,
		Unit:         domain.Grams,
		Nutrition:    &domain.IngredientNutrition{Kcal: 89, ProteinG: 5, CarbsG: 13.9, FatG: 0.5},
		Aisle:        "Dispensa › Legumi e cereali",
		Price:        0.73,
		Available:    true,
	}
}

func TestCatalogProductIsValidated(t *testing.T) {
	tests := []struct {
		name   string
		change func(*domain.CatalogProduct)
		want   map[string]string
	}{
		{"as seeded", func(*domain.CatalogProduct) {}, map[string]string{}},
		// Fresh produce often comes without a nutrition label.
		{"no nutrition", func(p *domain.CatalogProduct) { p.Nutrition = nil }, map[string]string{}},
		{"no aisle", func(p *domain.CatalogProduct) { p.Aisle = "" }, map[string]string{}},
		{"sold in pieces", func(p *domain.CatalogProduct) { p.PackQuantity, p.Unit = 6, domain.Pieces }, map[string]string{}},
		{"no code", func(p *domain.CatalogProduct) { p.Code = " " }, map[string]string{"Code": "Required"}},
		{"no name", func(p *domain.CatalogProduct) { p.Name = "" }, map[string]string{"Name": "Required"}},
		{"empty pack", func(p *domain.CatalogProduct) { p.PackQuantity = 0 }, map[string]string{"PackQuantity": "Must be more than 0"}},
		{"unknown unit", func(p *domain.CatalogProduct) { p.Unit = "kg" }, map[string]string{"Unit": "Must be g, ml or piece"}},
		{"negative nutrition", func(p *domain.CatalogProduct) { p.Nutrition.FatG = -1 }, map[string]string{"Nutrition": "Can't be negative"}},
		{"negative price", func(p *domain.CatalogProduct) { p.Price = -0.01 }, map[string]string{"Price": "Can't be negative"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := lentils()
			tt.change(&p)
			if got := p.Validate(); !maps.Equal(got, tt.want) {
				t.Errorf("Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProductMappingLinksOnlyAProductSoldInTheIngredientsUnit(t *testing.T) {
	eggs := domain.CatalogProduct{Code: "1", Name: "Esselunga Uova 6", PackQuantity: 6, Unit: domain.Pieces}
	milk := domain.CatalogProduct{Code: "2", Name: "Latte 1000 ml", PackQuantity: 1000, Unit: domain.Millilitres}
	tests := []struct {
		name       string
		ingredient domain.Ingredient
		product    domain.CatalogProduct
		ok         bool
	}{
		{"grams to grams", domain.Ingredient{Name: "lentils", Unit: domain.Grams}, lentils(), true},
		{"pieces to pieces", egg, eggs, true},
		{"millilitres to millilitres", domain.Ingredient{Name: "milk", Unit: domain.Millilitres}, milk, true},
		{"pieces to grams", egg, lentils(), false},
		{"grams to millilitres", chickenBreast, milk, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problem := domain.MappingProblem(tt.ingredient, tt.product)
			if (problem == "") != tt.ok {
				t.Errorf("MappingProblem(%s, %s) = %q, want ok %t", tt.ingredient.Name, tt.product.Name, problem, tt.ok)
			}
		})
	}
	if got := domain.MappingProblem(egg, lentils()); got != "egg is measured in piece, but Esselunga Lenticchie 400 g is sold in g" {
		t.Errorf("MappingProblem says %q", got)
	}
}
