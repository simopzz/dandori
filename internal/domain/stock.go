package domain

import (
	"math"
	"slices"
	"time"
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
	// ExpiresOn is the expiry date, or nil when there is none.
	ExpiresOn *time.Time
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

// Expired reports whether the Stock Item is past its expiry date on today.
// An expired Stock Item stays in the kitchen, but no Plan uses it.
func (s StockItem) Expired(today time.Time) bool {
	return s.ExpiresOn != nil && s.ExpiresOn.Before(today)
}

// Taking is what a Prep Session took of one Stock Item: how much, with the
// Stock Item's Ingredient, Storage Location and expiry date, so it can be
// given back as it was.
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
// order. Each Ingredient comes from its Stock Items with the earliest
// expiry first, those without one last, then the oldest, skipping any
// expired on the Prep Session's day, whatever their Storage Location. A
// Stock Item used up leaves the kitchen; stock that falls short is used up
// as far as it goes. A Prep Session from before planning has no day and
// uses nothing.
func UseUp(sessions []PrepSession, stock []StockItem) ([]StockItem, []StockUse) {
	held := slices.Clone(stock)
	uses := make([]StockUse, len(sessions))
	for i, ps := range sessions {
		if ps.At.On.IsZero() {
			continue
		}
		for _, ri := range ps.uses() {
			need := ri.Quantity
			for _, j := range drawOrder(held, ri.Ingredient.Name, ps.At.On) {
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
// in held that still hold some and aren't expired on day, in the order they
// are used up: the earliest expiry first, those without one last, then the
// oldest.
func drawOrder(held []StockItem, name string, day time.Time) []int {
	var order []int
	for j, s := range held {
		if s.Ingredient.Name == name && s.Quantity > 0 && !s.Expired(day) {
			order = append(order, j)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int {
		ea, eb := held[a].ExpiresOn, held[b].ExpiresOn
		switch {
		case ea == nil && eb == nil:
			return 0
		case ea == nil:
			return 1
		case eb == nil:
			return -1
		}
		return ea.Compare(*eb)
	})
	return order
}

// MissingFrom names the Recipe's Ingredients, in its order, that stock
// doesn't cover on today: those whose unexpired Stock Items, across every
// Storage Location, add up to less than the Recipe's quantity as written.
// A Recipe with none missing is covered by stock.
func (r Recipe) MissingFrom(stock []StockItem, today time.Time) []string {
	held := map[string]float64{}
	for _, s := range stock {
		if !s.Expired(today) {
			held[s.Ingredient.Name] += s.Quantity
		}
	}
	var missing []string
	for _, ri := range r.Ingredients {
		if held[ri.Ingredient.Name] < ri.Quantity {
			missing = append(missing, ri.Ingredient.Name)
		}
	}
	return missing
}
