package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// CatalogProduct is one purchasable supermarket product in the Known
// Catalog. Its name is the supermarket's own, never translated.
type CatalogProduct struct {
	// Code is the supermarket's product code; it identifies the product.
	Code string
	Name string
	// PackQuantity is how much one pack holds, in Unit: a "6 x 125 g"
	// multipack is 750 g.
	PackQuantity float64
	Unit         Unit
	// Nutrition is per 100 g, per 100 ml or per piece, as Unit says; nil
	// when the supermarket shows none, as for most fresh produce.
	Nutrition *IngredientNutrition
	// Aisle is the supermarket's category for the product, such as
	// "Dispensa › Legumi e cereali"; empty when it gives none.
	Aisle string
	// Price is the cached price of one pack, in euros. It is not
	// load-bearing (ADR-0004).
	Price float64
	// Available is the availability last seen, at AvailabilitySeen.
	Available        bool
	AvailabilitySeen time.Time
}

// Validate returns what is wrong with the Catalog Product, keyed by field.
// A valid one returns none.
func (p CatalogProduct) Validate() map[string]string {
	problems := map[string]string{}
	if strings.TrimSpace(p.Code) == "" {
		problems["Code"] = "Required"
	}
	if strings.TrimSpace(p.Name) == "" {
		problems["Name"] = "Required"
	}
	if p.PackQuantity <= 0 {
		problems["PackQuantity"] = "Must be more than 0"
	}
	if !slices.Contains(Units, p.Unit) {
		problems["Unit"] = "Must be g, ml or piece"
	}
	if n := p.Nutrition; n != nil && (n.Kcal < 0 || n.ProteinG < 0 || n.CarbsG < 0 || n.FatG < 0) {
		problems["Nutrition"] = "Can't be negative"
	}
	if p.Price < 0 {
		problems["Price"] = "Can't be negative"
	}
	return problems
}

// MappingProblem says why ing can't have p as its Product Mapping, or ""
// when it can: the Grocery List rounds an Ingredient's quantity up to whole
// packs, so the pack must be sold in the Ingredient's unit.
func MappingProblem(ing Ingredient, p CatalogProduct) string {
	if ing.Unit != p.Unit {
		return fmt.Sprintf("%s is measured in %s, but %s is sold in %s", ing.Name, ing.Unit, p.Name, p.Unit)
	}
	return ""
}
