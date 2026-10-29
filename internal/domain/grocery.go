package domain

import (
	"math"
	"slices"
	"strings"
	"time"
)

// ProductMapping is the Catalog Product preferred for buying an
// Ingredient. Proposed is true while the agent's proposal awaits the user.
type ProductMapping struct {
	Product  CatalogProduct
	Proposed bool
}

// GroceryLine is one Ingredient of a Grocery List: what the Plan needs of
// it beyond stock, in its Unit, and the whole packs of its Product Mapping
// that cover it.
type GroceryLine struct {
	Ingredient Ingredient
	Needed     float64
	// Mapping is the Ingredient's Product Mapping, or nil when it has none:
	// the line is then flagged, and Packs is 0.
	Mapping *ProductMapping
	Packs   int
}

// Bought is how much of the Ingredient the line brings: its whole packs, or
// what is needed when it has no Product Mapping and is bought another way.
func (l GroceryLine) Bought() float64 {
	if l.Mapping == nil {
		return l.Needed
	}
	return float64(l.Packs) * l.Mapping.Product.PackQuantity
}

// Surplus is how much rounding up to whole packs buys beyond what is
// needed.
func (l GroceryLine) Surplus() float64 { return l.Bought() - l.Needed }

// Aisle is the supermarket's category of the line's Catalog Product, or
// empty when it has none.
func (l GroceryLine) Aisle() string {
	if l.Mapping == nil {
		return ""
	}
	return l.Mapping.Product.Aisle
}

// GroceryList is what a Cycle's Plan needs minus current stock, rounded up
// to whole Catalog Products, by Ingredient name.
type GroceryList []GroceryLine

// tolerance absorbs float error in quantities scaled from Recipes, so 300 g
// computed as 299.9999 doesn't round up to another pack, nor 0.0001 g left
// over make a line.
const tolerance = 1e-6

// NewGroceryList is what plan needs of each Ingredient, less the Stock
// Items not expired on the Delivery day, rounded up to whole packs of its
// Product Mapping in mappings, keyed by Ingredient name. A Prep Session of
// a Recipe scales it to the Portions it yields. A Product Mapping to a
// Catalog Product no longer sold in the Ingredient's unit counts as none.
func NewGroceryList(plan Plan, stock []StockItem, mappings map[string]ProductMapping, delivery time.Time) GroceryList {
	needed := map[string]float64{}
	ingredients := map[string]Ingredient{}
	for _, ps := range plan.PrepSessions {
		for _, ri := range ps.uses() {
			needed[ri.Ingredient.Name] += ri.Quantity
			ingredients[ri.Ingredient.Name] = ri.Ingredient
		}
	}
	for _, s := range stock {
		if !s.Expired(delivery) {
			needed[s.Ingredient.Name] -= s.Quantity
		}
	}
	var list GroceryList
	for name, ing := range ingredients {
		if needed[name] <= tolerance {
			continue
		}
		line := GroceryLine{Ingredient: ing, Needed: needed[name]}
		if m, ok := mappings[name]; ok && MappingProblem(ing, m.Product) == "" {
			line.Mapping = &m
			line.Packs = int(math.Ceil(line.Needed/m.Product.PackQuantity - tolerance))
		}
		list = append(list, line)
	}
	slices.SortFunc(list, func(a, b GroceryLine) int { return strings.Compare(a.Ingredient.Name, b.Ingredient.Name) })
	return list
}

// uses is the Ingredients the Prep Session uses: its Recipe's, scaled to
// the Portions it yields, or those it sets out.
func (ps PrepSession) uses() []RecipeIngredient {
	if ps.Recipe == nil {
		return ps.Ingredients
	}
	scale := float64(len(ps.Portions)) / float64(ps.Recipe.Portions)
	var uses []RecipeIngredient
	for _, ri := range ps.Recipe.Ingredients {
		uses = append(uses, RecipeIngredient{Ingredient: ri.Ingredient, Quantity: ri.Quantity * scale})
	}
	return uses
}

// AisleGroup is the lines of a Grocery List in one aisle.
type AisleGroup struct {
	Aisle string
	// Unmapped is whether the group holds the lines with no Product
	// Mapping, flagged to map before Cart Fill.
	Unmapped bool
	Lines    []GroceryLine
}

// ByAisle groups the list's lines by aisle, in aisle order: those of
// Catalog Products with no aisle after them, and those with no Product
// Mapping last.
func (g GroceryList) ByAisle() []AisleGroup {
	var groups []AisleGroup
	for _, l := range g {
		i := slices.IndexFunc(groups, func(a AisleGroup) bool { return a.Aisle == l.Aisle() && a.Unmapped == (l.Mapping == nil) })
		if i < 0 {
			groups = append(groups, AisleGroup{Aisle: l.Aisle(), Unmapped: l.Mapping == nil})
			i = len(groups) - 1
		}
		groups[i].Lines = append(groups[i].Lines, l)
	}
	rank := func(a AisleGroup) int {
		switch {
		case a.Unmapped:
			return 2
		case a.Aisle == "":
			return 1
		}
		return 0
	}
	slices.SortStableFunc(groups, func(a, b AisleGroup) int {
		if r := rank(a) - rank(b); r != 0 {
			return r
		}
		return strings.Compare(a.Aisle, b.Aisle)
	})
	return groups
}

// StockItems are what the list brings once its Delivery arrives: a Stock
// Item of each line's Ingredient, of all it bought, kept where its aisle
// says, with no expiry date.
func (g GroceryList) StockItems() []StockItem {
	var items []StockItem
	for _, l := range g {
		items = append(items, StockItem{Ingredient: l.Ingredient, Quantity: l.Bought(), Location: AisleLocation(l.Aisle())})
	}
	return items
}

// freezerAisles and fridgeAisles are words of the supermarket's aisles
// whose products are kept frozen or chilled; any other aisle's go to the
// pantry. The aisles are the supermarket's own, in Italian.
var (
	freezerAisles = []string{"surgelat", "gelat"}
	fridgeAisles  = []string{"frutta", "verdur", "latte", "latticin", "yogurt", "burro", "panna", "formagg", "salum", "carne", "pesce", "uova", "fresc"}
)

// AisleLocation is the Storage Location a product from aisle is first kept
// in; the user moves it if it belongs elsewhere.
func AisleLocation(aisle string) StorageLocation {
	a := strings.ToLower(aisle)
	has := func(words []string) bool {
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(a, w) })
	}
	switch {
	case has(freezerAisles):
		return Freezer
	case has(fridgeAisles):
		return Fridge
	}
	return Pantry
}
