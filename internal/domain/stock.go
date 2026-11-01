package domain

import (
	"math"
	"slices"
)

// StorageLocation is where a Portion or Stock Item is kept.
type StorageLocation string

const (
	Pantry  StorageLocation = "pantry"
	Fridge  StorageLocation = "fridge"
	Freezer StorageLocation = "freezer"
)

// StorageLocations lists every StorageLocation.
var StorageLocations = []StorageLocation{Pantry, Fridge, Freezer}

// Validate returns what is wrong with l, or "" when it is one of the
// StorageLocations.
func (l StorageLocation) Validate() string {
	if !slices.Contains(StorageLocations, l) {
		return "Must be pantry, fridge or freezer"
	}
	return ""
}

// StockItem is a quantity of one Ingredient, in its Unit, held in a
// Storage Location.
type StockItem struct {
	Ingredient Ingredient
	Quantity   float64
	Location   StorageLocation
}

// Validate returns what is wrong with the Stock Item, keyed by field name
// such as "Quantity". A valid Stock Item returns none.
func (s StockItem) Validate() map[string]string {
	problems := map[string]string{}
	switch {
	case math.IsNaN(s.Quantity) || math.IsInf(s.Quantity, 0):
		problems["Quantity"] = "Must be a number"
	case s.Quantity <= 0:
		problems["Quantity"] = "Must be more than 0"
	}
	if msg := s.Location.Validate(); msg != "" {
		problems["Location"] = msg
	}
	return problems
}

// Taking is what a Prep Session took of one Stock Item: how much, with the
// Stock Item's Ingredient and Storage Location, so it can be given back as
// it was.
type Taking struct {
	StockItem
	// From is the index of the Stock Item in the stock it was taken from.
	From int
	// Leaves is what the Stock Item holds after it: 0 once it is used up.
	Leaves float64
}

// StockUse is what a Prep Session used up of stock.
type StockUse struct {
	Taken []Taking
	// Shortfall is what it needed of each Ingredient beyond the stock it
	// found, by Ingredient name, or nil when stock held enough.
	Shortfall map[string]float64
}

// UsingUpBefore is the Prep Sessions of sessions that use up their stock
// before the Slot until, in the order of their days: those prepared at a
// Slot before it, and before upcomingStart, the upcoming Cycle's first
// Slot, when there is an upcoming Cycle, since its Delivery drops the rest
// when it arrives. That also leaves out the upcoming Cycle's own Prep
// Sessions: each lies within its Cycle's Slots, and Cycles never overlap.
// A Prep Session from before planning has no day and uses up nothing.
func UsingUpBefore(sessions []PrepSession, until Slot, upcomingStart *Slot) []PrepSession {
	var using []PrepSession
	for _, ps := range sessions {
		if !ps.At.On.IsZero() && ps.At.Before(until) && (upcomingStart == nil || ps.At.Before(*upcomingStart)) {
			using = append(using, ps)
		}
	}
	SortInDayOrder(using)
	return using
}

// UseUp has sessions, in the order of their days, use up the Ingredients
// they use from stock, in the order it was recorded, and returns the Stock
// Items left, in that order, and what each Prep Session used, in its
// order. Each Ingredient comes from its Stock Items the oldest first,
// whatever their Storage Location. A Stock Item used up leaves the
// kitchen; stock that falls short is used up as far as it goes. A Prep
// Session from before planning has no day and uses nothing.
func UseUp(sessions []PrepSession, stock []StockItem) ([]StockItem, []StockUse) {
	held := slices.Clone(stock)
	uses := make([]StockUse, len(sessions))
	for i, ps := range sessions {
		if ps.At.On.IsZero() {
			continue
		}
		for _, ri := range ps.uses() {
			need := ri.Quantity
			for _, j := range drawOrder(held, ri.Ingredient.Name) {
				if need <= tolerance {
					break
				}
				take := min(need, held[j].Quantity)
				if held[j].Quantity-take <= tolerance {
					take = held[j].Quantity
				}
				held[j].Quantity -= take
				need -= take
				t := Taking{StockItem: stock[j], From: j, Leaves: held[j].Quantity}
				t.Quantity = take
				uses[i].Taken = append(uses[i].Taken, t)
			}
			if need > tolerance {
				if uses[i].Shortfall == nil {
					uses[i].Shortfall = map[string]float64{}
				}
				uses[i].Shortfall[ri.Ingredient.Name] += need
			}
		}
	}
	var left []StockItem
	for _, s := range held {
		if s.Quantity > 0 {
			left = append(left, s)
		}
	}
	return left, uses
}

// drawOrder is the indexes of the Stock Items of the Ingredient named name
// in held that still hold some, in the order they are used up: the oldest
// first.
func drawOrder(held []StockItem, name string) []int {
	var order []int
	for j, s := range held {
		if s.Ingredient.Name == name && s.Quantity > 0 {
			order = append(order, j)
		}
	}
	return order
}

// MissingFrom names the Recipe's Ingredients, in its order, that stock
// doesn't cover: those whose Stock Items, across every Storage Location,
// add up to less than the Recipe's quantity as written. A Recipe with none
// missing is covered by stock.
func (r Recipe) MissingFrom(stock []StockItem) []string {
	held := map[string]float64{}
	for _, s := range stock {
		held[s.Ingredient.Name] += s.Quantity
	}
	var missing []string
	for _, ri := range r.Ingredients {
		if held[ri.Ingredient.Name] < ri.Quantity {
			missing = append(missing, ri.Ingredient.Name)
		}
	}
	return missing
}
