package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Rule is a rule of the model a Plan can break.
type Rule string

const (
	RuleNoPortions              Rule = "no-portions"
	RuleUnplannedPortion        Rule = "unplanned-portion"
	RuleStartsInPast            Rule = "starts-in-past"
	RuleBeforeStart             Rule = "before-start"
	RuleAlreadyPrepared         Rule = "already-prepared"
	RulePrepSessionOutsideCycle Rule = "prep-session-outside-cycle"
	RuleEatenOutsideCycle       Rule = "eaten-outside-cycle"
	RuleEatenBeforePrepared     Rule = "eaten-before-prepared"
	RuleBackToBack              Rule = "back-to-back"
	RuleSlotFilledTwice         Rule = "slot-filled-twice"
	RulePortionInTwoSlots       Rule = "portion-in-two-slots"
	RuleUnknownPortion          Rule = "unknown-portion"
	RuleUnusedPortion           Rule = "unused-portion"
	RuleCalories                Rule = "calories"
	RuleProtein                 Rule = "protein"
	RuleExcluded                Rule = "excluded"
	RuleAllergen                Rule = "allergen"
	RuleExceptedSlot            Rule = "excepted-slot"
)

// Violation is a rule a Plan breaks, and where, said for the user.
type Violation struct {
	Rule    Rule
	Message string
}

func (v Violation) String() string { return string(v.Rule) + ": " + v.Message }

// KeptPortion is a Portion without a Slot a Plan may put in one.
type KeptPortion struct {
	ID int64
	// Name is what the Portion is: its Recipe, or its Ingredients.
	Name      string
	Nutrition Nutrition
}

// DayNutrition is what is planned to be eaten on a day, in its Slots.
type DayNutrition struct {
	On        time.Time
	Nutrition Nutrition
}

// PlanCheck is a proposed Plan with everything Plan validation needs.
type PlanCheck struct {
	Cycle Cycle
	// Plan is the Cycle's Plan from From on: a Plan Proposal replaces only
	// that part, and what comes before it stays.
	Plan Plan
	// From is the first Slot the Plan replaces; Today is the day it is
	// checked on, which From may not come before.
	From    Slot
	Today   time.Time
	Profile Profile
	// Kept are the Portions the Plan may put in Slots: those without a Slot,
	// and those prepared before From and not eaten before it.
	Kept []KeptPortion
	// Earlier is what Slots before From eat: earlier Cycles', and this
	// Cycle's that stay. A day missing from it, such as one between Cycles,
	// counts only the Untracked Allowance and its Meal Outs.
	Earlier []DayNutrition
	// Exceptions are the Slots logged as a Meal Out or skipped, from six
	// days before the Cycle's first day to its last: all its trailing
	// seven days can reach.
	Exceptions []Exception
	// Since is the first Cycle's delivery date: nothing was planned before
	// it, so trailing seven days reaching before it aren't checked.
	Since time.Time
}

// PlanDay is one day of a Cycle under a Plan.
type PlanDay struct {
	On time.Time
	// Eaten is what the day's Slots eat plus the Untracked Allowance.
	Eaten Nutrition
	// TrailingSeven is what the seven days up to this one eat, or nil when
	// they reach before Since.
	TrailingSeven *Nutrition
}

// NutritionPerPortion is what one Portion of the Prep Session holds: a
// Recipe's Portion, or the Ingredients set out divided over its Portions.
func (ps PrepSession) NutritionPerPortion() Nutrition {
	if ps.Recipe != nil {
		return ps.Recipe.NutritionPerPortion()
	}
	return Recipe{Portions: len(ps.Portions), Ingredients: ps.Ingredients}.NutritionPerPortion()
}

func (ps PrepSession) ingredients() []Ingredient {
	ris := ps.Ingredients
	if ps.Recipe != nil {
		ris = ps.Recipe.Ingredients
	}
	var ings []Ingredient
	for _, ri := range ris {
		ings = append(ings, ri.Ingredient)
	}
	return ings
}

// Name is what the Prep Session prepares: its Recipe, or its Ingredients.
func (ps PrepSession) Name() string {
	if ps.Recipe != nil {
		return ps.Recipe.Name
	}
	var names []string
	for _, ri := range ps.Ingredients {
		names = append(names, ri.Ingredient.Name)
	}
	return strings.Join(names, ", ")
}

func (s Slot) String() string { return string(s.Kind) + " on " + s.On.Format("Mon 2 Jan") }

func (s Slot) key() string { return s.On.Format(time.DateOnly) + " " + string(s.Kind) }

func (n Nutrition) plus(o Nutrition) Nutrition {
	return Nutrition{Kcal: n.Kcal + o.Kcal, ProteinG: n.ProteinG + o.ProteinG, CarbsG: n.CarbsG + o.CarbsG, FatG: n.FatG + o.FatG}
}

// inPlan reports whether s is a Slot the Plan fills: from From to the
// Cycle's last Slot.
func (c PlanCheck) inPlan(s Slot) bool {
	return !s.Before(c.From) && s.Before(c.Cycle.Until())
}

// excepted is the Exceptions by their Slot's key.
func (c PlanCheck) excepted() map[string]Exception {
	excepted := map[string]Exception{}
	for _, e := range c.Exceptions {
		excepted[e.Slot.key()] = e
	}
	return excepted
}

// Days is every day of the Cycle under the Plan, in order. An excepted
// Slot's Portion isn't eaten: only its Exception counts.
func (c PlanCheck) Days() []PlanDay {
	excepted := c.excepted()
	eats := func(s Slot) bool {
		_, ok := excepted[s.key()]
		return c.inPlan(s) && !ok
	}
	slots := map[string]Nutrition{}
	for _, ps := range c.Plan.PrepSessions {
		n := ps.NutritionPerPortion()
		for _, p := range ps.Portions {
			if p.EatAt != nil && eats(*p.EatAt) {
				day := p.EatAt.On.Format(time.DateOnly)
				slots[day] = slots[day].plus(n)
			}
		}
	}
	kept := map[int64]Nutrition{}
	for _, k := range c.Kept {
		kept[k.ID] = k.Nutrition
	}
	for _, k := range c.Plan.Kept {
		if n, ok := kept[k.PortionID]; ok && eats(k.Slot) {
			day := k.Slot.On.Format(time.DateOnly)
			slots[day] = slots[day].plus(n)
		}
	}
	for _, e := range c.Earlier {
		day := e.On.Format(time.DateOnly)
		slots[day] = slots[day].plus(e.Nutrition)
	}
	for _, e := range c.Exceptions {
		day := e.Slot.On.Format(time.DateOnly)
		slots[day] = slots[day].plus(e.Nutrition(c.Profile.MealOut))
	}
	eaten := func(d time.Time) Nutrition {
		return slots[d.Format(time.DateOnly)].plus(c.Profile.UntrackedAllowance)
	}

	var days []PlanDay
	for d := c.Cycle.Start.On; !d.After(c.Cycle.LastDay()); d = d.AddDate(0, 0, 1) {
		day := PlanDay{On: d, Eaten: eaten(d)}
		if start := d.AddDate(0, 0, -6); !start.Before(c.Since) {
			var sum Nutrition
			for w := start; !w.After(d); w = w.AddDate(0, 0, 1) {
				sum = sum.plus(eaten(w))
			}
			day.TrailingSeven = &sum
		}
		days = append(days, day)
	}
	return days
}

// ValidatePlan returns every rule of the model the Plan breaks; none when
// it keeps them all.
func ValidatePlan(c PlanCheck) []Violation {
	var vs []Violation
	add := func(r Rule, format string, args ...any) {
		vs = append(vs, Violation{Rule: r, Message: fmt.Sprintf(format, args...)})
	}
	excepted := c.excepted()
	filled := map[string]string{}
	fill := func(s Slot, name string) {
		if e, ok := excepted[s.key()]; ok {
			add(RuleExceptedSlot, "%s is eaten at %s, which is %s", name, s, e.Kind.Describe())
		}
		if earlier, ok := filled[s.key()]; ok {
			add(RuleSlotFilledTwice, "%s is filled twice: by %s and by %s", s, earlier, name)
			return
		}
		filled[s.key()] = name
	}

	past := c.From.On.Before(c.Today)
	if past {
		add(RuleStartsInPast, "The Plan Proposal changes the Plan from %s, which is past: what was prepared and eaten since stays", c.From)
	}
	for _, ps := range c.Plan.PrepSessions {
		name := ps.Name()
		switch {
		case ps.At.Before(c.From):
			add(RuleBeforeStart, "%s is prepared for %s, before the Plan Proposal starts at %s", name, ps.At, c.From)
		case !past && ps.At.Before(FirstUnprepared(c.From, c.Today)):
			add(RuleAlreadyPrepared, "%s is prepared for %s, today: what is prepared today is taken as done, so new Prep Sessions start tomorrow", name, ps.At)
		case !ps.At.Before(c.Cycle.Until()):
			add(RulePrepSessionOutsideCycle, "%s is prepared for %s, after the Cycle ends", name, ps.At)
		}
		if e, ok := excepted[ps.At.key()]; ok {
			add(RuleExceptedSlot, "%s is prepared for %s, which is %s", name, ps.At, e.Kind.Describe())
		}
		if len(ps.Portions) == 0 {
			add(RuleNoPortions, "%s at %s yields no Portion", name, ps.At)
		}
		for _, ing := range ps.ingredients() {
			for _, term := range ing.FallsUnder {
				switch {
				case slices.Contains(c.Profile.Allergies, term):
					add(RuleAllergen, "%s at %s has %s, which is %s: an allergy", name, ps.At, ing.Name, term)
				case slices.Contains(c.Profile.Exclusions, term):
					add(RuleExcluded, "%s at %s has %s, which is %s: excluded", name, ps.At, ing.Name, term)
				}
			}
		}
		// Portions of one Prep Session are at least two days apart, so the
		// same thing is never eaten back to back.
		var eaten []time.Time
		for _, p := range ps.Portions {
			if p.EatAt == nil {
				continue
			}
			for _, other := range eaten {
				if gap := p.EatAt.On.Sub(other).Hours() / 24; gap > -2 && gap < 2 {
					add(RuleBackToBack, "%s is eaten back to back, at %s and on %s", name, *p.EatAt, other.Format("Mon 2 Jan"))
					break
				}
			}
			eaten = append(eaten, p.EatAt.On)
		}
		for _, p := range ps.Portions {
			switch {
			case p.EatAt == nil:
				add(RuleUnplannedPortion, "%s at %s yields a Portion with no eating planned", name, ps.At)
				continue
			case p.EatAt.Before(ps.At):
				add(RuleEatenBeforePrepared, "%s is eaten at %s, before it is prepared at %s", name, *p.EatAt, ps.At)
			}
			switch {
			case p.EatAt.Before(c.From):
				add(RuleBeforeStart, "%s is eaten at %s, before the Plan Proposal starts at %s", name, *p.EatAt, c.From)
			case c.inPlan(*p.EatAt):
				fill(*p.EatAt, name)
			}
		}
	}

	kept := map[int64]KeptPortion{}
	for _, k := range c.Kept {
		kept[k.ID] = k
	}
	used := map[int64]bool{}
	for _, k := range c.Plan.Kept {
		portion, ok := kept[k.PortionID]
		switch {
		case !ok:
			add(RuleUnknownPortion, "Portion %d put in %s is not a Portion without a Slot", k.PortionID, k.Slot)
			continue
		case used[k.PortionID]:
			add(RulePortionInTwoSlots, "The kept Portion of %s is put in two Slots, the second at %s", portion.Name, k.Slot)
			continue
		}
		used[k.PortionID] = true
		switch {
		case k.Slot.Before(c.From):
			add(RuleBeforeStart, "The kept Portion of %s is put in %s, before the Plan Proposal starts at %s", portion.Name, k.Slot, c.From)
			continue
		case !c.inPlan(k.Slot):
			add(RuleEatenOutsideCycle, "The kept Portion of %s is put in %s, after the Cycle ends", portion.Name, k.Slot)
			continue
		}
		fill(k.Slot, "the kept Portion of "+portion.Name)
	}
	if len(c.Plan.PrepSessions) > 0 {
		for _, k := range c.Kept {
			if !used[k.ID] {
				add(RuleUnusedPortion, "The kept Portion of %s is left without a Slot while new Prep Sessions are scheduled", k.Name)
			}
		}
	}

	t := c.Profile.Targets
	for _, d := range c.Days() {
		w := d.TrailingSeven
		if w == nil {
			continue
		}
		to := d.On.Format("Mon 2 Jan")
		if !CaloriesWithinTarget(t.Kcal, w.Kcal) {
			add(RuleCalories, "The seven days to %s eat %d kcal, outside %d–%d", to, w.Kcal, 7*t.Kcal*95/100, 7*t.Kcal*105/100)
		}
		if w.ProteinG < 7*t.ProteinG {
			add(RuleProtein, "The seven days to %s eat %d g protein, under %d", to, w.ProteinG, 7*t.ProteinG)
		}
	}
	return vs
}
