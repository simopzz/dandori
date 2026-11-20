package domain

import "time"

// Verdict is what a draft Plan breaks and misses against the rules of the model: the
// hard rules it Broke, the seven days before its first Slot and the days it plans, the
// Targets and soft rules it Misses, and the Portions it makes for after the Cycle.
type Verdict struct {
	Broken         []Violation
	Trailing, Days []Day
	Misses         []Miss
	MadeForAfter   []MadeForAfter
}

// Judge is the Verdict on plan, a draft of cycle's Slots by profile's Slot times from the
// Slot from on, at now: lots are the Portion Lots the kitchen holds without a Slot, stock
// what it holds of each Stock Item, and earlier what approved Plans put in Slots before.
func Judge(plan Plan, cycle Cycle, from Slot, profile Profile, lots []PortionLot, stock map[StockKey]Quantity, earlier []Eaten, now time.Time) Verdict {
	var slots []Slot
	for _, s := range cycle.Slots(profile.SlotTimes) {
		if !s.Before(from) {
			slots = append(slots, s)
		}
	}
	last := from.On
	if len(slots) > 0 {
		last = slots[len(slots)-1].On
	}
	var unslotted []Portion
	frozen := map[int64]bool{}
	for _, lot := range lots {
		for _, id := range lot.IDs {
			unslotted = append(unslotted, Portion{ID: id, Name: lot.Name, Nutrition: lot.Nutrition})
			frozen[id] = lot.Location == Freezer
		}
	}
	var v Verdict
	v.Broken = append(HardRules(plan, slots, unslotted), RecipesUnfit(plan, profile.Appliances)...)
	v.Broken = append(v.Broken, NoEveningToThaw(plan, frozen, stock, now, cycle.Delivery.Arrives())...)
	v.Broken = append(v.Broken, StartsInThePast(from, profile.SlotTimes, now)...)
	v.Trailing, v.Days = PlanDays(earlier, plan.Eaten(unslotted), from, last)
	v.Misses = append(profile.Targets.Misses(v.Days), plan.SoftMisses(lots)...)
	v.MadeForAfter = plan.MadeForAfter()
	return v
}
