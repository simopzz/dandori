package domain

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Quantity is how much of an Ingredient, in its Unit, as a whole number of tenths: what
// a Stock Item holds, or a Movement's change, which may be below nothing. Tenths add up
// exactly, as the floats a piece's decimal place would be don't.
type Quantity int64

// tenths is how many of a Quantity make one of its Unit.
const tenths = 10

// maxQuantity bounds a quantity the User states: no kitchen holds a tonne of anything.
const maxQuantity = 1000000

// Number is q in its Unit.
func (q Quantity) Number() float64 { return float64(q) / tenths }

// QuantityOf is the Quantity of v in its Unit, as Number gives it.
func QuantityOf(v float64) Quantity { return Quantity(math.Round(v * tenths)) }

// ParseQuantity reads a quantity of unit as the User typed it, with a point or a comma:
// grams and millilitres are whole numbers, pieces have one decimal place, and nothing
// is negative.
func ParseQuantity(unit Unit, typed string) (Quantity, Problem) {
	typed = strings.ReplaceAll(strings.TrimSpace(typed), ",", ".")
	if typed == "" {
		return 0, Required
	}
	v, err := strconv.ParseFloat(typed, 64)
	switch {
	case err != nil || math.IsNaN(v) || math.IsInf(v, 0):
		return 0, NotANumber
	case v < 0:
		return 0, Negative
	case v >= maxQuantity:
		return 0, TooLarge
	case unit != Pieces && v != math.Trunc(v):
		return 0, NotWhole
	case math.Abs(v*tenths-math.Round(v*tenths)) > 1e-9:
		return 0, TooPrecise
	}
	return QuantityOf(v), ""
}

// Cause is the kind of thing that caused a Movement. The Kitchen Ledger stores it as
// data, so a page can tell it in the User's Language.
type Cause string

// The Causes of a Movement.
const (
	// Counted is the Cause of a Count.
	Counted Cause = "counted"
	// Moved is the Cause of a move.
	Moved Cause = "moved"
)

// Why a move is refused.
const (
	NothingMoved Problem = "nothing moved"
	MoreThanHeld Problem = "more than held"
)

// StockKey identifies a Stock Item: there is one per Ingredient and Storage Location.
type StockKey struct {
	Ingredient int64
	Location   StorageLocation
}

// Movement is one change to what the kitchen holds. A move is one Movement: Key is the
// Stock Item it comes from, To the Storage Location it goes to, and Change, above
// nothing, how much goes.
type Movement struct {
	At     time.Time
	Key    StockKey
	To     StorageLocation
	Change Quantity
	Cause  Cause
	// Believed is what a Count kept of what Dandori believed before it.
	Believed Quantity
	// Note is the User's, if any.
	Note string
}

// StockMovement is a Movement of one Stock Item with what it held before and after.
type StockMovement struct {
	Movement
	Before, After Quantity
}

// StockItem is what one Ingredient's Movements in one Storage Location add up to.
type StockItem struct {
	Held Quantity
	// Counted is when it was last counted; it is zero if it never was.
	Counted time.Time
}

// CountDifference is the change a Count makes: what was stated less what was believed.
func CountDifference(believed, stated Quantity) Quantity { return stated - believed }

// CheckMove says why a move of moved is refused from a Stock Item holding held, if it is.
func CheckMove(held, moved Quantity) Problem {
	switch {
	case moved <= 0:
		return NothingMoved
	case moved > held:
		return MoreThanHeld
	}
	return ""
}

// changeTo is what m changes the Stock Item key by; touches is false if it leaves key
// alone. A move touches both of its Stock Items.
func (m Movement) changeTo(key StockKey) (change Quantity, touches bool) {
	switch {
	case m.Key == key && m.Cause == Moved:
		return -m.Change, true
	case m.Key == key:
		return m.Change, true
	case m.Cause == Moved && m.Key.Ingredient == key.Ingredient && m.To == key.Location:
		return m.Change, true
	}
	return 0, false
}

// StockMovements is every one of movements, in the order they happened, that touched the
// Stock Item key, with what it held before and after each. A Count starts from what it
// kept of what Dandori believed.
func StockMovements(movements []Movement, key StockKey) []StockMovement {
	var (
		touched []StockMovement
		held    Quantity
	)
	for _, m := range movements {
		change, touches := m.changeTo(key)
		if !touches {
			continue
		}
		if m.Cause == Counted {
			held = m.Believed
		}
		touched = append(touched, StockMovement{Movement: m, Before: held, After: held + change})
		held += change
	}
	return touched
}

// AddUp is what the kitchen holds at a moment: what movements, in the order they
// happened, add up to by then, by Stock Item. One Movement at that very moment counts.
func AddUp(movements []Movement, at time.Time) map[StockKey]StockItem {
	items := map[StockKey]StockItem{}
	for _, m := range movements {
		if m.At.After(at) {
			continue
		}
		item := items[m.Key]
		if m.Cause == Moved {
			to := StockKey{Ingredient: m.Key.Ingredient, Location: m.To}
			target := items[to]
			target.Held += m.Change
			items[to] = target
			item.Held -= m.Change
		} else {
			item.Held += m.Change
		}
		if m.Cause == Counted {
			item.Counted = m.At
		}
		items[m.Key] = item
	}
	return items
}
