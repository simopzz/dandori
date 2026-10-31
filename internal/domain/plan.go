package domain

import (
	"slices"
	"time"
)

// SlotKind is which eating occasion of a day a Slot is.
type SlotKind string

const (
	Breakfast SlotKind = "breakfast"
	Lunch     SlotKind = "lunch"
	Snack     SlotKind = "snack"
	Dinner    SlotKind = "dinner"
)

// SlotKinds lists every SlotKind, in the order of a day.
var SlotKinds = []SlotKind{Breakfast, Lunch, Snack, Dinner}

// Slot is an eating occasion on a given day. A Prep Session is placed at
// one too: the occasion it is prepared for.
type Slot struct {
	On   time.Time
	Kind SlotKind
}

// FirstUnprepared is the first Slot a Plan from the Slot from on, made on
// today, may prepare a Prep Session at: from, but never today, since a
// Prep Session is taken as prepared from its day on.
func FirstUnprepared(from Slot, today time.Time) Slot {
	if tomorrow := FirstSlotAfter(today); from.Before(tomorrow) {
		return tomorrow
	}
	return from
}

// FirstSlotAfter is the first Slot of the day after day.
func FirstSlotAfter(day time.Time) Slot {
	return Slot{On: day.AddDate(0, 0, 1), Kind: SlotKinds[0]}
}

// Before reports whether s comes earlier than o.
func (s Slot) Before(o Slot) bool {
	if !s.On.Equal(o.On) {
		return s.On.Before(o.On)
	}
	return slices.Index(SlotKinds, s.Kind) < slices.Index(SlotKinds, o.Kind)
}

// Plan is the Prep Sessions of a Cycle with the Portions they yield, and
// the Portions without a Slot it puts in Slots.
type Plan struct {
	PrepSessions []PrepSession
	Kept         []KeptAssignment
}

// PrepSession is one preparation of food in a Plan: of Recipe, or, when
// Recipe is nil, of Ingredients eaten as they are.
type PrepSession struct {
	// ID identifies a stored Prep Session; it is 0 in a Plan Proposal.
	ID     int64
	At     Slot
	Recipe *Recipe
	// Ingredients are what a Prep Session without a Recipe sets out, for
	// all its Portions together.
	Ingredients []RecipeIngredient
	Portions    []PlannedPortion
}

// PlannedPortion is a Portion a Prep Session yields, with its planned
// eating.
type PlannedPortion struct {
	// ID identifies a stored Portion; it is 0 in a Plan Proposal.
	ID       int64
	Location StorageLocation
	// EatAt is when the Portion is to be eaten: its Slot when that falls in
	// the Cycle, and after the Cycle it stays without a Slot. It is nil when
	// no eating is planned for it.
	EatAt *Slot
}

// KeptAssignment puts a Portion without a Slot, such as one carried over
// from an earlier Cycle, in a Slot.
type KeptAssignment struct {
	PortionID int64
	Slot      Slot
}
