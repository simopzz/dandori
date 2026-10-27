package domain

import (
	"errors"
	"time"
)

// Cycle is the stretch one grocery delivery has to last. Its dates are
// days, at midnight UTC.
type Cycle struct {
	DeliveryOn time.Time
	// EndsOn is the planned end date, the Cycle's last day.
	EndsOn time.Time
	// ClosedOn is the delivery date of the next Cycle when it arrived before
	// EndsOn, closing this one early; nil otherwise.
	ClosedOn *time.Time
}

// defaultCycleDays is how long a Cycle lasts unless the user says otherwise.
const defaultCycleDays = 10

// DefaultCycleEnd is the planned end date of a Cycle delivered on delivery,
// unless the user picks another.
func DefaultCycleEnd(delivery time.Time) time.Time {
	return delivery.AddDate(0, 0, defaultCycleDays)
}

// Validate returns what is wrong with the Cycle, keyed by field name. A
// valid Cycle returns none.
func (c Cycle) Validate() map[string]string {
	problems := map[string]string{}
	if !c.EndsOn.After(c.DeliveryOn) {
		problems["EndsOn"] = "Must be after the delivery date"
	}
	return problems
}

// LastDay is the last day the Cycle lasts: its end date, or the day before
// the delivery that closed it early.
func (c Cycle) LastDay() time.Time {
	if c.ClosedOn != nil {
		return c.ClosedOn.AddDate(0, 0, -1)
	}
	return c.EndsOn
}

// ErrCycleOverlaps means a new Cycle would be delivered on or before the
// current Cycle's delivery date.
var ErrCycleOverlaps = errors.New("cycle overlaps the current one")

// Opening is what opening a Cycle does to the current one and its Plan.
type Opening struct {
	// ClosesCurrent is whether the new delivery arrives before the current
	// Cycle ends, closing it early.
	ClosesCurrent bool
	// DroppedPrepSessions are the current Plan's Prep Sessions from the new
	// delivery date on: they are never prepared.
	DroppedPrepSessions []int64
	// CarriedOver are the Portions the current Plan has eaten from the new
	// delivery date on but prepared before it: leftovers, which lose their
	// Slot.
	CarriedOver []int64
}

// OpenCycle opens next after current, the latest Cycle with its Plan, or
// after none when current is nil.
func OpenCycle(current *Cycle, plan Plan, next Cycle) (Opening, error) {
	var o Opening
	if current == nil {
		return o, nil
	}
	if !next.DeliveryOn.After(current.DeliveryOn) {
		return o, ErrCycleOverlaps
	}
	if next.DeliveryOn.After(current.LastDay()) {
		return o, nil
	}
	o.ClosesCurrent = true
	from := Slot{On: next.DeliveryOn, Kind: SlotKinds[0]}
	for _, ps := range plan.PrepSessions {
		if !ps.At.Before(from) {
			o.DroppedPrepSessions = append(o.DroppedPrepSessions, ps.ID)
			continue
		}
		for _, p := range ps.Portions {
			if p.EatAt != nil && !p.EatAt.Before(from) {
				o.CarriedOver = append(o.CarriedOver, p.ID)
			}
		}
	}
	for _, k := range plan.Kept {
		if !k.Slot.Before(from) {
			o.CarriedOver = append(o.CarriedOver, k.PortionID)
		}
	}
	return o, nil
}
