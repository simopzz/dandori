package domain

import (
	"fmt"
	"maps"
	"slices"
	"time"
)

// RuleNoEveningToThaw is frozen food needed at a Slot or Prep Session whose evening
// before, after the previous day's dinner, has passed.
const RuleNoEveningToThaw Rule = "no-evening-to-thaw"

// Thaw is what a Prep Session at At needs of Ingredient thawed from the freezer the
// evening before.
type Thaw struct {
	At         Slot
	Ingredient Ingredient
	Quantity   Quantity
}

// Thaws are what `p`'s Prep Sessions, in the order of their Slots, thaw of `stock`, what
// the kitchen holds of each Stock Item. Each uses up the `stock` outside the freezer, and
// the frozen `stock` of an Ingredient usually kept there, used from frozen; what it still
// needs is thawed from the freezer as far as the freezer holds it. What falls short comes
// from elsewhere, never frozen.
func (p Plan) Thaws(stock map[StockKey]Quantity) []Thaw {
	left := maps.Clone(stock)
	var thaws []Thaw
	for _, ps := range p.bySlot() {
		for _, ri := range ps.Ingredients() {
			if thawed := useUp(left, ri); thawed > 0 {
				thaws = append(thaws, Thaw{At: ps.At, Ingredient: ri.Ingredient, Quantity: thawed})
			}
		}
	}
	return thaws
}

// bySlot are `p`'s Prep Sessions in the order of their Slots.
func (p Plan) bySlot() []PrepSession {
	sessions := slices.Clone(p.PrepSessions)
	slices.SortStableFunc(sessions, func(a, b PrepSession) int {
		switch {
		case a.At.Before(b.At):
			return -1
		case b.At.Before(a.At):
			return 1
		}
		return 0
	})
	return sessions
}

// RuleMoreThanHeld is a Prep Session of the current Cycle needing more of an Ingredient
// than the kitchen will hold: no Delivery will bring the rest.
const RuleMoreThanHeld Rule = "more-than-held"

// NeedsMoreThanHeld are the violations of `p`'s Prep Sessions, in the order of their
// Slots, each needing more of an Ingredient than `stock`, what the kitchen holds of each
// Stock Item, has left of it once those before have used theirs up. A Staple never does:
// the kitchen always holds it.
func NeedsMoreThanHeld(p Plan, stock map[StockKey]Quantity) []Violation {
	left := maps.Clone(stock)
	var vs []Violation
	for _, ps := range p.bySlot() {
		for _, ri := range ps.Ingredients() {
			var held Quantity
			for _, loc := range StorageLocations {
				held += max(0, left[StockKey{Ingredient: ri.Ingredient.ID, Location: loc}])
			}
			if !ri.Ingredient.Staple && held < ri.Quantity {
				unit := ri.Ingredient.Unit
				vs = append(vs, Violation{Rule: RuleMoreThanHeld, Message: fmt.Sprintf("%s at %s needs %g %s %s, and the kitchen will hold only %g %s",
					ps.Name(), ps.At, ri.Quantity.Number(), unit, ri.Ingredient.Name, held.Number(), unit)})
			}
			useUp(left, ri)
		}
	}
	return vs
}

// useUp takes `ri` from `stock` as a Prep Session uses it up: the `stock` outside the
// freezer, and the frozen `stock` of an Ingredient usually kept there, then what it still
// needs from the freezer, and is how much of that it thaws.
func useUp(stock map[StockKey]Quantity, ri RecipeIngredient) (thawed Quantity) {
	need := ri.Quantity
	// shortcut: a Stock Item is one per Storage Location, so "oldest first" is the order
	// of StorageLocations; it changes nothing `thawed`.
	for _, loc := range StorageLocations {
		if loc == Freezer && ri.Ingredient.Usual != Freezer {
			continue
		}
		need -= take(stock, StockKey{Ingredient: ri.Ingredient.ID, Location: loc}, need)
	}
	return take(stock, StockKey{Ingredient: ri.Ingredient.ID, Location: Freezer}, need)
}

// take takes up to `need` from what `stock` holds at `key`, and is how much it took.
func take(stock map[StockKey]Quantity, key StockKey, need Quantity) Quantity {
	took := min(need, stock[key])
	if took <= 0 {
		return 0
	}
	stock[key] -= took
	return took
}

// NoEveningToThaw are the violations of `p`'s frozen food with no evening left to thaw it
// at `now`, the evening before a Slot passing at midnight of its day: a Portion without a
// Slot in the freezer, as `frozen` says by id, put in a Slot; what `p`'s Prep Sessions
// thaw of `stock`, unless the Delivery arriving at `arrives` is still to come and brings
// it fresh; and a Portion `p` puts in the freezer eaten the day it is prepared.
func NoEveningToThaw(p Plan, frozen map[int64]bool, stock map[StockKey]Quantity, now, arrives time.Time) []Violation {
	var vs []Violation
	add := func(format string, args ...any) {
		vs = append(vs, Violation{Rule: RuleNoEveningToThaw, Message: fmt.Sprintf(format, args...)})
	}
	passed := func(s Slot) bool { return !now.Before(s.On) }
	for _, k := range p.Kept {
		if frozen[k.Portion] && passed(k.At) {
			add("Portion %d is frozen, and the evening before %s to thaw it has passed", k.Portion, k.At)
		}
	}
	for _, t := range p.Thaws(stock) {
		if passed(t.At) && !now.Before(arrives) {
			add("%s needs %g %s %s thawed from the freezer, and the evening before to thaw it has passed", t.At, t.Quantity.Number(), t.Ingredient.Unit, t.Ingredient.Name)
		}
	}
	for _, ps := range p.PrepSessions {
		for _, pp := range ps.Portions {
			if pp.Location == Freezer && pp.EatAt != nil && !pp.EatAt.Before(ps.At) && !pp.EatAt.On.After(ps.At.On) {
				add("%s is frozen at %s and eaten at %s, with no evening between to thaw it", ps.Name(), ps.At, *pp.EatAt)
			}
		}
	}
	return vs
}
