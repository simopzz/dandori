package domain

import (
	"slices"
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

// Before is the kind before `k` in the day; breakfast has none, "".
func (k SlotKind) Before() SlotKind {
	if i := slices.Index(SlotKinds, k); i > 0 {
		return SlotKinds[i-1]
	}
	return ""
}

// SlotTimes is the Profile's time of day of each SlotKind, the same every day.
type SlotTimes map[SlotKind]TimeOfDay

// On is `times` on any day.
func (times SlotTimes) On(time.Time) SlotTimes { return times }

// Times is the time of day of each SlotKind on a day, `day` at midnight in the clock's
// zone: SlotTimes, the same every day, or a TimesChange.
type Times interface {
	On(day time.Time) SlotTimes
}

// TimesChange is the Profile's times of day across a change of them: Was before the day
// From, the one after the day of the change, and Now from it on. A change takes effect
// from the next day so that a Slot that has passed stays passed. From is zero when
// there's no change to wait for.
type TimesChange struct {
	Was, Now SlotTimes
	From     time.Time
}

// On is the times of day in force on `day`, From and `day` each read as a date in its
// own zone.
func (c TimesChange) On(day time.Time) SlotTimes {
	date := func(t time.Time) time.Time { y, m, d := t.Date(); return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	if date(day).Before(date(c.From)) {
		return c.Was
	}
	return c.Now
}

// DefaultSlotTimes are the times of day of a Profile the User hasn't changed.
var DefaultSlotTimes = SlotTimes{Breakfast: 8 * 60, Lunch: 13 * 60, Snack: 17 * 60, Dinner: 20 * 60}

// NotAfterTheOneBefore is a kind's time of day not after the one before it in the day.
const NotAfterTheOneBefore Problem = "not after the one before"

// ParseSlotTimes reads SlotTimes as `typed`, by SlotKind, with a problem per field
// refused: a kind's time not after the one before's is refused on the later kind, and a
// field that isn't a time skips the order check against it.
func ParseSlotTimes(typed map[string]string) (SlotTimes, map[string]Problem) {
	times, problems := SlotTimes{}, map[string]Problem{}
	for _, k := range SlotKinds {
		t, problem := ParseTimeOfDay(typed[string(k)])
		if problem != "" {
			problems[string(k)] = problem
			continue
		}
		times[k] = t
		if before, ok := times[k.Before()]; ok && t <= before {
			problems[string(k)] = NotAfterTheOneBefore
		}
	}
	return times, problems
}

// Slot is one eating occasion: a SlotKind on a day, at midnight in the clock's zone.
type Slot struct {
	On   time.Time
	Kind SlotKind
}

// At is when `s` is, by `times`, on the wall clock of its day.
func (s Slot) At(times Times) time.Time {
	y, m, d := s.On.Date()
	t := times.On(s.On)[s.Kind]
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

// TodayAt is the day's Slots at `at`, by `times`. A Slot is now from its time of day up
// to and including NowFor after, to the minute; the latest such is First. Otherwise First
// is the next Slot, tomorrow's breakfast once dinner's hour is past, and a dinner whose
// hour runs past midnight is still now after it.
func TodayAt(at time.Time, times Times) Today {
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

// PassBetween reports whether the time of day of some Slot, by `times`, falls after
// `from` and by `to`, its days those of `to`'s zone, the clock's.
func PassBetween(times Times, from, to time.Time) bool {
	y, m, d := from.In(to.Location()).Date()
	for day := time.Date(y, m, d, 0, 0, 0, 0, to.Location()); !day.After(to); day = day.AddDate(0, 0, 1) {
		for _, k := range SlotKinds {
			if t := (Slot{On: day, Kind: k}).At(times); t.After(from) && !t.After(to) {
				return true
			}
		}
	}
	return false
}
