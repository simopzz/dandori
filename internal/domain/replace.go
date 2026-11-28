package domain

import "slices"

// Replace is what a Plan Proposal of the Slots from `first` to `last` replaces and keeps
// of `p`, `lots` being the Portion Lots the kitchen holds without a Slot. It keeps `p`'s
// Prep Sessions outside those Slots, their Portions eaten in them left without a Slot,
// and the Portions without a Slot `p` puts in Slots outside them. Free are the Portions
// the Plan Proposal may put in Slots: `lots`, less those kept in a Slot, then the
// Portions the Prep Sessions kept before `first` leave without a Slot, prepared but not
// yet in the Kitchen Ledger, a Portion Lot for each such Prep Session's Storage Location,
// by PlannedID, unless `lots` lists them already. `p` is left as it was.
func (p Plan) Replace(first, last Slot, lots []PortionLot) (kept Plan, free []PortionLot) {
	in := func(s Slot) bool { return !s.Before(first) && !last.Before(s) }
	for _, ps := range p.PrepSessions {
		if in(ps.At) {
			continue
		}
		ps.Portions = slices.Clone(ps.Portions)
		for i, pp := range ps.Portions {
			if pp.EatAt != nil && in(*pp.EatAt) {
				ps.Portions[i].EatAt = nil
			}
		}
		kept.PrepSessions = append(kept.PrepSessions, ps)
	}
	for _, k := range p.Kept {
		if !in(k.At) {
			kept.Kept = append(kept.Kept, k)
		}
	}
	for _, lot := range lots {
		lot.IDs = slices.DeleteFunc(slices.Clone(lot.IDs), func(id int64) bool {
			return slices.ContainsFunc(kept.Kept, func(k Kept) bool { return k.Portion == id })
		})
		if len(lot.IDs) > 0 {
			free = append(free, lot)
		}
	}
	for _, ps := range kept.PrepSessions {
		if !ps.At.Before(first) {
			continue
		}
		ps.Portions = slices.DeleteFunc(slices.Clone(ps.Portions), func(pp PlannedPortion) bool {
			return pp.EatAt != nil || slices.ContainsFunc(lots, func(l PortionLot) bool { return slices.Contains(l.IDs, PlannedID(pp.ID)) })
		})
		free = append(free, ps.Unslotted()...)
	}
	return kept, free
}

// With is `p` once a Plan Proposal's Plan `q` from the Slot `first` is approved: what
// Replace keeps of `p` over the Slots `q` fills, a kept Prep Session's Portion `q` puts
// in a Slot eaten there, then `q`'s Prep Sessions, the Portions without a Slot it puts in
// Slots, and its freezer splits. `p` is left as it was.
func (p Plan) With(first Slot, q Plan) Plan {
	out, _ := p.Replace(first, q.last(first), nil)
	for _, k := range q.Kept {
		if !out.place(k) {
			out.Kept = append(out.Kept, k)
		}
	}
	out.PrepSessions = append(out.PrepSessions, q.PrepSessions...)
	out.FreezerSplits = q.FreezerSplits
	return out
}

// place eats the Planned Portion `k` names at `k`'s Slot, and reports whether `p` has it.
func (p Plan) place(k Kept) bool {
	for _, ps := range p.PrepSessions {
		for i, pp := range ps.Portions {
			if pp.ID != 0 && PlannedID(pp.ID) == k.Portion {
				at := k.At
				ps.Portions[i].EatAt = &at
				return true
			}
		}
	}
	return false
}

// last is the last Slot `p` fills, `from` when it fills none later.
func (p Plan) last(from Slot) Slot {
	last := from
	for _, ps := range p.PlannedSlots() {
		if last.Before(ps.At) {
			last = ps.At
		}
	}
	return last
}

// Unslotted is the Portions `ps` prepares without a Slot, a Portion Lot for each Storage
// Location, by PlannedID.
func (ps PrepSession) Unslotted() []PortionLot {
	var byLocation []PortionLot
	for _, pp := range ps.Portions {
		if pp.EatAt != nil {
			continue
		}
		i := slices.IndexFunc(byLocation, func(l PortionLot) bool { return l.Location == pp.Location })
		if i < 0 {
			lot := PortionLot{Name: ps.Name(), Nutrition: ps.NutritionPerPortion().Nutrition, Location: pp.Location}
			byLocation, i = append(byLocation, lot), len(byLocation)
		}
		byLocation[i].IDs = append(byLocation[i].IDs, PlannedID(pp.ID))
	}
	return byLocation
}
