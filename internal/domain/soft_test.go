package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// aim are the Targets the soft rules are judged against: 2000 kcal within 10%, at least
// 100 g protein and 30 g fibre, at most 50 g sugar.
var aim = domain.Targets{Kcal: 2000, Protein: 100, Fibre: 30, Sugar: 50, Tolerance: 10}

// values are a Portion's or a day's `kcal`, `protein`, `fibre` and `sugar`, `fibre` and
// `sugar` each unknown when nil, and only at least that much when floor says so.
func values(kcal, protein float64, fibre, sugar *float64, fibreFloor, sugarFloor bool) domain.PortionNutrition {
	amount := func(v *float64) domain.Amount {
		if v == nil {
			return domain.Unknown
		}
		return domain.Known(*v)
	}
	return domain.PortionNutrition{
		Nutrition:  domain.Nutrition{Kcal: domain.Known(kcal), Protein: domain.Known(protein), Fibre: amount(fibre), Sugar: amount(sugar)},
		FibreFloor: fibreFloor, SugarFloor: sugarFloor,
	}
}

// g is `v` grams, for values.
func g(v float64) *float64 { return &v }

// A day eats the sum of the Portions in its Slots: a value is known when any of them
// knows it, and only at least that much when another doesn't or knows only that much; a
// day no Plan fills knows nothing. Unknown fibre or sugar is never counted as none.
func TestADaysNutritionIsTheSumOfItsPortions(t *testing.T) {
	soup := domain.Portion{ID: 9, Name: "Soup", Nutrition: values(400, 20, nil, g(5), false, false).Nutrition}
	plan := domain.Plan{
		PrepSessions: []domain.PrepSession{
			{At: slot(5, domain.Dinner), Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{
				eaten(in(slot(5, domain.Dinner))), eaten(in(slot(6, domain.Lunch))), eaten(nil),
			}},
			{At: slot(6, domain.Breakfast), Ingredient: &yogurt, Quantity: grams(150), Portions: []domain.PlannedPortion{eaten(in(slot(6, domain.Breakfast)))}},
		},
		Kept: []domain.Kept{{Portion: 9, At: slot(6, domain.Dinner)}},
	}
	days := domain.Days(plan.Eaten([]domain.Portion{soup}), day(4), day(6))
	tests := []struct {
		name   string
		got    domain.Day
		want   domain.PortionNutrition
		filled int
	}{
		{"no Plan", days[0], domain.PortionNutrition{}, 0},
		// Chili: 300 kcal, 25 g protein, at least 1 g fibre, 2 g sugar.
		{"one Portion", days[1], values(300, 25, g(1), g(2), true, false), 1},
		// Yogurt 90 kcal, 15 g protein, 0 g fibre, 6 g sugar; chili; soup, fibre unknown.
		{"three Portions", days[2], values(790, 60, g(1), g(13), true, false), 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.got.Nutrition
			got.Carbs, got.Fat = domain.Unknown, domain.Unknown // no Target aims at them
			if got != tt.want || tt.got.Filled != tt.filled {
				t.Errorf("day = %+v filled %d, want %+v filled %d", got, tt.got.Filled, tt.want, tt.filled)
			}
		})
	}
}

// A value misses its Target as shown, rounded to whole: calories outside the tolerance
// either side, protein and fibre under their floors, sugar over its ceiling. One known
// only to be at least so much misses only when that is already too much; an unknown one
// never does.
func TestAValueAgainstItsTarget(t *testing.T) {
	tests := []struct {
		name   string
		target domain.Target
		n      domain.PortionNutrition
		want   bool
	}{
		{"calories within the tolerance", domain.TargetKcal, values(2200, 0, nil, nil, false, false), false},
		{"calories over it", domain.TargetKcal, values(2201, 0, nil, nil, false, false), true},
		{"calories under it", domain.TargetKcal, values(1799, 0, nil, nil, false, false), true},
		{"calories rounded to the edge", domain.TargetKcal, values(1799.5, 0, nil, nil, false, false), false},
		{"protein at its floor", domain.TargetProtein, values(0, 100, nil, nil, false, false), false},
		{"protein under it", domain.TargetProtein, values(0, 99, nil, nil, false, false), true},
		{"fibre unknown", domain.TargetFibre, values(0, 0, nil, nil, false, false), false},
		{"fibre at least enough", domain.TargetFibre, values(0, 0, g(30), nil, true, false), false},
		{"fibre at least too little", domain.TargetFibre, values(0, 0, g(20), nil, true, false), false},
		{"fibre too little", domain.TargetFibre, values(0, 0, g(20), nil, false, false), true},
		{"sugar at its ceiling", domain.TargetSugar, values(0, 0, nil, g(50), false, false), false},
		{"sugar at least too much", domain.TargetSugar, values(0, 0, nil, g(51), false, true), true},
		{"sugar at least under it", domain.TargetSugar, values(0, 0, nil, g(40), false, true), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aim.Missed(tt.target, tt.n); got != tt.want {
				t.Errorf("Missed(%s) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

// The Targets' results over the planned days: the average against each Target, then
// each day more than 25% off one in its direction, in whole percent, a day planned only
// partly left out. None when no day is judged.
func TestTheTargetsResults(t *testing.T) {
	on := func(d int, n domain.PortionNutrition) domain.Day {
		return domain.Day{On: day(d), Nutrition: n, Filled: 4}
	}
	ok := values(2000, 100, g(30), g(50), false, false)
	tests := []struct {
		name string
		days []domain.Day
		want []domain.Miss
	}{
		{"no day judged", nil, nil},
		{"every Target met", []domain.Day{on(5, ok), on(6, ok)}, nil},
		{"protein averages over its floor", []domain.Day{on(5, values(2000, 96, g(30), g(50), false, false)), on(6, values(2000, 108, g(30), g(50), false, false))}, nil},
		{"protein averages under its floor", []domain.Day{on(5, values(2000, 90, g(30), g(50), false, false)), on(6, values(2000, 105, g(30), g(50), false, false))}, []domain.Miss{
			{Rule: domain.RuleTargetAverage, Target: domain.TargetProtein, Value: 98},
		}},
		{"a day 31% under calories", []domain.Day{on(5, values(1380, 100, g(30), g(50), false, false)), on(6, values(2500, 100, g(30), g(50), false, false))}, []domain.Miss{
			{Rule: domain.RuleDayOff, Target: domain.TargetKcal, On: day(5), Value: 31, Under: true},
		}},
		{"exactly 25% off is within the cap", []domain.Day{on(5, values(1500, 100, g(30), g(50), false, false)), on(6, values(2500, 100, g(30), g(50), false, false))}, nil},
		{"a day over calories", []domain.Day{on(5, values(2600, 100, g(30), g(50), false, false)), on(6, values(1800, 100, g(30), g(50), false, false))}, []domain.Miss{
			{Rule: domain.RuleDayOff, Target: domain.TargetKcal, On: day(5), Value: 30},
		}},
		{"a day planned only partly isn't judged", []domain.Day{{On: day(5), Nutrition: values(500, 20, g(5), g(5), false, false), Filled: 2, Partly: true}, on(6, ok)}, nil},
		{"protein and fibre are never over, sugar never under", []domain.Day{on(5, values(2000, 200, g(60), g(10), false, false))}, nil},
		{"fibre at least too little is unsure", []domain.Day{on(5, values(2000, 100, g(10), g(50), true, false))}, nil},
		{"fibre unknown on a day averages at least the rest", []domain.Day{on(5, values(2000, 100, nil, g(50), false, false)), on(6, ok)}, nil},
		{"sugar at least too much", []domain.Day{on(5, values(2000, 100, g(30), g(70), false, true))}, []domain.Miss{
			{Rule: domain.RuleTargetAverage, Target: domain.TargetSugar, Value: 70, Floor: true},
			{Rule: domain.RuleDayOff, Target: domain.TargetSugar, On: day(5), Value: 40, Floor: true},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aim.Misses(tt.days)
			if !slices.EqualFunc(got, tt.want, sameMiss) {
				t.Errorf("Misses() = %+v,\nwant %+v", got, tt.want)
			}
		})
	}
}

// sameMiss reports whether `a` and `b` are the same miss, their days compared as days.
func sameMiss(a, b domain.Miss) bool {
	return a.Rule == b.Rule && a.Target == b.Target && a.Value == b.Value && a.Floor == b.Floor && a.Under == b.Under &&
		a.On.Equal(b.On) && a.Name == b.Name && a.Location == b.Location && a.Count == b.Count && a.Held == b.Held &&
		slices.EqualFunc(a.Days, b.Days, time.Time.Equal)
}

// The soft rules of a Plan's structure: Portions of one Prep Session eaten within two
// days of each other, and Portions carried into the Cycle left without a Slot.
func TestTheSoftRulesOfAPlansStructure(t *testing.T) {
	tests := []struct {
		name string
		plan domain.Plan
		want []domain.Miss
	}{
		{"two days apart", domain.Plan{PrepSessions: []domain.PrepSession{chiliEaten(in(slot(2, domain.Dinner)), in(slot(4, domain.Lunch)))}}, nil},
		{"the next day", domain.Plan{PrepSessions: []domain.PrepSession{chiliEaten(in(slot(3, domain.Dinner)), in(slot(4, domain.Lunch)), in(slot(7, domain.Lunch)))}}, []domain.Miss{
			{Rule: domain.RuleBackToBack, Name: "Chili", Days: []time.Time{day(3), day(4)}},
		}},
		{"the same day", domain.Plan{PrepSessions: []domain.PrepSession{chiliEaten(in(slot(2, domain.Lunch)), in(slot(2, domain.Dinner)))}}, []domain.Miss{
			{Rule: domain.RuleBackToBack, Name: "Chili", Days: []time.Time{day(2)}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.SoftMisses(nil, domain.Plan{}); !slices.EqualFunc(got, tt.want, sameMiss) {
				t.Errorf("SoftMisses() = %+v,\nwant %+v", got, tt.want)
			}
		})
	}
}

// Of the Portions left without a Slot, only those carried into the Cycle are a miss,
// told with how many of them the kitchen holds; those the Plan makes are made for after
// the Cycle.
func TestPortionsLeftWithoutASlot(t *testing.T) {
	lasagne := []domain.PortionLot{
		{Name: "lasagne", Location: domain.Freezer, IDs: []int64{1, 2, 3}},
		{Name: "lasagne", Location: domain.Fridge, IDs: []int64{4}},
	}
	rice := domain.PrepSession{At: slot(2, domain.Dinner), Recipe: &domain.Recipe{Name: "Rice bowl"}, Size: 1,
		Portions: []domain.PlannedPortion{{Location: domain.Freezer}}}
	tests := []struct {
		name    string
		plan    domain.Plan
		misses  []domain.Miss
		madeFor []domain.MadeForAfter
	}{
		{"carried in and left unused", domain.Plan{Kept: []domain.Kept{{Portion: 2, At: slot(2, domain.Lunch)}, {Portion: 4, At: slot(3, domain.Lunch)}}}, []domain.Miss{
			{Rule: domain.RuleUnusedPortion, Name: "lasagne", Location: domain.Freezer, Count: 2, Held: 3},
		}, nil},
		{"carried in and used", domain.Plan{Kept: []domain.Kept{{Portion: 1}, {Portion: 2}, {Portion: 3}, {Portion: 4}}}, nil, nil},
		{"made by the Plan with no Slot", domain.Plan{
			Kept:         []domain.Kept{{Portion: 1}, {Portion: 2}, {Portion: 3}, {Portion: 4}},
			PrepSessions: []domain.PrepSession{chiliEaten(in(slot(2, domain.Dinner)), nil, nil), rice},
		}, nil, []domain.MadeForAfter{
			{Name: "Chili", Location: domain.Fridge, Portions: 2},
			{Name: "Rice bowl", Location: domain.Freezer, Portions: 1},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.SoftMisses(lasagne, domain.Plan{}); !slices.EqualFunc(got, tt.misses, sameMiss) {
				t.Errorf("SoftMisses() = %+v,\nwant %+v", got, tt.misses)
			}
			if got := tt.plan.MadeForAfter(); !slices.Equal(got, tt.madeFor) {
				t.Errorf("MadeForAfter() = %+v, want %+v", got, tt.madeFor)
			}
		})
	}
}

// chiliEaten is a Prep Session of chili at Tuesday's dinner, a Portion eaten at each of
// `at`, nil kept for later.
func chiliEaten(at ...*domain.Slot) domain.PrepSession {
	ps := domain.PrepSession{At: slot(2, domain.Dinner), Recipe: &chili, Size: 1}
	for _, s := range at {
		ps.Portions = append(ps.Portions, eaten(s))
	}
	return ps
}

// A Plan's days are shown beside the seven before them, as earlier Plans fill them; a
// first day it fills only partly shows what earlier Plans put in its other Slots, and is
// marked so; what earlier Plans put from its first Slot on is the Plan's to replace.
func TestAPlansDaysBesideTheSevenBefore(t *testing.T) {
	n := values(500, 30, g(5), g(5), false, false)
	earlier := []domain.Eaten{
		{At: slot(1, domain.Dinner), Nutrition: n},
		{At: slot(6, domain.Breakfast), Nutrition: n},
		{At: slot(6, domain.Lunch), Nutrition: n},
	}
	planned := []domain.Eaten{{At: slot(6, domain.Lunch), Nutrition: n}, {At: slot(7, domain.Lunch), Nutrition: n}}
	tests := []struct {
		name             string
		first            domain.Slot
		trailing, filled []int
		partly           bool
	}{
		{"from breakfast", slot(6, domain.Breakfast), []int{0, 0, 1, 0, 0, 0, 0}, []int{1, 1}, false},
		{"from lunch", slot(6, domain.Lunch), []int{0, 0, 1, 0, 0, 0, 0}, []int{2, 1}, true},
		{"the next day", slot(8, domain.Breakfast), []int{1, 0, 0, 0, 0, 3, 1}, []int{0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			last := tt.first.On.AddDate(0, 0, len(tt.filled)-1)
			trailing, days := domain.PlanDays(earlier, planned, tt.first, last)
			filled := func(days []domain.Day) []int {
				var out []int
				for _, d := range days {
					out = append(out, d.Filled)
				}
				return out
			}
			if got := filled(trailing); !slices.Equal(got, tt.trailing) || !trailing[0].On.Equal(tt.first.On.AddDate(0, 0, -7)) {
				t.Errorf("trailing filled %v from %v, want %v", got, trailing[0].On, tt.trailing)
			}
			if got := filled(days); !slices.Equal(got, tt.filled) {
				t.Errorf("days filled %v, want %v", got, tt.filled)
			}
			if days[0].Partly != tt.partly || slices.ContainsFunc(days[1:], func(d domain.Day) bool { return d.Partly }) {
				t.Errorf("first day partly %v, want %v; later days never", days[0].Partly, tt.partly)
			}
		})
	}
}
