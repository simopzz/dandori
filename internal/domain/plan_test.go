package domain_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// meal is an Ingredient eaten as it is: 500 kcal and 25 g protein a piece.
var meal = domain.Ingredient{Name: "meal", Unit: domain.Pieces, Nutrition: domain.IngredientNutrition{Kcal: 500, ProteinG: 25}}

// single is a Prep Session of one meal, eaten straight away.
func single(on string, kind domain.SlotKind) domain.PrepSession {
	return domain.PrepSession{
		At:          *at(on, kind),
		Ingredients: []domain.RecipeIngredient{{Ingredient: meal, Quantity: 1}},
		Portions:    []domain.PlannedPortion{{Location: domain.Fridge, EatAt: at(on, kind)}},
	}
}

// week is the first Cycle, 2026-11-02 to 2026-11-08, with a meal at every
// breakfast, lunch and dinner and the kept Portion 7 as Sunday's snack.
// Against Targets of 2000 kcal and 100 g protein a day, with an Untracked
// Allowance of 200 kcal and 10 g, the week eats 21×500 + 7×200 = 11900 kcal
// and 21×25 + 7×10 = 595 g protein before the snack: a snack of 2100 kcal
// and 105 g lands it on Target. The Plan Proposal replaces the whole Plan,
// from the delivery on, and is checked the day before.
func week(snack domain.Nutrition) domain.PlanCheck {
	c := domain.PlanCheck{
		Cycle: domain.Cycle{DeliveryOn: day("2026-11-02"), EndsOn: day("2026-11-08")},
		From:  *at("2026-11-02", domain.Breakfast),
		Today: day("2026-11-01"),
		Since: day("2026-11-02"),
		Profile: domain.Profile{
			Targets:            domain.Nutrition{Kcal: 2000, ProteinG: 100},
			UntrackedAllowance: domain.Nutrition{Kcal: 200, ProteinG: 10},
		},
		Kept: []domain.KeptPortion{{ID: 7, Nutrition: snack}},
		Plan: domain.Plan{Kept: []domain.KeptAssignment{{PortionID: 7, Slot: *at("2026-11-08", domain.Snack)}}},
	}
	for d := 2; d <= 8; d++ {
		on := fmt.Sprintf("2026-11-%02d", d)
		for _, kind := range []domain.SlotKind{domain.Breakfast, domain.Lunch, domain.Dinner} {
			c.Plan.PrepSessions = append(c.Plan.PrepSessions, single(on, kind))
		}
	}
	return c
}

var onTarget = domain.Nutrition{Kcal: 2100, ProteinG: 105}

func rules(vs []domain.Violation) []domain.Rule {
	var rs []domain.Rule
	for _, v := range vs {
		rs = append(rs, v.Rule)
	}
	slices.Sort(rs)
	return rs
}

// checkPlan runs Plan validation on c and compares the rules broken.
func checkPlan(t *testing.T, c domain.PlanCheck, want ...domain.Rule) {
	t.Helper()
	got := domain.ValidatePlan(c)
	slices.Sort(want)
	if !slices.Equal(rules(got), want) {
		t.Errorf("ValidatePlan() broke %v, want %v:\n%v", rules(got), want, got)
	}
}

func TestPlanOnTargetBreaksNoRule(t *testing.T) {
	checkPlan(t, week(onTarget))
}

func TestPlanYieldAndSlots(t *testing.T) {
	tests := []struct {
		name   string
		change func(p *domain.Plan)
		want   []domain.Rule
	}{
		{
			"a Prep Session yielding no Portion",
			func(p *domain.Plan) {
				s := single("2026-11-04", domain.Snack)
				s.Portions = nil
				p.PrepSessions = append(p.PrepSessions, s)
			},
			[]domain.Rule{domain.RuleNoPortions},
		},
		{
			"a Portion with no eating planned",
			func(p *domain.Plan) { p.PrepSessions[0].Portions[0].EatAt = nil },
			// Monday's breakfast is no longer eaten: 500 kcal less is within
			// 5%, 25 g protein less is under the floor.
			[]domain.Rule{domain.RuleUnplannedPortion, domain.RuleProtein},
		},
		{
			"a Portion eaten after the Cycle stays without a Slot",
			func(p *domain.Plan) {
				s := single("2026-11-08", domain.Snack)
				s.Portions = append(s.Portions, domain.PlannedPortion{Location: domain.Freezer, EatAt: at("2026-11-12", domain.Lunch)})
				s.Portions[0].EatAt = at("2026-11-10", domain.Dinner)
				p.PrepSessions = append(p.PrepSessions, s)
			},
			nil,
		},
		{
			"a Portion eaten before the Cycle",
			func(p *domain.Plan) { p.PrepSessions[0].Portions[0].EatAt = at("2026-11-01", domain.Dinner) },
			[]domain.Rule{domain.RuleBeforeStart, domain.RuleEatenBeforePrepared, domain.RuleProtein},
		},
		{
			"a Prep Session outside the Cycle",
			func(p *domain.Plan) {
				s := single("2026-11-09", domain.Breakfast)
				p.PrepSessions = append(p.PrepSessions, s)
			},
			[]domain.Rule{domain.RulePrepSessionOutsideCycle},
		},
		{
			"two Portions in one Slot",
			func(p *domain.Plan) { p.PrepSessions = append(p.PrepSessions, single("2026-11-03", domain.Lunch)) },
			[]domain.Rule{domain.RuleSlotFilledTwice},
		},
		{
			"a kept Portion in two Slots",
			func(p *domain.Plan) {
				p.Kept = append(p.Kept, domain.KeptAssignment{PortionID: 7, Slot: *at("2026-11-07", domain.Snack)})
			},
			[]domain.Rule{domain.RulePortionInTwoSlots, domain.RuleCalories},
		},
		{
			"a Portion that isn't kept",
			func(p *domain.Plan) {
				p.Kept = append(p.Kept, domain.KeptAssignment{PortionID: 8, Slot: *at("2026-11-07", domain.Snack)})
			},
			[]domain.Rule{domain.RuleUnknownPortion},
		},
		{
			"a kept Portion in a Slot outside the Cycle",
			func(p *domain.Plan) { p.Kept[0].Slot = *at("2026-11-09", domain.Snack) },
			[]domain.Rule{domain.RuleEatenOutsideCycle, domain.RuleCalories, domain.RuleProtein},
		},
		{
			"a kept Portion left unused while new Prep Sessions are scheduled",
			func(p *domain.Plan) { p.Kept = nil },
			[]domain.Rule{domain.RuleUnusedPortion, domain.RuleCalories, domain.RuleProtein},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := week(onTarget)
			tt.change(&c.Plan)
			checkPlan(t, c, tt.want...)
		})
	}
}

func TestKeptPortionsMayStayUnusedWhenNoPrepSessionIsScheduled(t *testing.T) {
	c := week(onTarget)
	c.Plan = domain.Plan{}
	c.Since = c.Cycle.EndsOn // no trailing window to check
	checkPlan(t, c)
}

func TestPlanTargetBoundaries(t *testing.T) {
	// Before the snack the week eats 11900 kcal and 595 g protein; seven
	// days of the Targets are 14000 kcal (±5%: 13300–14700) and 700 g.
	tests := []struct {
		name  string
		snack domain.Nutrition
		want  []domain.Rule
	}{
		{"calories on the lower bound", domain.Nutrition{Kcal: 1400, ProteinG: 105}, nil},
		{"calories just under the lower bound", domain.Nutrition{Kcal: 1399, ProteinG: 105}, []domain.Rule{domain.RuleCalories}},
		{"calories on the upper bound", domain.Nutrition{Kcal: 2800, ProteinG: 105}, nil},
		{"calories just over the upper bound", domain.Nutrition{Kcal: 2801, ProteinG: 105}, []domain.Rule{domain.RuleCalories}},
		{"protein on the floor", domain.Nutrition{Kcal: 2100, ProteinG: 105}, nil},
		{"protein well over the floor", domain.Nutrition{Kcal: 2100, ProteinG: 400}, nil},
		{"protein just under the floor", domain.Nutrition{Kcal: 2100, ProteinG: 104}, []domain.Rule{domain.RuleProtein}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkPlan(t, week(tt.snack), tt.want...)
		})
	}
}

func TestTrailingSevenDays(t *testing.T) {
	// A second Cycle, 2026-11-09 to 2026-11-10, after the first one ended
	// on 2026-11-08. Its days eat one 500 kcal, 25 g meal each, plus the
	// 200 kcal, 10 g Untracked Allowance.
	earlierDay := domain.Nutrition{Kcal: 1800, ProteinG: 90}
	fullWeek := func() []domain.DayNutrition {
		var days []domain.DayNutrition
		for d := 2; d <= 8; d++ {
			days = append(days, domain.DayNutrition{On: day(fmt.Sprintf("2026-11-%02d", d)), Nutrition: earlierDay})
		}
		return days
	}
	tests := []struct {
		name    string
		since   string
		earlier []domain.DayNutrition
		// want is the trailing seven days' calories and protein on the
		// Cycle's two days; nil when they aren't checked.
		want [2]*domain.Nutrition
	}{
		{
			// To Mon 9: 3–8 Nov eat 6×2000 kcal and 6×100 g, the 9th 700 kcal and 35 g.
			name: "across the Cycle boundary", since: "2026-11-02", earlier: fullWeek(),
			want: [2]*domain.Nutrition{{Kcal: 12700, ProteinG: 635}, {Kcal: 11400, ProteinG: 570}},
		},
		{
			// 7 and 8 Nov had no Cycle: they count only the Untracked Allowance.
			name: "across days between Cycles", since: "2026-11-02", earlier: fullWeek()[:5],
			want: [2]*domain.Nutrition{{Kcal: 9100, ProteinG: 455}, {Kcal: 7800, ProteinG: 390}},
		},
		{
			// The first Cycle was delivered on 4 Nov: the window to the 9th
			// reaches before it, the one to the 10th doesn't.
			name: "reaching before the first Cycle", since: "2026-11-04", earlier: fullWeek(),
			want: [2]*domain.Nutrition{nil, {Kcal: 11400, ProteinG: 570}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := domain.PlanCheck{
				Cycle: domain.Cycle{DeliveryOn: day("2026-11-09"), EndsOn: day("2026-11-10")},
				Since: day(tt.since),
				Profile: domain.Profile{
					Targets:            domain.Nutrition{Kcal: 2000, ProteinG: 100},
					UntrackedAllowance: domain.Nutrition{Kcal: 200, ProteinG: 10},
				},
				Earlier: tt.earlier,
				Plan: domain.Plan{PrepSessions: []domain.PrepSession{
					single("2026-11-09", domain.Dinner), single("2026-11-10", domain.Dinner),
				}},
			}
			days := c.Days()
			if len(days) != 2 {
				t.Fatalf("Days() has %d days, want 2", len(days))
			}
			for i, d := range days {
				got, want := d.TrailingSeven, tt.want[i]
				if (got == nil) != (want == nil) || got != nil && (got.Kcal != want.Kcal || got.ProteinG != want.ProteinG) {
					t.Errorf("trailing seven days to %s = %+v, want %+v", d.On.Format(time.DateOnly), got, want)
				}
			}
			if !days[0].On.Equal(day("2026-11-09")) || days[0].Eaten.Kcal != 700 || days[0].Eaten.ProteinG != 35 {
				t.Errorf("Days()[0] = %+v, want 9 Nov eating 700 kcal and 35 g", days[0])
			}
		})
	}
}

func TestEarlyClosedCycleEndsTheDayBeforeTheNextDelivery(t *testing.T) {
	closed := day("2026-11-06")
	c := week(onTarget)
	c.Cycle.ClosedOn = &closed
	c.Plan = domain.Plan{}
	if days := c.Days(); len(days) != 4 || !days[3].On.Equal(day("2026-11-05")) {
		t.Errorf("Days() of a Cycle closed on 6 Nov = %d days, want 2–5 Nov", len(days))
	}
}

// batch is a dinner Prep Session on 2026-11-02 yielding Portions eaten
// at each of eats; it replaces the meal of each Slot it fills.
func batch(p *domain.Plan, eats ...*domain.Slot) {
	s := single("2026-11-02", domain.Dinner)
	s.Ingredients[0].Quantity = float64(len(eats))
	s.Portions = nil
	for _, e := range eats {
		s.Portions = append(s.Portions, domain.PlannedPortion{Location: domain.Fridge, EatAt: e})
	}
	p.PrepSessions = slices.DeleteFunc(p.PrepSessions, func(ps domain.PrepSession) bool {
		return slices.ContainsFunc(eats, func(e *domain.Slot) bool { return *ps.Portions[0].EatAt == *e })
	})
	p.PrepSessions = append(p.PrepSessions, s)
}

func TestPortionsOfAPrepSessionAreNotEatenBackToBack(t *testing.T) {
	tests := []struct {
		name string
		eats []*domain.Slot
		want []domain.Rule
	}{
		{
			"now, at lunch two days later and two days after that",
			[]*domain.Slot{at("2026-11-02", domain.Dinner), at("2026-11-04", domain.Lunch), at("2026-11-06", domain.Dinner)},
			nil,
		},
		{
			"the third three days after the second",
			[]*domain.Slot{at("2026-11-02", domain.Dinner), at("2026-11-04", domain.Lunch), at("2026-11-07", domain.Dinner)},
			nil,
		},
		{
			"on the next day",
			[]*domain.Slot{at("2026-11-02", domain.Dinner), at("2026-11-03", domain.Lunch)},
			[]domain.Rule{domain.RuleBackToBack},
		},
		{
			"twice on one day",
			[]*domain.Slot{at("2026-11-02", domain.Dinner), at("2026-11-04", domain.Lunch), at("2026-11-04", domain.Dinner)},
			[]domain.Rule{domain.RuleBackToBack},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := week(onTarget)
			batch(&c.Plan, tt.eats...)
			checkPlan(t, c, tt.want...)
		})
	}
}

func TestPlanRespectsExclusionsAndAllergies(t *testing.T) {
	prawns := domain.Ingredient{Name: "gamberi", Unit: domain.Grams, FallsUnder: []string{"shellfish", "seafood"}}
	pork := domain.Ingredient{Name: "salsiccia", Unit: domain.Grams, FallsUnder: []string{"pork"}}
	rice := domain.Ingredient{Name: "riso", Unit: domain.Grams, FallsUnder: []string{"gluten-free"}}
	tests := []struct {
		name        string
		ingredients []domain.Ingredient
		inRecipe    bool
		want        []domain.Rule
	}{
		{"an Ingredient under no term of the Profile", []domain.Ingredient{rice}, false, nil},
		{"an Ingredient under an allergy", []domain.Ingredient{rice, prawns}, false, []domain.Rule{domain.RuleAllergen}},
		{"an Ingredient under an exclusion", []domain.Ingredient{pork}, false, []domain.Rule{domain.RuleExcluded}},
		{"a Recipe's Ingredient under an allergy", []domain.Ingredient{prawns}, true, []domain.Rule{domain.RuleAllergen}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := week(onTarget)
			c.Profile.Exclusions = []string{"pork"}
			c.Profile.Allergies = []string{"shellfish"}
			ps := &c.Plan.PrepSessions[0]
			var ris []domain.RecipeIngredient
			for _, ing := range tt.ingredients {
				ris = append(ris, domain.RecipeIngredient{Ingredient: ing, Quantity: 1})
			}
			ps.Ingredients = append(ps.Ingredients, ris...)
			if tt.inRecipe {
				ps.Recipe = &domain.Recipe{Name: "Risotto ai gamberi", Portions: 1, Ingredients: ps.Ingredients}
				ps.Ingredients = nil
			}
			checkPlan(t, c, tt.want...)
		})
	}
}

// midWeek is week's Plan Proposal replacing the Plan from Thursday's
// breakfast, checked on Wednesday. Monday to Wednesday stay as the Plan
// has them: each day eats 1500 kcal and 75 g protein in its Slots.
func midWeek() domain.PlanCheck {
	c := week(onTarget)
	c.From, c.Today = *at("2026-11-05", domain.Breakfast), day("2026-11-04")
	c.Plan.PrepSessions = slices.DeleteFunc(c.Plan.PrepSessions, func(ps domain.PrepSession) bool {
		return ps.At.On.Before(c.From.On)
	})
	for _, d := range []string{"2026-11-02", "2026-11-03", "2026-11-04"} {
		c.Earlier = append(c.Earlier, domain.DayNutrition{On: day(d), Nutrition: domain.Nutrition{Kcal: 1500, ProteinG: 75}})
	}
	return c
}

func TestPlanProposalChangesThePlanOnlyFromItsStart(t *testing.T) {
	tests := []struct {
		name   string
		change func(c *domain.PlanCheck)
		want   []domain.Rule
	}{
		{"what stays before the start counts toward the Targets", func(*domain.PlanCheck) {}, nil},
		{
			"a Prep Session before the start",
			func(c *domain.PlanCheck) {
				s := single("2026-11-04", domain.Dinner)
				s.Portions[0].EatAt = at("2026-11-07", domain.Snack)
				c.Plan.PrepSessions = append(c.Plan.PrepSessions, s)
			},
			[]domain.Rule{domain.RuleBeforeStart},
		},
		{
			"a Portion eaten before the start",
			func(c *domain.PlanCheck) {
				s := single("2026-11-05", domain.Snack)
				s.Portions[0].EatAt = at("2026-11-04", domain.Snack)
				c.Plan.PrepSessions = append(c.Plan.PrepSessions, s)
			},
			[]domain.Rule{domain.RuleBeforeStart, domain.RuleEatenBeforePrepared},
		},
		{
			// It would no longer be eaten: the week misses the snack's 2100 kcal.
			"a kept Portion put in a Slot before the start",
			func(c *domain.PlanCheck) { c.Plan.Kept[0].Slot = *at("2026-11-04", domain.Snack) },
			[]domain.Rule{domain.RuleBeforeStart, domain.RuleCalories, domain.RuleProtein},
		},
		{
			"a start already past",
			func(c *domain.PlanCheck) { c.Today = day("2026-11-06") },
			[]domain.Rule{domain.RuleStartsInPast},
		},
		{"a start today", func(c *domain.PlanCheck) { c.Today = day("2026-11-05") }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := midWeek()
			tt.change(&c)
			checkPlan(t, c, tt.want...)
		})
	}
}

func TestDaysBeforeTheStartShowWhatStays(t *testing.T) {
	days := midWeek().Days()
	// Monday keeps its 1500 kcal, Thursday has three new meals of 500; each
	// adds the 200 kcal Untracked Allowance.
	if len(days) != 7 || days[0].Eaten.Kcal != 1700 || days[3].Eaten.Kcal != 1700 {
		t.Errorf("Days() = %+v, want 1700 kcal on Monday and on Thursday", days)
	}
}
