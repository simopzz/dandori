package domain

import (
	"maps"
	"slices"
	"time"
)

// Forecast is the kitchen forecast at a Slot: what it will hold of each Stock Item, the
// Portion Lots it will hold without a Slot, and the Portions a Plan will have Made by
// then and left without one, which have no id until they are prepared.
type Forecast struct {
	Stock map[StockKey]Quantity
	Lots  []PortionLot
	Made  []MadeForAfter
}

// Forecast is the kitchen forecast at the Slot at, by times, of the kitchen holding stock
// and lots at now, as p brings Movements at the Slots after now and before at: its Prep
// Sessions use up stock and make Portions, those not eaten before at left without a
// Slot, and the Portions without a Slot it puts in those Slots are eaten. What p brings
// before now the kitchen is taken to hold already. Stock never goes below nothing.
func (p Plan) Forecast(stock map[StockKey]Quantity, lots []PortionLot, times SlotTimes, now time.Time, at Slot) Forecast {
	between := func(s Slot) bool { return s.At(times).After(now) && s.Before(at) }
	f := Forecast{Stock: maps.Clone(stock)}
	var made Plan
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
				left.Portions = append(left.Portions, PlannedPortion{Location: pp.Location})
			}
		}
		made.PrepSessions = append(made.PrepSessions, left)
	}
	f.Made = made.MadeForAfter()
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
