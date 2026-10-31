package domain

import (
	"errors"
	"fmt"
	"maps"
	"time"
)

// TimeOfDay is a time of day on the user's wall clock, in minutes after
// midnight.
type TimeOfDay int

// ParseTimeOfDay reads a time of day written as HH:MM.
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err
	}
	return TimeOfDay(t.Hour()*60 + t.Minute()), nil
}

func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t/60, t%60) }

// SlotTimes is the time of day of each SlotKind.
type SlotTimes map[SlotKind]TimeOfDay

// DefaultSlotTimes are the Slot times of a Profile the user hasn't changed.
var DefaultSlotTimes = SlotTimes{Breakfast: 8 * 60, Lunch: 13 * 60, Snack: 17 * 60, Dinner: 20 * 60}

// Delivery is one supermarket delivery: its date, at midnight UTC, and its
// time window.
type Delivery struct {
	On       time.Time
	From, To TimeOfDay
	// Ordered is whether the user checked out on the supermarket's site;
	// until then the Delivery is tentative.
	Ordered bool
}

// Arrival is when the Delivery is taken to have arrived: the end of its
// window, on the wall clock as WallClock gives it.
func (d Delivery) Arrival() time.Time { return d.On.Add(time.Duration(d.To) * time.Minute) }

// Cycle is the stretch one Delivery has to last. Its dates are days, at
// midnight UTC.
type Cycle struct {
	Delivery Delivery
	// Start is the Cycle's first Slot: the first at or after the end of
	// its Delivery window.
	Start Slot
	// EndsOn is the planned end date, the Cycle's last day.
	EndsOn time.Time
	// ClosedAt is the next Cycle's first Slot when its Delivery arrived
	// before EndsOn, closing this one early; nil otherwise.
	ClosedAt *Slot
}

// NewCycle is the Cycle delivered by d and planned to end on endsOn, its
// Slots at times.
func NewCycle(d Delivery, endsOn time.Time, times SlotTimes) Cycle {
	start := Slot{On: d.On.AddDate(0, 0, 1), Kind: SlotKinds[0]}
	for _, k := range SlotKinds {
		if times[k] >= d.To {
			start = Slot{On: d.On, Kind: k}
			break
		}
	}
	return Cycle{Delivery: d, Start: start, EndsOn: endsOn}
}

// DayOf is the day t falls on, in t's own time zone, as a date of the
// Cycles: midnight UTC.
func DayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// WallClock is t as it reads on a wall clock in t's own time zone, as a
// moment of the Cycles: in UTC, so it compares with a Delivery's Arrival.
func WallClock(t time.Time) time.Time {
	return DayOf(t).Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute)
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
	if !c.EndsOn.After(c.Delivery.On) {
		problems["EndsOn"] = "Must be after the delivery date"
	}
	if c.Delivery.To <= c.Delivery.From {
		problems["Delivery.To"] = "Must be after the start of the window"
	}
	return problems
}

// Until is the first Slot after the Cycle: the next Cycle's first when it
// closed this one early, or breakfast the day after its end date.
func (c Cycle) Until() Slot {
	if c.ClosedAt != nil {
		return *c.ClosedAt
	}
	return Slot{On: c.EndsOn.AddDate(0, 0, 1), Kind: SlotKinds[0]}
}

// Contains reports whether s is one of the Cycle's Slots.
func (c Cycle) Contains(s Slot) bool { return !s.Before(c.Start) && s.Before(c.Until()) }

// LastDay is the day of the Cycle's last Slot.
func (c Cycle) LastDay() time.Time {
	if u := c.Until(); u.Kind != SlotKinds[0] {
		return u.On
	}
	return c.Until().On.AddDate(0, 0, -1)
}

// Arrived reports whether the Cycle's Delivery has arrived at now: it is
// upcoming until then, and current after.
func (c Cycle) Arrived(now time.Time) bool { return !WallClock(now).Before(c.Delivery.Arrival()) }

// planNextDays is how many days before a Cycle's end date Dandori offers
// to plan the next one.
const planNextDays = 3

// PlanNextDue reports whether, on today, it is time to plan the Cycle
// after this one.
func (c Cycle) PlanNextDue(today time.Time) bool {
	return !today.Before(c.EndsOn.AddDate(0, 0, -planNextDays))
}

// ErrCycleOverlaps means a new Cycle would be delivered on or before the
// current Cycle's delivery date.
var ErrCycleOverlaps = errors.New("cycle overlaps the current one")

// ErrCycleUpcoming means a Cycle is already upcoming: at most one is.
var ErrCycleUpcoming = errors.New("a cycle is already upcoming")

// ErrDeliveryOrdered means the Delivery was ordered, so its Cycle can no
// longer be dropped.
var ErrDeliveryOrdered = errors.New("delivery already ordered")

// OpenCycle checks next can be opened after latest, the latest Cycle, or
// after none when latest is nil, at now.
func OpenCycle(latest *Cycle, now time.Time, next Cycle) error {
	switch {
	case latest == nil:
		return nil
	case !latest.Arrived(now):
		return ErrCycleUpcoming
	case !next.Delivery.On.After(latest.Delivery.On):
		return ErrCycleOverlaps
	}
	return nil
}

// Drop checks the upcoming Cycle c can be dropped: only while its Delivery
// is tentative.
func (c Cycle) Drop() error {
	if c.Delivery.Ordered {
		return ErrDeliveryOrdered
	}
	return nil
}

// MovedTo is the Cycle delivered by d instead, its first Slot at times. A
// Delivery moved later moves the end date as many days later, since it is
// to last as long; one moved earlier leaves it, for the next Plan Proposal
// to judge from what is left.
func (c Cycle) MovedTo(d Delivery, times SlotTimes) Cycle {
	ends := c.EndsOn
	if d.On.After(c.Delivery.On) {
		ends = ends.Add(d.On.Sub(c.Delivery.On))
	}
	return NewCycle(d, ends, times)
}

// MoveDelivery checks the upcoming Cycle, delivered after current (nil
// when there is none), can become moved, a Cycle delivered at another
// time. It reports whether current's Plan, which reached the upcoming
// Cycle, no longer reaches the moved one and needs a new Plan Proposal.
func MoveDelivery(current *Cycle, upcoming, moved Cycle) (currentNeedsProposal bool, err error) {
	if current == nil {
		return false, nil
	}
	if !moved.Delivery.On.After(current.Delivery.On) {
		return false, ErrCycleOverlaps
	}
	reaches := func(s Slot) bool { return !current.Until().Before(s) }
	return reaches(upcoming.Start) && !reaches(moved.Start), nil
}

// Arrival is what the arrival of the next Cycle's Delivery does to the
// current one and its Plan.
type Arrival struct {
	// Current is the current Cycle as the arrival leaves it.
	Current Cycle
	// ClosesEarly is whether the Delivery arrived before the current Cycle
	// ends, closing it early.
	ClosesEarly bool
	// DroppedPrepSessions are the current Plan's Prep Sessions from the
	// next Cycle's first Slot on: they are never prepared.
	DroppedPrepSessions []int64
	// CarriedOver are the Portions the current Plan has eaten from the next
	// Cycle's first Slot on but prepared before it: they carry over and
	// lose their Slot.
	CarriedOver []int64
}

// Arrive is the arrival of next's Delivery after current, the current
// Cycle with its Plan, or after none when current is nil.
func Arrive(current *Cycle, plan Plan, next Cycle) Arrival {
	var a Arrival
	if current == nil {
		return a
	}
	a.Current = *current
	from := next.Start
	if !from.Before(current.Until()) {
		return a
	}
	a.ClosesEarly = true
	a.Current.ClosedAt = &from
	for _, ps := range plan.PrepSessions {
		if !ps.At.Before(from) {
			a.DroppedPrepSessions = append(a.DroppedPrepSessions, ps.ID)
			continue
		}
		for _, p := range ps.Portions {
			if p.EatAt != nil && !p.EatAt.Before(from) {
				a.CarriedOver = append(a.CarriedOver, p.ID)
			}
		}
	}
	for _, k := range plan.Kept {
		if !k.Slot.Before(from) {
			a.CarriedOver = append(a.CarriedOver, k.PortionID)
		}
	}
	return a
}

// CarryOverChanged reports whether next's arrival after current would
// carry over other Portions with current's Plan changed from before to
// after: how many of each Recipe, whatever their ids, since an approval
// stores again the Prep Sessions it keeps. kept names the Portions without
// a Slot either Plan puts in Slots.
func CarryOverChanged(current Cycle, before, after Plan, kept []KeptPortion, next Cycle) bool {
	return !maps.Equal(carriesOver(current, before, kept, next), carriesOver(current, after, kept, next))
}

// carriesOver is how many Portions of each Recipe, by name, next's
// arrival after current with plan carries over.
func carriesOver(current Cycle, plan Plan, kept []KeptPortion, next Cycle) map[string]int {
	names := map[int64]string{}
	for _, k := range kept {
		names[k.ID] = k.Name
	}
	for _, ps := range plan.PrepSessions {
		for _, p := range ps.Portions {
			names[p.ID] = ps.Name()
		}
	}
	counts := map[string]int{}
	for _, id := range Arrive(&current, plan, next).CarriedOver {
		counts[names[id]]++
	}
	return counts
}
