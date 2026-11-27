package domain

import (
	"cmp"
	"slices"
	"time"
)

// AssumedDay is what Dandori assumed on one day, a row per Slot in the day's order, the
// evening's thaws after its dinner.
type AssumedDay struct {
	On   time.Time
	Rows []AssumedRow
}

// AssumedRow is the Movements assumed at one Slot, or, when Thaws, the thaws of the
// evening of Slot, its dinner, for the next day's Slots.
type AssumedRow struct {
	Slot      Slot
	Thaws     bool
	Movements []Movement
}

// AssumedByDay is the assumed Movements of `ms`, in the order they happened, grouped by
// day, oldest first. A Movement with no Slot, a Delivery's or one reported, isn't listed.
func AssumedByDay(ms []Movement) []AssumedDay {
	var rows []AssumedRow
	for _, m := range ms {
		if m.Slot.On.IsZero() {
			continue
		}
		key := AssumedRow{Slot: m.Slot, Thaws: m.Cause == Thawed}
		if key.Thaws {
			key.Slot = ThawEvening(m.Slot.On)
		}
		i := slices.IndexFunc(rows, func(r AssumedRow) bool { return r.Slot.Equal(key.Slot) && r.Thaws == key.Thaws })
		if i < 0 {
			rows, i = append(rows, key), len(rows)
		}
		rows[i].Movements = append(rows[i].Movements, m)
	}
	slices.SortStableFunc(rows, func(a, b AssumedRow) int {
		return cmp.Or(a.Slot.On.Compare(b.Slot.On),
			cmp.Compare(slices.Index(SlotKinds, a.Slot.Kind), slices.Index(SlotKinds, b.Slot.Kind)),
			cmp.Compare(boolInt(a.Thaws), boolInt(b.Thaws)))
	})
	var days []AssumedDay
	for _, r := range rows {
		if len(days) == 0 || !days[len(days)-1].On.Equal(r.Slot.On) {
			days = append(days, AssumedDay{On: r.Slot.On})
		}
		days[len(days)-1].Rows = append(days[len(days)-1].Rows, r)
	}
	return days
}

// boolInt is 1 for true, so false sorts first.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// AssumedPart is one part of an AssumedRow's summary: a Prep Session's Portions, named as
// Portion is, Prepared of them and Eaten of those at the Slot, or, with nothing Prepared,
// a Portion from earlier the Slot ate.
type AssumedPart struct {
	Portion         int64
	Prepared, Eaten int
}

// Summary is `r` summed up: each Prep Session it prepared, in the order it did, then each
// Portion it ate from earlier.
func (r AssumedRow) Summary() []AssumedPart {
	var parts []AssumedPart
	sessions := map[int64]int{}   // part by Prep Session
	preparedBy := map[int64]int{} // part by Portion prepared
	for _, m := range r.Movements {
		if m.Cause != Prepared {
			continue
		}
		i, ok := sessions[m.PrepSession]
		if !ok {
			i, sessions[m.PrepSession] = len(parts), len(parts)
			parts = append(parts, AssumedPart{Portion: m.Portions[0]})
		}
		parts[i].Prepared += len(m.Portions)
		for _, id := range m.Portions {
			preparedBy[id] = i
		}
	}
	for _, m := range r.Movements {
		if m.Cause != PortionEaten {
			continue
		}
		if i, ok := preparedBy[m.Portions[0]]; ok {
			parts[i].Eaten++
		} else {
			parts = append(parts, AssumedPart{Portion: m.Portions[0], Eaten: 1})
		}
	}
	return parts
}
