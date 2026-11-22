package domain_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// chili is a Recipe written for 4 Portions: 400 g of mince and 2 onions, 300 kcal and
// 25 g protein a Portion, its fibre unknown in the mince.
var chili = domain.Recipe{ID: 7, Name: "Chili", Portions: 4, Ingredients: []domain.RecipeIngredient{
	{Ingredient: domain.Ingredient{Name: "beef mince", Unit: domain.Grams, Nutrition: nutrition(250, 20, 0, 18, domain.Unknown, domain.Known(0))}, Quantity: grams(400)},
	{Ingredient: domain.Ingredient{Name: "onion", Unit: domain.Pieces, Nutrition: nutrition(100, 10, 10, 2, domain.Known(3), domain.Known(4))}, Quantity: grams(2)},
}}

// yogurt is an Ingredient set out as it is.
var yogurt = domain.Ingredient{ID: 3, Name: "yogurt", Unit: domain.Grams, Nutrition: nutrition(60, 10, 4, 0, domain.Known(0), domain.Known(4))}

// eaten is a Portion kept in the fridge, eaten at `s`, or without a Slot when `s` is nil.
func eaten(s *domain.Slot) domain.PlannedPortion {
	return domain.PlannedPortion{Location: domain.Fridge, EatAt: s}
}

// in is `s`, as a Portion's Slot.
func in(s domain.Slot) *domain.Slot { return &s }

// A Prep Session of a Recipe scales each Ingredient by the Portions it yields over the
// Portions as written, times its size; one of an Ingredient sets out its quantity for
// each Portion. What is counted in pieces is rounded to whole ones, never to none.
func TestAPrepSessionScalesItsIngredients(t *testing.T) {
	tests := []struct {
		name string
		ps   domain.PrepSession
		want []float64
	}{
		{"as written", domain.PrepSession{Recipe: &chili, Size: 1, Portions: make([]domain.PlannedPortion, 4)}, []float64{400, 2}},
		{"3 Portions at 1.25×", domain.PrepSession{Recipe: &chili, Size: 1.25, Portions: make([]domain.PlannedPortion, 3)}, []float64{375, 2}},
		{"6 Portions at 0.75×", domain.PrepSession{Recipe: &chili, Size: 0.75, Portions: make([]domain.PlannedPortion, 6)}, []float64{450, 2}},
		{"7 Portions at 1.5×", domain.PrepSession{Recipe: &chili, Size: 1.5, Portions: make([]domain.PlannedPortion, 7)}, []float64{1050, 5}},
		{"pieces never round to none", domain.PrepSession{Recipe: &chili, Size: 0.75, Portions: make([]domain.PlannedPortion, 1)}, []float64{75, 1}},
		{"an Ingredient", domain.PrepSession{Ingredient: &yogurt, Quantity: grams(150), Portions: make([]domain.PlannedPortion, 3)}, []float64{450}},
		{"an Ingredient in pieces", domain.PrepSession{Ingredient: &egg, Quantity: grams(0.5), Portions: make([]domain.PlannedPortion, 3)}, []float64{2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []float64
			for _, ri := range tt.ps.Ingredients() {
				got = append(got, ri.Quantity.Number())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Ingredients() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A Portion's nutritional values scale with its size, a floor staying a floor; one of an
// Ingredient is that Ingredient's values at its quantity.
func TestAPortionsNutritionScalesWithItsSize(t *testing.T) {
	tests := []struct {
		name          string
		ps            domain.PrepSession
		kcal, protein float64
		fibre         domain.Amount
		fibreFloor    bool
		sugar         domain.Amount
	}{
		// As written: 1200 kcal, 100 g protein, 6 g fibre at least, 8 g sugar over 4.
		{"as written", domain.PrepSession{Recipe: &chili, Size: 1}, 300, 25, domain.Known(1), true, domain.Known(2)},
		{"at 1.5×", domain.PrepSession{Recipe: &chili, Size: 1.5}, 450, 38, domain.Known(2), true, domain.Known(3)},
		{"at 0.75×", domain.PrepSession{Recipe: &chili, Size: 0.75}, 225, 19, domain.Known(1), true, domain.Known(2)},
		{"an Ingredient", domain.PrepSession{Ingredient: &yogurt, Quantity: grams(150)}, 90, 15, domain.Known(0), false, domain.Known(6)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := tt.ps.NutritionPerPortion()
			if n.Kcal != domain.Known(tt.kcal) || n.Protein != domain.Known(tt.protein) {
				t.Errorf("kcal, protein = %v, %v; want %v, %v", n.Kcal, n.Protein, tt.kcal, tt.protein)
			}
			if n.Fibre != tt.fibre || n.FibreFloor != tt.fibreFloor || n.Sugar != tt.sugar {
				t.Errorf("fibre %v (floor %v), sugar %v; want %v (floor %v), %v", n.Fibre, n.FibreFloor, n.Sugar, tt.fibre, tt.fibreFloor, tt.sugar)
			}
		})
	}
}

// A draft Plan that can't be read is refused before any rule: a size outside 0.75–1.5, a
// Prep Session yielding no Portion, a Portion without a Slot put in two.
func TestADraftPlanThatCantBeRead(t *testing.T) {
	session := func(size float64, portions int) domain.PrepSession {
		return domain.PrepSession{At: slot(5, domain.Dinner), Recipe: &chili, Size: size, Portions: make([]domain.PlannedPortion, portions)}
	}
	tests := []struct {
		name string
		plan domain.Plan
		want int
	}{
		{"readable", domain.Plan{PrepSessions: []domain.PrepSession{session(0.75, 1), session(1.5, 3)}}, 0},
		{"too small", domain.Plan{PrepSessions: []domain.PrepSession{session(0.7, 3)}}, 1},
		{"too large", domain.Plan{PrepSessions: []domain.PrepSession{session(1.6, 3)}}, 1},
		{"no Portion", domain.Plan{PrepSessions: []domain.PrepSession{session(1, 0)}}, 1},
		{"an Ingredient has no size", domain.Plan{PrepSessions: []domain.PrepSession{{At: slot(5, domain.Lunch), Ingredient: &yogurt, Quantity: grams(150), Portions: make([]domain.PlannedPortion, 1)}}}, 0},
		{"a Portion put in two Slots", domain.Plan{Kept: []domain.Kept{{Portion: 9, At: slot(5, domain.Lunch)}, {Portion: 9, At: slot(6, domain.Lunch)}}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.Unreadable(); len(got) != tt.want {
				t.Errorf("Unreadable() = %q, want %d", got, tt.want)
			}
		})
	}
}

// The structural hard rules: a Slot filled twice or left empty, a Portion eaten before
// it is prepared or that doesn't exist, food outside the Cycle.
func TestTheStructuralHardRules(t *testing.T) {
	// The Plan fills Thursday 5 November's dinner to Friday 6's lunch.
	slots := []domain.Slot{slot(5, domain.Dinner), slot(6, domain.Breakfast), slot(6, domain.Lunch)}
	soup := domain.Portion{ID: 9, Name: "Soup"}
	full := func() domain.Plan {
		return domain.Plan{
			PrepSessions: []domain.PrepSession{{At: slot(5, domain.Dinner), Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{
				eaten(in(slot(5, domain.Dinner))), eaten(in(slot(6, domain.Lunch))), eaten(nil),
			}}},
			Kept: []domain.Kept{{Portion: 9, At: slot(6, domain.Breakfast)}},
		}
	}
	tests := []struct {
		name   string
		change func(*domain.Plan)
		want   []domain.Rule
	}{
		{"every Slot filled once", func(*domain.Plan) {}, nil},
		{"a Slot filled twice", func(p *domain.Plan) { p.Kept[0].At = slot(6, domain.Lunch) }, []domain.Rule{domain.RuleSlotFilledTwice, domain.RuleEmptySlot}},
		{"a Slot left empty", func(p *domain.Plan) { p.Kept = nil }, []domain.Rule{domain.RuleEmptySlot}},
		{"a Portion eaten before it is prepared", func(p *domain.Plan) { p.PrepSessions[0].At = slot(6, domain.Breakfast) }, []domain.Rule{domain.RuleEatenBeforePrepared}},
		{"a Portion that doesn't exist", func(p *domain.Plan) { p.Kept[0].Portion = 10 }, []domain.Rule{domain.RuleUnknownPortion, domain.RuleEmptySlot}},
		{"a Prep Session outside the Cycle", func(p *domain.Plan) { p.PrepSessions[0].At = slot(5, domain.Lunch) }, []domain.Rule{domain.RuleOutsideCycle}},
		{"a Portion eaten after the Cycle", func(p *domain.Plan) {
			p.PrepSessions[0].Portions[2].EatAt = in(slot(6, domain.Dinner))
		}, []domain.Rule{domain.RuleOutsideCycle}},
		{"a kept Portion put before the Cycle", func(p *domain.Plan) { p.Kept[0].At = slot(5, domain.Snack) }, []domain.Rule{domain.RuleOutsideCycle, domain.RuleEmptySlot}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := full()
			tt.change(&p)
			var got []domain.Rule
			for _, v := range domain.HardRules(p, slots, []domain.Portion{soup}) {
				if v.Message == "" {
					t.Errorf("%s has no message", v.Rule)
				}
				got = append(got, v.Rule)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("HardRules() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Two Slots are the same Slot when they are the same kind on the same day, whatever zone
// each day is in.
func TestSlotsAreEqualByDayAndKind(t *testing.T) {
	inUTC := domain.Slot{On: time.Date(2026, time.November, 6, 0, 0, 0, 0, time.UTC), Kind: domain.Lunch}
	tests := []struct {
		name string
		a, b domain.Slot
		want bool
	}{
		{"the same", slot(6, domain.Lunch), slot(6, domain.Lunch), true},
		{"in another zone", slot(6, domain.Lunch), inUTC, true},
		{"another kind", slot(6, domain.Lunch), slot(6, domain.Dinner), false},
		{"another day", slot(6, domain.Lunch), slot(7, domain.Lunch), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Only starting in the past and filling a Slot with an Exception stop the User approving
// a Plan.
func TestWhichRulesBlockApproval(t *testing.T) {
	tests := []struct {
		rule domain.Rule
		want bool
	}{
		{domain.RuleSlotFilledTwice, false},
		{domain.RuleEmptySlot, false},
		{domain.RuleEatenBeforePrepared, false},
		{domain.RuleUnknownPortion, false},
		{domain.RuleOutsideCycle, false},
		{domain.RuleStartsInThePast, true},
		{domain.RuleSlotWithException, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.rule), func(t *testing.T) {
			if got := tt.rule.BlocksApproval(); got != tt.want {
				t.Errorf("BlocksApproval() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A Plan starts in the past once its first Slot's time of day is reached.
func TestAPlanStartsInThePastOnceItsFirstSlotsTimeIsReached(t *testing.T) {
	first := slot(6, domain.Breakfast) // at 8:00
	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"the evening before", time.Date(2026, time.November, 5, 21, 0, 0, 0, rome), false},
		{"a minute before", time.Date(2026, time.November, 6, 7, 59, 0, 0, rome), false},
		{"at its time", time.Date(2026, time.November, 6, 8, 0, 0, 0, rome), true},
		{"later that day", time.Date(2026, time.November, 6, 13, 0, 0, 0, rome), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := domain.StartsInThePast(first, domain.DefaultSlotTimes, tt.now)
			if got := len(vs) == 1 && vs[0].Rule == domain.RuleStartsInThePast; got != tt.want {
				t.Errorf("StartsInThePast() = %v, want it broken %v", vs, tt.want)
			}
		})
	}
}

// A Plan as far as the next Cycle's Delivery arriving early: its Prep Sessions after the
// arrival go, its Portions eaten after it carry over without a Slot, and the Portions
// without a Slot it put in Slots after it stay without one. What comes before stays.
func TestAPlanUntilTheNextDeliveryArrives(t *testing.T) {
	until := cycle(3, "14:00", "16:00", 9).Delivery.Arrives()
	plan := domain.Plan{
		PrepSessions: []domain.PrepSession{
			{At: slot(3, domain.Lunch), Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{eaten(in(slot(3, domain.Lunch))), eaten(in(slot(3, domain.Dinner))), eaten(nil)}},
			{At: slot(3, domain.Snack), Ingredient: &yogurt, Quantity: grams(150), Portions: []domain.PlannedPortion{eaten(in(slot(3, domain.Snack)))}},
		},
		Kept: []domain.Kept{{Portion: 1, At: slot(3, domain.Breakfast)}, {Portion: 2, At: slot(4, domain.Lunch)}},
	}
	got := plan.Until(until, domain.DefaultSlotTimes)
	want := domain.Plan{
		PrepSessions: []domain.PrepSession{
			{At: slot(3, domain.Lunch), Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{eaten(in(slot(3, domain.Lunch))), eaten(nil), eaten(nil)}},
		},
		Kept: []domain.Kept{{Portion: 1, At: slot(3, domain.Breakfast)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if plan.PrepSessions[0].Portions[1].EatAt == nil {
		t.Error("cutting a Plan changed the Plan cut")
	}
}
