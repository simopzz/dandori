package domain

import (
	"maps"
	"slices"
	"time"
)

// Forecast is the kitchen forecast at a Slot: what it will hold of each Stock Item, the
// Portion Lots it will hold without a Slot, those a Plan will have prepared by then by
// PlannedID, and what an ordered Delivery is Arriving with by then, put away, which Stock
// holds already.
type Forecast struct {
	Stock    map[StockKey]Quantity
	Lots     []PortionLot
	Arriving []PutAwayLine
}

// WithOrdered is `f` once Delivery `d` has arrived, if it is ordered: `lines`, its fixed
// Grocery List, put away by the Plan's freezer `splits` (PutAway). A tentative Delivery
// adds nothing: what's missing is what its Grocery List buys.
func (f Forecast) WithOrdered(d Delivery, lines []GroceryLine, splits []RecipeIngredient) Forecast {
	if d.State != Ordered {
		return f
	}
	stock := maps.Clone(f.Stock)
	if stock == nil {
		stock = map[StockKey]Quantity{}
	}
	f.Stock, f.Arriving = stock, PutAway(lines, splits)
	for _, l := range f.Arriving {
		stock[l.Key()] += l.Quantity
	}
	return f
}

// Forecast is the kitchen forecast at the Slot `at`, by `times`, of the kitchen holding
// `stock` and `lots` at `now`, as `p` brings Movements at the Slots after `now` and
// before `at`: its Prep Sessions use up `stock` and make Portions, those not eaten before
// `at` left without a Slot, after `lots`, a Portion Lot for each Prep Session's Storage
// Location, by PlannedID, and the Portions without a Slot it puts in those Slots are
// eaten. What `p` brings before `now` the kitchen is taken to hold already. Stock never
// goes below nothing.
func (p Plan) Forecast(stock map[StockKey]Quantity, lots []PortionLot, times Times, now time.Time, at Slot) Forecast {
	between := func(s Slot) bool { return s.At(times).After(now) && s.Before(at) }
	f := Forecast{Stock: maps.Clone(stock)}
	lots = slices.Clone(lots)
	for _, ps := range p.PrepSessions {
		if !between(ps.At) {
			continue
		}
		for _, ri := range ps.Ingredients() {
			useUp(f.Stock, ri)
		}
		left := ps
		left.Portions = nil
		for _, pp := range ps.Portions {
			if pp.EatAt == nil || !pp.EatAt.Before(at) {
				pp.EatAt = nil
				left.Portions = append(left.Portions, pp)
			}
		}
		lots = append(lots, left.Unslotted()...)
	}
	for _, lot := range lots {
		lot.IDs = slices.DeleteFunc(slices.Clone(lot.IDs), func(id int64) bool {
			return slices.ContainsFunc(p.Kept, func(k Kept) bool { return k.Portion == id && between(k.At) })
		})
		if len(lot.IDs) > 0 {
			f.Lots = append(f.Lots, lot)
		}
	}
	return f
}
