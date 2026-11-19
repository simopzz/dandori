package domain

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// The range of a Prep Session's Portion size, against its Recipe's as written.
const (
	MinSize = 0.75
	MaxSize = 1.5
)

// Plan is Prep Sessions with the Portions they yield, the Portions without a Slot it puts
// in Slots, and its freezer splits: a Cycle's from a first Slot on.
type Plan struct {
	PrepSessions []PrepSession
	Kept         []Kept
	// FreezerSplits are how much of each Ingredient goes in the freezer when the
	// Delivery is put away.
	FreezerSplits []RecipeIngredient
}

// PrepSession is one preparation of food at a Slot: of Recipe, its Portions all of Size,
// or, with no Recipe, of Ingredient set out as it is, Quantity of it for each Portion.
type PrepSession struct {
	At         Slot
	Recipe     *Recipe
	Size       float64
	Ingredient *Ingredient
	Quantity   Quantity
	Portions   []PlannedPortion
}

// PlannedPortion is a Portion a Prep Session yields: where it is kept, and the Slot it
// is eaten at, nil when it has none and carries over.
type PlannedPortion struct {
	Location StorageLocation
	EatAt    *Slot
}

// Kept puts the Portion without a Slot whose id is Portion in the Slot At.
type Kept struct {
	Portion int64
	At      Slot
}

// Name is what ps prepares: its Recipe, or its Ingredient.
func (ps PrepSession) Name() string {
	if ps.Recipe != nil {
		return ps.Recipe.Name
	}
	return ps.Ingredient.Name
}

// Ingredients are what ps uses for all its Portions: its Recipe's Ingredients scaled by
// the Portions it yields over the Portions as written, times its size, or its
// Ingredient's quantity for each Portion. What is counted in pieces is rounded to whole
// ones, never to none.
func (ps PrepSession) Ingredients() []RecipeIngredient {
	n := float64(len(ps.Portions))
	if ps.Recipe == nil {
		return []RecipeIngredient{scaled(*ps.Ingredient, ps.Quantity.Number()*n)}
	}
	out := make([]RecipeIngredient, len(ps.Recipe.Ingredients))
	for i, ri := range ps.Recipe.Ingredients {
		out[i] = scaled(ri.Ingredient, ri.Quantity.Number()*n/float64(ps.Recipe.Portions)*ps.Size)
	}
	return out
}

// scaled is v of ing, whole and at least one if ing is counted in pieces.
func scaled(ing Ingredient, v float64) RecipeIngredient {
	if ing.Unit == Pieces {
		v = max(1, math.Round(v))
	}
	return RecipeIngredient{Ingredient: ing, Quantity: QuantityOf(v)}
}

// NutritionPerPortion is one of ps's Portions' nutritional values: its Recipe's Portion
// as written scaled by its size, or its Ingredient at its quantity.
func (ps PrepSession) NutritionPerPortion() PortionNutrition {
	if ps.Recipe == nil {
		return perPortion([]RecipeIngredient{{Ingredient: *ps.Ingredient, Quantity: ps.Quantity}}, 1)
	}
	return perPortion(ps.Recipe.Ingredients, float64(ps.Recipe.Portions)/ps.Size)
}

// Before reports whether s comes earlier than o.
func (s Slot) Before(o Slot) bool {
	if !s.On.Equal(o.On) {
		return s.On.Before(o.On)
	}
	return slices.Index(SlotKinds, s.Kind) < slices.Index(SlotKinds, o.Kind)
}

// String is s as the planner reads it: "dinner on Thu 5 Nov".
func (s Slot) String() string { return string(s.Kind) + " on " + s.On.Format(PlannerDay) }

// key is s as a map key: Slots on the same day in different zones are the same Slot.
func (s Slot) key() string { return s.On.Format(time.DateOnly) + " " + string(s.Kind) }

// Equal reports whether s and o are the same Slot: the same kind on the same day, in
// whatever zone each is.
func (s Slot) Equal(o Slot) bool { return s.key() == o.key() }

// Unreadable is why p can't be read as a Plan, one line each: a Portion size outside
// MinSize–MaxSize, a Prep Session yielding no Portion, a Portion without a Slot put in
// two. None when it can.
func (p Plan) Unreadable() []string {
	var problems []string
	for _, ps := range p.PrepSessions {
		if ps.Recipe != nil && (ps.Size < MinSize || ps.Size > MaxSize) {
			problems = append(problems, fmt.Sprintf("%s at %s has Portions of size %g, outside %g–%g", ps.Name(), ps.At, ps.Size, MinSize, MaxSize))
		}
		if len(ps.Portions) == 0 {
			problems = append(problems, fmt.Sprintf("%s at %s yields no Portion", ps.Name(), ps.At))
		}
	}
	put := map[int64]bool{}
	for _, k := range p.Kept {
		if put[k.Portion] {
			problems = append(problems, fmt.Sprintf("Portion %d is put in two Slots", k.Portion))
		}
		put[k.Portion] = true
	}
	return problems
}

// Rule is a rule a Plan can break.
type Rule string

// The hard rules a Plan's structure can break.
const (
	RuleSlotFilledTwice     Rule = "slot-filled-twice"
	RuleEmptySlot           Rule = "empty-slot"
	RuleEatenBeforePrepared Rule = "eaten-before-prepared"
	RuleUnknownPortion      Rule = "unknown-portion"
	RuleOutsideCycle        Rule = "outside-cycle"
)

// The hard rules a Plan's Recipes can break: eaten at a kind of Slot they aren't for, or
// needing an appliance the User doesn't own.
const (
	RuleWrongKindOfSlot   Rule = "wrong-kind-of-slot"
	RuleApplianceNotOwned Rule = "appliance-not-owned"
)

// RecipesUnfit are the violations of p's Prep Sessions of a Recipe: each Portion eaten at
// a kind of Slot the Recipe isn't for, and each appliance it needs that owned lacks.
func RecipesUnfit(p Plan, owned Appliances) []Violation {
	var vs []Violation
	for _, ps := range p.PrepSessions {
		if ps.Recipe == nil {
			continue
		}
		for _, pp := range ps.Portions {
			if pp.EatAt != nil && !slices.Contains(ps.Recipe.SlotKinds, pp.EatAt.Kind) {
				vs = append(vs, Violation{Rule: RuleWrongKindOfSlot, Message: fmt.Sprintf("%s is eaten at %s, a kind of Slot it isn't for: %s",
					ps.Name(), *pp.EatAt, strings.Join(Codes(ps.Recipe.SlotKinds), ", "))})
			}
		}
		for _, k := range ps.Recipe.Appliances {
			if _, ok := owned[k]; !ok {
				vs = append(vs, Violation{Rule: RuleApplianceNotOwned, Message: fmt.Sprintf("%s at %s needs the appliance %s, which the user doesn't own",
					ps.Name(), ps.At, k)})
			}
		}
	}
	return vs
}

// Codes are kinds by their codes, as the database stores them and the planner reads them.
func Codes[K ~string](kinds []K) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// The hard rules a Plan breaks by when it is: starting at a Slot already passed, and
// filling a Slot with an Exception, which step 7's Exceptions first reach.
const (
	RuleStartsInThePast   Rule = "starts-in-the-past"
	RuleSlotWithException Rule = "slot-with-exception"
)

// BlocksApproval reports whether breaking r stops the User approving a Plan: only the
// rules about time passing or an Exception do. Every hard rule stops the planner
// submitting one.
func (r Rule) BlocksApproval() bool {
	return r == RuleStartsInThePast || r == RuleSlotWithException
}

// StartsInThePast is the violation of a Plan whose first Slot has passed at now, its
// time of day by times reached, or none.
func StartsInThePast(first Slot, times SlotTimes, now time.Time) []Violation {
	if first.At(times).After(now) {
		return nil
	}
	return []Violation{{Rule: RuleStartsInThePast, Message: fmt.Sprintf("its first Slot, %s, has passed", first)}}
}

// Violation is a rule a Plan breaks, and where, said for the planner.
type Violation struct {
	Rule    Rule
	Message string
}

func (v Violation) String() string { return string(v.Rule) + ": " + v.Message }

// HardRules are the hard rules p breaks, planning slots, in order, with unslotted the
// Portions without a Slot it may put in them: none when it keeps them all.
func HardRules(p Plan, slots []Slot, unslotted []Portion) []Violation {
	var vs []Violation
	add := func(r Rule, format string, args ...any) {
		vs = append(vs, Violation{Rule: r, Message: fmt.Sprintf(format, args...)})
	}
	planned := map[string]bool{}
	for _, s := range slots {
		planned[s.key()] = true
	}
	filled := map[string]string{}
	fill := func(s Slot, name string) {
		if !planned[s.key()] {
			add(RuleOutsideCycle, "%s is eaten at %s, outside the Slots planned", name, s)
			return
		}
		if earlier, ok := filled[s.key()]; ok {
			add(RuleSlotFilledTwice, "%s is filled twice: by %s and by %s", s, earlier, name)
			return
		}
		filled[s.key()] = name
	}
	for _, ps := range p.PrepSessions {
		name := ps.Name()
		if !planned[ps.At.key()] {
			add(RuleOutsideCycle, "%s is prepared at %s, outside the Slots planned", name, ps.At)
		}
		for _, pp := range ps.Portions {
			if pp.EatAt == nil {
				continue
			}
			if pp.EatAt.Before(ps.At) {
				add(RuleEatenBeforePrepared, "%s is eaten at %s, before it is prepared at %s", name, *pp.EatAt, ps.At)
			}
			fill(*pp.EatAt, name)
		}
	}
	names := map[int64]string{}
	for _, u := range unslotted {
		names[u.ID] = u.Name
	}
	for _, k := range p.Kept {
		name, ok := names[k.Portion]
		if !ok {
			add(RuleUnknownPortion, "Portion %d put in %s is not a Portion without a Slot", k.Portion, k.At)
			continue
		}
		fill(k.At, fmt.Sprintf("Portion %d (%s)", k.Portion, name))
	}
	for _, s := range slots {
		if _, ok := filled[s.key()]; !ok {
			add(RuleEmptySlot, "%s is empty: no Portion fills it", s)
		}
	}
	return vs
}
