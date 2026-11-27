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

// Thaw is a move of frozen food to the fridge the evening before the Slot it is for: of
// some of an Ingredient for a Prep Session at that Slot, or of the Portion filling it.
// StockThaw and PortionThaw are the only ways to make one, so exactly one of the two
// is set.
type Thaw struct {
	slot        Slot
	prepSession int64
	ingredient  *Ingredient
	quantity    Quantity
	portion     *Portion
}

// StockThaw is the thaw of `quantity` of `ing` for the Prep Session `ps` at `s`.
func StockThaw(s Slot, ps int64, ing Ingredient, quantity Quantity) Thaw {
	return Thaw{slot: s, prepSession: ps, ingredient: &ing, quantity: quantity}
}

// PortionThaw is the thaw of the Portion `p` filling `s`.
func PortionThaw(s Slot, p Portion) Thaw {
	return Thaw{slot: s, portion: &p}
}

// For is the Slot `t` thaws for.
func (t Thaw) For() Slot { return t.slot }

// Ingredient is what `t` thaws of, with how much in its Unit: `ok` is false for a
// Portion's thaw.
func (t Thaw) Ingredient() (ing Ingredient, quantity Quantity, ok bool) {
	if t.ingredient == nil {
		return Ingredient{}, 0, false
	}
	return *t.ingredient, t.quantity, true
}

// Portion is the Portion `t` thaws: `ok` is false for a Stock Item's thaw.
func (t Thaw) Portion() (p Portion, ok bool) {
	if t.portion == nil {
		return Portion{}, false
	}
	return *t.portion, true
}

// Thaws are the StockThaws `p`'s Prep Sessions, in the order of their Slots, thaw of
// `stock`, what the kitchen holds of each Stock Item. Each uses up the `stock` outside
// the freezer, and the frozen `stock` of an Ingredient usually kept there, used from
// frozen; what it still needs is thawed from the freezer as far as the freezer holds it.
// What falls short comes from elsewhere, never frozen.
func (p Plan) Thaws(stock map[StockKey]Quantity) []Thaw {
	left := maps.Clone(stock)
	var thaws []Thaw
	for _, ps := range p.bySlot() {
		for _, ri := range ps.Ingredients() {
			if thawed := useUp(left, ri); thawed > 0 {
				thaws = append(thaws, StockThaw(ps.At, ps.ID, ri.Ingredient, thawed))
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

// RuleMoreThanHeld is a Prep Session of the current Cycle, or of one whose Delivery is
// ordered, needing more of an Ingredient than the kitchen will hold: no Delivery will
// bring the rest.
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
	for _, took := range usesFrom(stock, ri) {
		need -= took.Quantity
	}
	return take(stock, StockKey{Ingredient: ri.Ingredient.ID, Location: Freezer}, need)
}

// took is how much of a Stock Item a Prep Session took.
type took struct {
	Key      StockKey
	Quantity Quantity
}

// usesFrom takes `ri` from `stock` as a Prep Session uses it up without thawing: the
// `stock` outside the freezer, and the frozen `stock` of an Ingredient usually kept
// there, as far as it goes. It is what it took of each Stock Item, in Storage
// Location order.
func usesFrom(stock map[StockKey]Quantity, ri RecipeIngredient) []took {
	need := ri.Quantity
	var taken []took
	// shortcut: a Stock Item is one per Storage Location, so "oldest first" is the order
	// of StorageLocations; it changes nothing `thawed`.
	for _, loc := range StorageLocations {
		if loc == Freezer && !ri.Ingredient.UseFromFrozen {
			continue
		}
		key := StockKey{Ingredient: ri.Ingredient.ID, Location: loc}
		if q := take(stock, key, need); q > 0 {
			need -= q
			taken = append(taken, took{key, q})
		}
	}
	return taken
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
		if passed(t.slot) && !now.Before(arrives) {
			add("%s needs %g %s %s thawed from the freezer, and the evening before to thaw it has passed", t.slot, t.quantity.Number(), t.ingredient.Unit, t.ingredient.Name)
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

// ThawsOf are the Thaws of `thawed`, thawed Movements, in their order, their Ingredients
// found among `known` and their Portions among `portions`. A Movement of one found in
// neither is left out: there is nothing to tell of it.
func ThawsOf(thawed []Movement, known []Ingredient, portions []Portion) []Thaw {
	var thaws []Thaw
	for _, m := range thawed {
		if len(m.Portions) > 0 {
			if i := slices.IndexFunc(portions, func(p Portion) bool { return p.ID == m.Portions[0] }); i >= 0 {
				thaws = append(thaws, PortionThaw(m.Slot, portions[i]))
			}
		} else if i := slices.IndexFunc(known, func(ing Ingredient) bool { return ing.ID == m.Key.Ingredient }); i >= 0 {
			thaws = append(thaws, StockThaw(m.Slot, m.PrepSession, known[i], m.Change))
		}
	}
	return thaws
}
