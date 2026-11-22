package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// The Friday the planner judges: a Cycle whose Delivery comes on Thursday 5 November
// between 6 and 8 pm and that ends on Friday 6, so its Slots are Friday's four; rice at
// 350 kcal and 7 g protein a 100 g, its fibre and sugar unknown; the Rice bowl, two
// Portions of 200 g rice as written, for lunch and dinner on a hob and a microwave;
// yogurt at 60 kcal and 10 g protein a 100 g; and three Portions of lasagne in the
// freezer (ids 1–3, 620 kcal, 32 g protein, 4 g fibre, 8 g sugar) and one of soup in the
// fridge (id 4), none with a Slot.
var (
	fridayCycle = domain.Cycle{Delivery: domain.Delivery{On: day(5), From: 18 * 60, To: 20 * 60}, Ends: day(6)}
	plainRice   = domain.Ingredient{ID: 1, Name: "rice", Unit: domain.Grams, Usual: domain.Pantry, Nutrition: nutrition(350, 7, 78, 1, domain.Unknown, domain.Unknown)}
	plainYogurt = domain.Ingredient{ID: 2, Name: "yogurt", Unit: domain.Grams, Usual: domain.Fridge, Nutrition: nutrition(60, 10, 4, 0, domain.Unknown, domain.Unknown)}
	riceBowl    = domain.Recipe{ID: 1, Name: "Rice bowl", Portions: 2, SlotKinds: []domain.SlotKind{domain.Lunch, domain.Dinner},
		Appliances: []domain.ApplianceKind{domain.Hob, domain.Microwave}, Ingredients: []domain.RecipeIngredient{{Ingredient: plainRice, Quantity: grams(200)}}}
	kitchenLots = []domain.PortionLot{
		{Name: "lasagne", Location: domain.Freezer, Nutrition: nutrition(620, 32, 60, 25, domain.Known(4), domain.Known(8)), IDs: []int64{1, 2, 3}},
		{Name: "soup", Location: domain.Fridge, Nutrition: nutrition(200, 10, 20, 5, domain.Known(3), domain.Known(2)), IDs: []int64{4}},
	}
)

// judging is what Judge is given, all but the Cycle Friday's.
type judging struct {
	plan    domain.Plan
	from    domain.Slot
	profile domain.Profile
	current domain.Plan
	stock   map[domain.StockKey]domain.Quantity
	earlier []domain.Eaten
	now     time.Time
}

// friday is a draft Plan of Friday keeping every hard rule, judged from its breakfast at
// 9 pm on Thursday: yogurt at breakfast, a Rice bowl prepared at lunch for three Portions
// at 1.25×, eaten at lunch and dinner with the third frozen with no Slot, and a Portion
// of lasagne at the snack. The kitchen holds 800 g of rice in the pantry and 500 g of
// yogurt in the fridge. The User owns a hob and a microwave, and aims at 2200 kcal
// within 8%, at least 130 g protein and 30 g fibre, at most 50 g sugar. Friday eats 1586
// kcal and 65 g protein: 90 and 15 of yogurt, 438 and 9 of each Rice bowl, 620 and 32
// of lasagne.
func friday() judging {
	return judging{
		plan: domain.Plan{
			PrepSessions: []domain.PrepSession{
				{At: slot(6, domain.Breakfast), Ingredient: &plainYogurt, Quantity: grams(150), Portions: []domain.PlannedPortion{
					{Location: domain.Fridge, EatAt: in(slot(6, domain.Breakfast))},
				}},
				{At: slot(6, domain.Lunch), Recipe: &riceBowl, Size: 1.25, Portions: []domain.PlannedPortion{
					{Location: domain.Fridge, EatAt: in(slot(6, domain.Lunch))},
					{Location: domain.Fridge, EatAt: in(slot(6, domain.Dinner))},
					{Location: domain.Freezer},
				}},
			},
			Kept: []domain.Kept{{Portion: 1, At: slot(6, domain.Snack)}},
		},
		from: slot(6, domain.Breakfast),
		profile: domain.Profile{
			SlotTimes:  domain.DefaultSlotTimes,
			Targets:    domain.Targets{Kcal: 2200, Protein: 130, Fibre: 30, Sugar: 50, Tolerance: 8},
			Appliances: domain.Appliances{domain.Hob: "", domain.Microwave: ""},
		},
		stock: held(800, 500),
		now:   at(5, "21:00"),
	}
}

func (j judging) judge() domain.Verdict {
	return domain.Judge(j.plan, fridayCycle, j.from, j.profile, j.current, kitchenLots, j.stock, j.earlier, j.now)
}

// held is what the kitchen holds of rice in the pantry and of yogurt in the fridge.
func held(rice, yogurt float64) map[domain.StockKey]domain.Quantity {
	return map[domain.StockKey]domain.Quantity{
		{Ingredient: plainRice.ID, Location: domain.Pantry}:   grams(rice),
		{Ingredient: plainYogurt.ID, Location: domain.Fridge}: grams(yogurt),
	}
}

// Judge gives every hard rule a draft breaks, as the planner reads it: of its structure,
// of its Recipes, of frozen food with no evening left to thaw it, and a first Slot
// already passed. It knows which Portions without a Slot the kitchen holds, and which are
// frozen, from its Portion Lots.
func TestJudgeGivesTheHardRulesADraftBreaks(t *testing.T) {
	tests := []struct {
		name   string
		change func(*judging)
		want   []string
	}{
		{"every hard rule kept", func(*judging) {}, nil},
		{"breaking every structural hard rule", func(j *judging) {
			// The Rice bowl prepared at dinner is eaten at lunch, before it, and on
			// Saturday, after the Cycle; lasagne fills breakfast a second time; and
			// Portion 999 isn't in the kitchen, leaving the snack empty.
			bowl := &j.plan.PrepSessions[1]
			bowl.At = slot(6, domain.Dinner)
			bowl.Portions[2].EatAt = in(slot(7, domain.Lunch))
			j.plan.Kept = []domain.Kept{{Portion: 1, At: slot(6, domain.Breakfast)}, {Portion: 999, At: slot(6, domain.Snack)}}
		}, []string{
			"eaten-before-prepared: Rice bowl is eaten at lunch on Fri 6 Nov, before it is prepared at dinner on Fri 6 Nov",
			"outside-cycle: Rice bowl is eaten at lunch on Sat 7 Nov, outside the Slots planned",
			"slot-filled-twice: breakfast on Fri 6 Nov is filled twice: by yogurt and by Portion 1 (lasagne)",
			"unknown-portion: Portion 999 put in snack on Fri 6 Nov is not a Portion without a Slot",
			"empty-slot: snack on Fri 6 Nov is empty: no Portion fills it",
		}},
		{"a Recipe at a kind of Slot it isn't for", func(j *judging) {
			j.plan.PrepSessions[0].Portions[0].EatAt = in(slot(6, domain.Lunch))
			j.plan.PrepSessions[1].At = slot(6, domain.Breakfast)
			j.plan.PrepSessions[1].Portions[0].EatAt = in(slot(6, domain.Breakfast))
		}, []string{"wrong-kind-of-slot: Rice bowl is eaten at breakfast on Fri 6 Nov, a kind of Slot it isn't for: lunch, dinner"}},
		{"a Recipe needing an appliance not owned", func(j *judging) {
			j.profile.Appliances = domain.Appliances{domain.Hob: "", domain.Oven: ""}
		}, []string{"appliance-not-owned: Rice bowl at lunch on Fri 6 Nov needs the appliance microwave, which the user doesn't own"}},
		{"a frozen Portion put in a Slot today", func(j *judging) { j.now = at(6, "07:00") }, []string{
			"no-evening-to-thaw: Portion 1 is frozen, and the evening before snack on Fri 6 Nov to thaw it has passed",
		}},
		{"a Portion in the fridge put in a Slot today", func(j *judging) {
			j.now = at(6, "07:00")
			j.plan.Kept[0].Portion = 4
		}, nil},
		{"freezer stock needed today", func(j *judging) {
			j.now = at(6, "07:00")
			j.plan.Kept[0].Portion = 4
			j.stock = held(0, 500)
			j.stock[domain.StockKey{Ingredient: plainRice.ID, Location: domain.Freezer}] = grams(1000)
		}, []string{"no-evening-to-thaw: lunch on Fri 6 Nov needs 375 g rice thawed from the freezer, and the evening before to thaw it has passed"}},
		{"a Portion frozen and eaten the day it is prepared", func(j *judging) {
			j.plan.PrepSessions[1].Portions[1].Location = domain.Freezer
		}, []string{"no-evening-to-thaw: Rice bowl is frozen at lunch on Fri 6 Nov and eaten at dinner on Fri 6 Nov, with no evening between to thaw it"}},
		{"on the current Cycle, more of an Ingredient than the kitchen will hold", func(j *judging) {
			j.stock = held(200, 100)
		}, []string{
			"more-than-held: yogurt at breakfast on Fri 6 Nov needs 150 g yogurt, and the kitchen will hold only 100 g",
			"more-than-held: Rice bowl at lunch on Fri 6 Nov needs 375 g rice, and the kitchen will hold only 200 g",
		}},
		{"each Prep Session after those before it have used theirs up", func(j *judging) {
			j.stock = held(500, 500)
			j.plan.PrepSessions = append(j.plan.PrepSessions, domain.PrepSession{At: slot(6, domain.Dinner), Recipe: &riceBowl, Size: 1, Portions: []domain.PlannedPortion{{Location: domain.Freezer}, {Location: domain.Freezer}}})
		}, []string{"more-than-held: Rice bowl at dinner on Fri 6 Nov needs 200 g rice, and the kitchen will hold only 125 g"}},
		{"a Staple, which the kitchen always holds", func(j *judging) {
			j.stock = held(0, 500)
			staple := plainRice
			staple.Staple = true
			bowl := riceBowl
			bowl.Ingredients = []domain.RecipeIngredient{{Ingredient: staple, Quantity: grams(200)}}
			j.plan.PrepSessions[1].Recipe = &bowl
		}, nil},
		{"on the current Cycle, a freezer split", func(j *judging) {
			j.plan.FreezerSplits = []domain.RecipeIngredient{{Ingredient: plainYogurt, Quantity: grams(200)}}
		}, []string{"split-after-put-away: 200 g yogurt is split to the freezer, and the Delivery is already put away"}},
		{"on the upcoming Cycle, whose Delivery brings the rest and is yet to be put away", func(j *judging) {
			j.now = at(5, "19:00")
			j.stock = nil
			j.plan.FreezerSplits = []domain.RecipeIngredient{{Ingredient: plainYogurt, Quantity: grams(200)}}
		}, nil},
		{"a first Slot already passed", func(j *judging) {
			j.now = at(6, "08:00")
			j.plan.Kept[0].Portion = 4
		}, []string{"starts-in-the-past: its first Slot, breakfast on Fri 6 Nov, has passed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := friday()
			tt.change(&j)
			var got []string
			for _, v := range j.judge().Broken {
				got = append(got, v.String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Judge() broke\n%q,\nwant\n%q", got, tt.want)
			}
		})
	}
}

// Judge gives the days a draft plans beside the seven before, as approved Plans fill
// them: what they put from its first Slot on is the draft's to replace, and a first day
// it plans only from lunch on is partly planned, eating what they put at its breakfast.
func TestJudgeGivesADraftsDaysAndTheSevenBefore(t *testing.T) {
	n := values(500, 30, g(5), g(5), false, false)
	earlier := []domain.Eaten{{At: slot(5, domain.Dinner), Nutrition: n}, {At: slot(6, domain.Breakfast), Nutrition: n}}
	tests := []struct {
		name     string
		change   func(*judging)
		trailing []int
		filled   int
		kcal     float64
		partly   bool
	}{
		{"from breakfast", func(*judging) {}, []int{0, 0, 0, 0, 0, 0, 1}, 4, 1586, false},
		{"from lunch", func(j *judging) {
			j.from = slot(6, domain.Lunch)
			j.plan.PrepSessions = j.plan.PrepSessions[1:]
		}, []int{0, 0, 0, 0, 0, 0, 1}, 4, 1996, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := friday()
			j.earlier = earlier
			tt.change(&j)
			v := j.judge()
			var trailing []int
			for _, d := range v.Trailing {
				trailing = append(trailing, d.Filled)
			}
			if !slices.Equal(trailing, tt.trailing) || !v.Trailing[0].On.Equal(time.Date(2026, time.October, 30, 0, 0, 0, 0, rome)) {
				t.Errorf("the seven days before filled %v from %v, want %v from Fri 30 Oct", trailing, v.Trailing[0].On, tt.trailing)
			}
			if len(v.Days) != 1 || !v.Days[0].On.Equal(day(6)) {
				t.Fatalf("Judge() planned %+v, want Friday alone", v.Days)
			}
			if d := v.Days[0]; d.Filled != tt.filled || d.Nutrition.Kcal.Value != tt.kcal || d.Partly != tt.partly {
				t.Errorf("Friday filled %d with %v kcal, partly %v; want %d with %v, partly %v", d.Filled, d.Nutrition.Kcal.Value, d.Partly, tt.filled, tt.kcal, tt.partly)
			}
		})
	}
}

// Judge gives the Targets a draft misses, on average and by day, then the soft rules
// it breaks; a day only a little under a Target misses it only on average, and a first
// day partly planned isn't judged against the Targets.
func TestJudgeGivesTheTargetsAndSoftRulesADraftMisses(t *testing.T) {
	unused := []string{"Rice bowl eaten back to back, Fri 6 Nov", "lasagne: 2 of 3 left unused, in the freezer", "soup: 1 of 1 left unused, in the fridge"}
	tests := []struct {
		name   string
		change func(*judging)
		want   []string
	}{
		{"under calories and protein", func(*judging) {}, append([]string{
			"Calories average 1586 a day against 2024–2376",
			"Protein averages 65 g a day against 130 g",
			"Fri 6 Nov: 28% under kcal",
			"Fri 6 Nov: 50% under protein",
		}, unused...)},
		{"a little under protein", func(j *judging) {
			j.profile.Targets = domain.Targets{Kcal: 1600, Protein: 70, Fibre: 30, Sugar: 50, Tolerance: 5}
		}, append([]string{"Protein averages 65 g a day against 70 g"}, unused...)},
		{"a first day partly planned", func(j *judging) {
			j.from = slot(6, domain.Lunch)
			j.plan.PrepSessions = j.plan.PrepSessions[1:]
		}, unused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := friday()
			tt.change(&j)
			var got []string
			for _, m := range j.judge().Misses {
				got = append(got, m.Describe(j.profile.Targets))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Judge() missed\n%q,\nwant\n%q", got, tt.want)
			}
		})
	}
}

// Judge gives the Portions a draft makes with no Slot, made for after the Cycle, as the
// kitchen will hold them: how many of each name, ignoring case and surrounding space, in
// each Storage Location, in the order the draft first makes them.
func TestJudgeGivesThePortionsMadeForAfterTheCycle(t *testing.T) {
	j := friday()
	bowl := &j.plan.PrepSessions[1]
	bowl.Portions = append(bowl.Portions, domain.PlannedPortion{Location: domain.Fridge}, domain.PlannedPortion{Location: domain.Freezer})
	chili := domain.Recipe{Name: "Chili"}
	j.plan.PrepSessions = append(j.plan.PrepSessions,
		domain.PrepSession{At: slot(6, domain.Dinner), Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{{Location: domain.Fridge}}},
		domain.PrepSession{At: slot(6, domain.Dinner), Recipe: &domain.Recipe{Name: " rice BOWL"}, Size: 1, Portions: []domain.PlannedPortion{{Location: domain.Freezer}}},
	)
	want := []domain.MadeForAfter{
		{Name: "Rice bowl", Location: domain.Freezer, Portions: 3},
		{Name: "Rice bowl", Location: domain.Fridge, Portions: 1},
		{Name: "Chili", Location: domain.Fridge, Portions: 1},
	}
	if got := j.judge().MadeForAfter; !slices.Equal(got, want) {
		t.Errorf("Judge() made for after the Cycle %+v, want %+v", got, want)
	}
}

// A Prep Session the Plan keeps before the draft's first Slot counts towards back to back
// with the Portions of it the draft puts in Slots again, when the draft puts one close to
// another; and one it leaves without a Slot is made for after the Cycle, no miss, as are
// those of the draft's own Prep Sessions. Here the Plan set out yogurt
// at Friday's breakfast for three Portions, eaten at breakfast and lunch, the third
// without a Slot, and the draft plans Friday from its snack.
func TestJudgeCountsTheKeptPrepSessionsOfAReplan(t *testing.T) {
	current := domain.Plan{PrepSessions: []domain.PrepSession{{At: slot(6, domain.Breakfast), Ingredient: &plainYogurt, Quantity: grams(150), Portions: []domain.PlannedPortion{
		{ID: 31, Location: domain.Fridge, EatAt: in(slot(6, domain.Breakfast))},
		{ID: 32, Location: domain.Fridge, EatAt: in(slot(6, domain.Lunch))},
		{ID: 33, Location: domain.Fridge},
	}}}}
	tests := []struct {
		name string
		kept []domain.Kept
		want []string
	}{
		{"the draft eats the third the same day", []domain.Kept{{Portion: 4, At: slot(6, domain.Snack)}, {Portion: domain.PlannedID(33), At: slot(6, domain.Dinner)}},
			[]string{"yogurt eaten back to back, Fri 6 Nov", "lasagne: 3 of 3 left unused, in the freezer"}},
		{"the draft leaves it", []domain.Kept{{Portion: 4, At: slot(6, domain.Snack)}, {Portion: 1, At: slot(6, domain.Dinner)}},
			[]string{"lasagne: 2 of 3 left unused, in the freezer"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := friday()
			j.from, j.now, j.current = slot(6, domain.Snack), at(6, "16:00"), current
			j.plan = domain.Plan{Kept: tt.kept}
			var got []string
			for _, m := range j.judge().Misses {
				got = append(got, m.Describe(j.profile.Targets))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Judge() missed\n%q,\nwant\n%q", got, tt.want)
			}
		})
	}
}
