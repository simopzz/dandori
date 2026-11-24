package domain

import (
	"slices"
	"strings"
)

// GroceryList is what a Plan needs beyond what the kitchen will hold: a line for each
// Ingredient short, and the Staples it uses, which the kitchen always holds.
type GroceryList struct {
	Lines   []GroceryLine
	Staples []Ingredient
}

// GroceryLine is how much of an Ingredient a Grocery List buys, in its Unit.
type GroceryLine struct {
	Ingredient Ingredient
	Quantity   Quantity
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
			list.Lines = append(list.Lines, GroceryLine{Ingredient: ing, Quantity: short.RoundUp()})
		}
	}
	return list
}

// PutAwayLine is how much of a Grocery List line goes in one Storage Location when its
// Delivery is put away.
type PutAwayLine struct {
	GroceryLine
	Location StorageLocation
}

// PutAway is where the Delivery bringing `lines` puts them, by the Plan's freezer
// `splits`: each line in its Ingredient's usual Storage Location, but for what a split
// sends to the freezer, never more than the line brings. A split of an Ingredient no
// line buys puts nothing away: stock already in the kitchen stays where it is.
func PutAway(lines []GroceryLine, splits []RecipeIngredient) []PutAwayLine {
	var put []PutAwayLine
	for _, l := range lines {
		var frozen Quantity
		for _, s := range splits {
			if s.Ingredient.ID == l.Ingredient.ID {
				frozen += s.Quantity
			}
		}
		if l.Ingredient.Usual == Freezer {
			frozen = l.Quantity
		}
		frozen = min(frozen, l.Quantity)
		if rest := l.Quantity - frozen; rest > 0 {
			put = append(put, PutAwayLine{GroceryLine{l.Ingredient, rest}, l.Ingredient.Usual})
		}
		if frozen > 0 {
			put = append(put, PutAwayLine{GroceryLine{l.Ingredient, frozen}, Freezer})
		}
	}
	return put
}
