package domain

import "time"

// The lengths of a Cycle in days after its Delivery's date: its end date unless the User
// types one, and the longest a planning Run plans in one go.
const (
	DefaultCycleDays = 10
	MaxCycleDays     = 14
)

// The fields of opening a Cycle, as typed.
const (
	CycleDelivery = "delivery"
	CycleFrom     = "from"
	CycleTo       = "to"
	CycleEnds     = "ends"
)

// Why opening a Cycle is refused on a field.
const (
	// NotAfterFrom is a Delivery window ending before it starts, or as it starts.
	NotAfterFrom Problem = "not after from"
	// AlreadyEnded is a Delivery window that has already ended, on its date when that was
	// before today.
	AlreadyEnded Problem = "already ended"
	// NotAfterDelivery is an end date on or before the Delivery's date.
	NotAfterDelivery Problem = "not after the delivery"
	// TooLong is an end date more than MaxCycleDays after the Delivery's date.
	TooLong Problem = "too long"
)

// Delivery is one supermarket delivery: its day, at midnight in the clock's zone, and its
// time window on that day.
type Delivery struct {
	On       time.Time
	From, To TimeOfDay
}

// Arrives is when d is taken to have arrived: the end of its window.
func (d Delivery) Arrives() time.Time {
	y, m, day := d.On.Date()
	return time.Date(y, m, day, int(d.To/60), int(d.To%60), 0, 0, d.On.Location())
}

// Cycle is the stretch one Delivery has to last, to its end date: a day at midnight in
// the clock's zone.
type Cycle struct {
	Delivery Delivery
	Ends     time.Time
}

// CycleStatus is where a Cycle is in its life.
type CycleStatus string

const (
	Upcoming CycleStatus = "upcoming"
	Current  CycleStatus = "current"
	Closed   CycleStatus = "closed"
)

// ParseCycle reads a Cycle opened at now as typed, by field, in l's order of day and
// month, with a problem per field refused. A blank end date is DefaultCycleDays after the
// Delivery's date.
func ParseCycle(l Language, typed map[string]string, now time.Time) (Cycle, map[string]Problem) {
	problems := map[string]Problem{}
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	date := func(field string) time.Time {
		t, problem := ParseDate(l, typed[field])
		if problem != "" {
			problems[field] = problem
			return time.Time{}
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
	}
	clock := func(field string) TimeOfDay {
		t, problem := ParseTimeOfDay(typed[field])
		if problem != "" {
			problems[field] = problem
		}
		return t
	}
	var c Cycle
	c.Delivery.On, c.Delivery.From, c.Delivery.To = date(CycleDelivery), clock(CycleFrom), clock(CycleTo)
	dated := problems[CycleDelivery] == ""
	if dated && c.Delivery.On.Before(today) {
		problems[CycleDelivery] = AlreadyEnded
	}
	switch {
	case problems[CycleTo] != "":
	case problems[CycleFrom] == "" && c.Delivery.To <= c.Delivery.From:
		problems[CycleTo] = NotAfterFrom
	case problems[CycleDelivery] == "" && !c.Delivery.Arrives().After(now):
		problems[CycleTo] = AlreadyEnded
	}
	if typed[CycleEnds] == "" {
		if dated {
			c.Ends = c.Delivery.On.AddDate(0, 0, DefaultCycleDays)
		}
	} else if c.Ends = date(CycleEnds); dated && problems[CycleEnds] == "" {
		switch {
		case !c.Ends.After(c.Delivery.On):
			problems[CycleEnds] = NotAfterDelivery
		case c.Ends.After(c.Delivery.On.AddDate(0, 0, MaxCycleDays)):
			problems[CycleEnds] = TooLong
		}
	}
	if len(problems) > 0 {
		return Cycle{}, problems
	}
	return c, nil
}

// CycleStatuses are the status at now of each of cycles, in the order of their
// Deliveries: upcoming until its Delivery arrives, current until the next one's does,
// then closed.
func CycleStatuses(cycles []Cycle, now time.Time) []CycleStatus {
	statuses := make([]CycleStatus, len(cycles))
	for i, c := range cycles {
		switch {
		case c.Delivery.Arrives().After(now):
			statuses[i] = Upcoming
		case i+1 < len(cycles) && !cycles[i+1].Delivery.Arrives().After(now):
			statuses[i] = Closed
		default:
			statuses[i] = Current
		}
	}
	return statuses
}

// Slots are c's Slots by times, from the first after its Delivery arrives to the last of
// its end date, every kind every day.
func (c Cycle) Slots(times SlotTimes) []Slot {
	var slots []Slot
	for on := c.Delivery.On; !on.After(c.Ends); on = on.AddDate(0, 0, 1) {
		for _, k := range SlotKinds {
			if s := (Slot{On: on, Kind: k}); s.At(times).After(c.Delivery.Arrives()) {
				slots = append(slots, s)
			}
		}
	}
	return slots
}

// CycleOf is the index of the one of cycles, in the order of their Deliveries, that s
// belongs to by times: the one whose Delivery arrived before s, unless the next one's
// did too, and whose end date s isn't past. ok is false when s belongs to none.
func CycleOf(cycles []Cycle, s Slot, times SlotTimes) (i int, ok bool) {
	at := s.At(times)
	for i, c := range cycles {
		next := i+1 < len(cycles) && at.After(cycles[i+1].Delivery.Arrives())
		if at.After(c.Delivery.Arrives()) && !next && !s.On.After(c.Ends) {
			return i, true
		}
	}
	return 0, false
}
