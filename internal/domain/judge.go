package domain

import (
	"fmt"
	"time"
)

// Verdict is what a draft Plan breaks and misses against the rules of the model: the hard
// rules it Broke, the seven days before its first Slot and the days it plans, the Targets
// and soft rules it Misses, and the Portions it makes for after the Cycle.
type Verdict struct {
	Broken         []Violation
	Trailing, Days []Day
	Misses         []Miss
	MadeForAfter   []MadeForAfter
}

// Judge is the Verdict on `plan`, a draft of `cycle`'s Slots by `profile`'s Slot times
// from the Slot `from` on, replacing `current`, the Cycle's Plan if it has one, at `now`:
// `lots` are the Portion Lots the kitchen holds without a Slot, `stock` what it holds of
// each Stock Item, and `earlier` what approved Plans put in Slots before. The Portions it
// may put in Slots are those `current.Replace` frees, and the Prep Sessions it keeps
// count towards back to back. On a current Cycle, no Delivery brings what the kitchen
// doesn't hold, and its Delivery is put away, splitting nothing more. On an upcoming
// Cycle whose Delivery is ordered, it brings only what `stock` holds already, but its
// freezer splits may still change.
func Judge(plan Plan, cycle Cycle, from Slot, profile Profile, current Plan, lots []PortionLot, stock map[StockKey]Quantity, earlier []Eaten, now time.Time) Verdict {
	var slots []Slot
	for _, s := range cycle.Slots(profile.SlotTimes) {
		if !s.Before(from) {
			slots = append(slots, s)
		}
	}
	last := from
	if len(slots) > 0 {
		last = slots[len(slots)-1]
	}
	kept, free := current.Replace(from, last, lots)
	var unslotted []Portion
	frozen := map[int64]bool{}
	for _, lot := range free {
		for _, id := range lot.IDs {
			unslotted = append(unslotted, Portion{ID: id, Name: lot.Name, Nutrition: lot.Nutrition})
			frozen[id] = lot.Location == Freezer
		}
	}
	var v Verdict
	v.Broken = append(HardRules(plan, slots, unslotted), RecipesUnfit(plan, profile.Appliances)...)
	v.Broken = append(v.Broken, NoEveningToThaw(plan, frozen, stock, now, cycle.Delivery.Arrives())...)
	arrived := !cycle.Delivery.Arrives().After(now)
	if arrived || cycle.Delivery.State == Ordered {
		v.Broken = append(v.Broken, NeedsMoreThanHeld(plan, stock)...)
	}
	if arrived {
		v.Broken = append(v.Broken, SplitsAfterPutAway(plan)...)
	}
	v.Broken = append(v.Broken, StartsInThePast(from, profile.SlotTimes, now)...)
	v.Trailing, v.Days = PlanDays(earlier, plan.Eaten(unslotted), from, last.On)
	v.Misses = append(profile.Targets.Misses(v.Days), plan.SoftMisses(free, kept)...)
	v.MadeForAfter = plan.MadeForAfter()
	return v
}

// RuleSplitAfterPutAway is a freezer split in a Plan of the current Cycle: its Delivery
// is already put away.
const RuleSplitAfterPutAway Rule = "split-after-put-away"

// SplitsAfterPutAway are the violations of `p`'s freezer splits, `p` being a Plan of the
// current Cycle.
func SplitsAfterPutAway(p Plan) []Violation {
	var vs []Violation
	for _, fs := range p.FreezerSplits {
		vs = append(vs, Violation{Rule: RuleSplitAfterPutAway, Message: fmt.Sprintf("%g %s %s is split to the freezer, and the Delivery is already put away",
			fs.Quantity.Number(), fs.Ingredient.Unit, fs.Ingredient.Name)})
	}
	return vs
}
