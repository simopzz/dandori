package domain

import (
	"time"
)

// SlotKind is the kind of eating occasion a Slot is.
type SlotKind string

const (
	Breakfast SlotKind = "breakfast"
	Lunch     SlotKind = "lunch"
	Snack     SlotKind = "snack"
	Dinner    SlotKind = "dinner"
)

// SlotKinds lists every SlotKind in the order of a day.
var SlotKinds = []SlotKind{Breakfast, Lunch, Snack, Dinner}

// SlotTimes is the Profile's time of day of each SlotKind, the same every day.
type SlotTimes map[SlotKind]TimeOfDay

// DefaultSlotTimes are the times of day of a Profile the User hasn't changed.
var DefaultSlotTimes = SlotTimes{Breakfast: 8 * 60, Lunch: 13 * 60, Snack: 17 * 60, Dinner: 20 * 60}

// Slot is one eating occasion: a SlotKind on a day, at midnight in the clock's zone.
type Slot struct {
	On   time.Time
	Kind SlotKind
}

// At is when s is, by times, on the wall clock of its day.
func (s Slot) At(times SlotTimes) time.Time {
	y, m, d := s.On.Date()
	t := times[s.Kind]
	return time.Date(y, m, d, int(t/60), int(t%60), 0, 0, s.On.Location())
}

// NowFor is how long a Slot is now after its time of day, its last minute included.
const NowFor = time.Hour

// Today is the day's Slots as Today shows them at a moment.
type Today struct {
	// First is the Slot now, or the next one when none is.
	First Slot
	Now   bool
	// Later are the day's Slots after First.
	Later []Slot
}

// TodayAt is the day's Slots at at, by times. A Slot is now from its time of day up to
// and including NowFor after, to the minute; the latest such is First. Otherwise First is
// the next Slot, tomorrow's breakfast once dinner's hour is past, and a dinner whose hour
// runs past midnight is still now after it.
func TodayAt(at time.Time, times SlotTimes) Today {
	y, m, d := at.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, at.Location())
	var slots []Slot
	for _, day := range []time.Time{today.AddDate(0, 0, -1), today, today.AddDate(0, 0, 1)} {
		for _, k := range SlotKinds {
			slots = append(slots, Slot{On: day, Kind: k})
		}
	}
	minute := at.Truncate(time.Minute)
	first, now := -1, false
	for i, s := range slots {
		if start := s.At(times); !at.Before(start) && !minute.After(start.Add(NowFor)) {
			first, now = i, true
		}
	}
	if !now {
		for i, s := range slots {
			if s.At(times).After(at) {
				first = i
				break
			}
		}
	}
	t := Today{First: slots[first], Now: now}
	for _, s := range slots[first+1:] {
		if s.On.Equal(today) {
			t.Later = append(t.Later, s)
		}
	}
	return t
}
