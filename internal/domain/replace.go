package domain

import (
	"cmp"
	"slices"
	"time"
)

// Replacement is what a Plan Proposal from a Slot on does to what stands:
// the part of the Plan it replaces, and the Portions it may put in Slots.
type Replacement struct {
	// From is the first Slot it fills; Prepared the first it may prepare a
	// Prep Session at.
	From, Prepared Slot
	// Dropped are the Cycle's Prep Sessions from Prepared on: never
	// prepared, they go with their Portions.
	Dropped []int64
	// Freed are the Portions of Prep Sessions that stay that lose their
	// Slot of the Cycle from From on.
	Freed []int64
	// Kept are the Portions it may put in Slots, by id: those Freed, and
	// those of Prep Sessions that stay without a Slot.
	Kept []KeptPortion
}

// Replace is what a Plan Proposal of c from the Slot from on, decided on
// today, replaces of c's Plan, whose Prep Sessions are own; others are the
// Prep Sessions of other Cycles and from before planning. It replaces the
// Plan only from a Slot on: the Prep Sessions before it stay, and so do
// today's, already taken as prepared, and their Portions not yet eaten are
// the Plan Proposal's to put in Slots again. Another Cycle's Prep Sessions
// from Prepared on, the next Cycle's, are left as they are.
func Replace(c Cycle, from Slot, today time.Time, own, others []PrepSession) Replacement {
	r := Replacement{From: from, Prepared: FirstUnprepared(from, today)}
	stays := func(ps PrepSession) {
		for _, p := range ps.Portions {
			switch {
			case p.EatAt == nil:
			case c.Contains(*p.EatAt) && !p.EatAt.Before(from):
				r.Freed = append(r.Freed, p.ID)
			default:
				continue
			}
			r.Kept = append(r.Kept, KeptPortion{ID: p.ID, Name: ps.Name(), Nutrition: ps.NutritionPerPortion()})
		}
	}
	for _, ps := range own {
		if ps.At.Before(r.Prepared) {
			stays(ps)
		} else {
			r.Dropped = append(r.Dropped, ps.ID)
		}
	}
	for _, ps := range others {
		// A Prep Session from before planning has no day, so it comes first.
		if ps.At.Before(r.Prepared) {
			stays(ps)
		}
	}
	slices.Sort(r.Freed)
	slices.SortFunc(r.Kept, func(a, b KeptPortion) int { return cmp.Compare(a.ID, b.ID) })
	return r
}
