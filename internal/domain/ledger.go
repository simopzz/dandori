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
// grams and millilitres are whole numbers, pieces have one decimal place, and none is
// negative.
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

// Cause is the kind of thing that caused a Movement. The Kitchen Ledger stores it as data,
// so a page can tell it in the User's Language.
type Cause string

// Counted is the Cause of a Count.
const Counted Cause = "counted"

// StockKey identifies a Stock Item: there is one per Ingredient and Storage Location.
type StockKey struct {
	Ingredient int64
	Location   StorageLocation
}

// Movement is one change to what the kitchen holds, as AddUp needs it.
type Movement struct {
	At     time.Time
	Key    StockKey
	Change Quantity
	Cause  Cause
}

// StockItem is what one Ingredient's Movements in one Storage Location add up to.
type StockItem struct {
	Held Quantity
	// Counted is when it was last counted; it is zero if it never was.
	Counted time.Time
}

// CountDifference is the change a Count makes: what the User stated less what Dandori
// believed.
func CountDifference(believed, stated Quantity) Quantity { return stated - believed }

// AddUp is what the kitchen holds at a moment: what movements, in the order they
// happened, add up to by then, by Stock Item. One Movement at that very moment counts.
func AddUp(movements []Movement, at time.Time) map[StockKey]StockItem {
	items := map[StockKey]StockItem{}
	for _, m := range movements {
		if m.At.After(at) {
			continue
		}
		item := items[m.Key]
		item.Held += m.Change
		if m.Cause == Counted {
			item.Counted = m.At
		}
		items[m.Key] = item
	}
	return items
}
