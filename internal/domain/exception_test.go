package domain_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

func TestExceptedSlotGetsNoPortionAndNoPrepSession(t *testing.T) {
	mealOut := func(on string, kind domain.SlotKind) domain.Exception {
		return domain.Exception{Slot: *at(on, kind), Kind: domain.MealOut}
	}
	skipped := func(on string, kind domain.SlotKind) domain.Exception {
		return domain.Exception{Slot: *at(on, kind), Kind: domain.Skipped}
	}
	tests := []struct {
		name       string
		exceptions []domain.Exception
		change     func(p *domain.Plan)
		want       []domain.Rule
	}{
		{
			name:       "an Exception on a Slot the Plan leaves empty",
			exceptions: []domain.Exception{skipped("2026-11-04", domain.Snack)},
		},
		{
			// Wednesday's lunch is the meal prepared and eaten then.
			// The lunch isn't eaten either: the week falls short of protein.
			name:       "a Portion and its Prep Session on a skipped Slot",
			exceptions: []domain.Exception{skipped("2026-11-04", domain.Lunch)},
			want:       []domain.Rule{domain.RuleExceptedSlot, domain.RuleExceptedSlot, domain.RuleProtein},
		},
		{
			// Thursday's breakfast is prepared then and eaten at the Meal
			// Out's lunch.
			name:       "a Portion on a Meal Out",
			exceptions: []domain.Exception{mealOut("2026-11-05", domain.Snack)},
			change: func(p *domain.Plan) {
				s := single("2026-11-05", domain.Breakfast)
				s.Portions[0].EatAt = at("2026-11-05", domain.Snack)
				p.PrepSessions[9] = s
			},
			// Nor is the breakfast: short of protein.
			want: []domain.Rule{domain.RuleExceptedSlot, domain.RuleProtein},
		},
		{
			// Thursday's breakfast is still prepared at the Meal Out's
			// breakfast, but eaten at the snack.
			name:       "a Prep Session on a Meal Out",
			exceptions: []domain.Exception{mealOut("2026-11-05", domain.Breakfast)},
			change: func(p *domain.Plan) {
				p.PrepSessions[9].Portions[0].EatAt = at("2026-11-05", domain.Snack)
			},
			want: []domain.Rule{domain.RuleExceptedSlot},
		},
		{
			name:       "a kept Portion on a Meal Out",
			exceptions: []domain.Exception{mealOut("2026-11-08", domain.Snack)},
			// Nor the kept Portion: short of calories and protein.
			want: []domain.Rule{domain.RuleExceptedSlot, domain.RuleCalories, domain.RuleProtein},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := week(onTarget)
			// The Meal Out counts nothing here, so only an uneaten Portion
			// moves the Targets.
			c.Profile.MealOut = domain.Nutrition{}
			c.Exceptions = tt.exceptions
			if tt.change != nil {
				tt.change(&c.Plan)
			}
			checkPlan(t, c, tt.want...)
		})
	}
}

func TestMealOutCountsTowardTheTrailingSevenDays(t *testing.T) {
	kcal := 1000
	// The second Cycle of TestTrailingSevenDays, 2026-11-09 to 2026-11-10,
	// after a first eating 1800 kcal and 90 g a day, and a default Meal Out
	// estimate of 800 kcal and 40 g.
	c := domain.PlanCheck{
		Cycle: delivered("2026-11-09", "07:00", "07:30", "2026-11-10"),
		Since: day("2026-11-02"),
		Profile: domain.Profile{
			Targets:            domain.Nutrition{Kcal: 2000, ProteinG: 100},
			UntrackedAllowance: domain.Nutrition{Kcal: 200, ProteinG: 10},
			MealOut:            domain.Nutrition{Kcal: 800, ProteinG: 40},
		},
		Plan: domain.Plan{PrepSessions: []domain.PrepSession{
			single("2026-11-09", domain.Dinner), single("2026-11-10", domain.Dinner),
		}},
		Exceptions: []domain.Exception{
			// Before both windows: counted in neither.
			{Slot: *at("2026-11-02", domain.Dinner), Kind: domain.MealOut},
			// In the first Cycle: the default estimate.
			{Slot: *at("2026-11-08", domain.Dinner), Kind: domain.MealOut},
			// Its own calories, the default protein.
			{Slot: *at("2026-11-09", domain.Lunch), Kind: domain.MealOut, Estimate: domain.Estimate{Kcal: &kcal}},
			{Slot: *at("2026-11-10", domain.Breakfast), Kind: domain.Skipped},
		},
	}
	for d := 2; d <= 8; d++ {
		c.Earlier = append(c.Earlier, domain.DayNutrition{On: day(fmt.Sprintf("2026-11-%02d", d)), Nutrition: domain.Nutrition{Kcal: 1800, ProteinG: 90}})
	}
	days := c.Days()
	// TestTrailingSevenDays's 12700 kcal, 635 g and 11400 kcal, 570 g,
	// with 800 + 1000 kcal and 40 + 40 g more.
	want := []struct{ eatenKcal, eatenProtein, kcal, protein int }{
		{1700, 75, 14500, 715},
		{700, 35, 13200, 650},
	}
	if len(days) != len(want) {
		t.Fatalf("Days() has %d days, want %d", len(days), len(want))
	}
	for i, d := range days {
		w := want[i]
		if d.Eaten.Kcal != w.eatenKcal || d.Eaten.ProteinG != w.eatenProtein {
			t.Errorf("%s eats %+v, want %d kcal and %d g", d.On.Format(time.DateOnly), d.Eaten, w.eatenKcal, w.eatenProtein)
		}
		if d.TrailingSeven == nil || d.TrailingSeven.Kcal != w.kcal || d.TrailingSeven.ProteinG != w.protein {
			t.Errorf("trailing seven days to %s = %+v, want %d kcal and %d g", d.On.Format(time.DateOnly), d.TrailingSeven, w.kcal, w.protein)
		}
	}
}

func TestOnlyASlotStillToComeIsLoggedAhead(t *testing.T) {
	tests := []struct {
		now  string
		slot *domain.Slot
		want bool
	}{
		{"2026-11-05 12:59", at("2026-11-05", domain.Lunch), true},
		{"2026-11-05 13:00", at("2026-11-05", domain.Lunch), false},
		{"2026-11-04 23:00", at("2026-11-05", domain.Breakfast), true},
		{"2026-11-06 07:00", at("2026-11-05", domain.Dinner), false},
	}
	for _, tt := range tests {
		now, err := time.Parse(time.DateOnly+" 15:04", tt.now)
		if err != nil {
			t.Fatal(err)
		}
		if got := tt.slot.ToCome(now, domain.DefaultSlotTimes); got != tt.want {
			t.Errorf("%s ToCome(%s) = %v, want %v", tt.slot, tt.now, got, tt.want)
		}
	}
}
