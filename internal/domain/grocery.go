package domain

import (
	"slices"
	"strings"
)

// GroceryList is what a Plan needs beyond what the kitchen will hold: a line for each
// Ingredient short, and the Staples it uses, which the kitchen always holds.
type GroceryList struct {
	Lines   []RecipeIngredient
	Staples []Ingredient
}

// GroceryList is the Grocery List of `p` against `stock`, the kitchen forecast at its
// first Slot: what its Prep Sessions need of each Ingredient minus what `stock` holds of
// it in every Storage Location, rounded up to a whole unit, listed only above zero, in
// name order. A Staple is never a line: those `p` uses are named apart, in name order.
func (p Plan) GroceryList(stock map[StockKey]Quantity) GroceryList {
	var list GroceryList
	need := map[int64]Quantity{}
	var used []Ingredient
	for _, ps := range p.PrepSessions {
		for _, ri := range ps.Ingredients() {
			if _, ok := need[ri.Ingredient.ID]; !ok {
				used = append(used, ri.Ingredient)
			}
			need[ri.Ingredient.ID] += ri.Quantity
		}
	}
	slices.SortFunc(used, func(a, b Ingredient) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	for _, ing := range used {
		if ing.Staple {
			list.Staples = append(list.Staples, ing)
			continue
		}
		short := need[ing.ID]
		for _, loc := range StorageLocations {
			short -= max(0, stock[StockKey{Ingredient: ing.ID, Location: loc}])
		}
		if short > 0 {
			list.Lines = append(list.Lines, RecipeIngredient{Ingredient: ing, Quantity: (short + tenths - 1) / tenths * tenths})
		}
	}
	return list
}
