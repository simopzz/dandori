package app_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/domain"
)

// approvesFriday plans friday as plansFriday does and approves it: yogurt set out at
// breakfast, a Rice bowl prepared at lunch for three Portions at 1.25×, eaten at lunch
// and dinner with the third frozen, and a Portion of lasagne at the snack. Its Delivery,
// on Thursday evening, brings the yogurt.
func approvesFriday(t *testing.T, s *apptest.Scenario) {
	t.Helper()
	plansFriday(t, s, "Rice bowls for Friday.")
	s.Open("/plan")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
}

// assumed is how many Movements of `cause` the kitchen of `s` holds, for a check before
// any page has brought it up to now.
func assumed(t *testing.T, s *apptest.Scenario, cause domain.Cause) int {
	t.Helper()
	var n int
	s.Scan(t, &n, `SELECT count(*) FROM movement WHERE cause_kind = $1`, string(cause))
	return n
}

// At a Slot's time, its Prep Session uses up its stock and prepares its Portions where
// the Plan keeps them, and the Portion filling it is eaten: the Kitchen page shows it,
// once opened, and each Stock Item's Movements say why. The Delivery the night before
// is put away at its arrival, and breakfast uses what it brought.
func TestOpeningAPageBringsThePlanUpToNow(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Open("/profile")
	s.SavesTimesOfDay(apptest.TimesOfDay{Breakfast: "8:00", Lunch: "13:02", Snack: "17:00", Dinner: "20:00"})
	approvesFriday(t, s)

	s.At("Fri 13:03") // the beat, at 13:00, came before lunch
	s.Open("/kitchen")
	s.Sees("rice 425 g")
	s.Sees("02 Fridge Rice bowl × 1") // the lunch's eaten, the dinner's left
	s.Sees("Count Rice bowl in the freezer")
	s.Follows("rice")
	s.Sees("Used up 800 g → 425 g Fri, Nov 6 · 1:02 pm")

	s.Open("/kitchen/movements?ingredient=2&location=fridge")
	s.Sees("Used up 150 g → 0 g Fri, Nov 6 · 8:00 am Delivered 0 g → 150 g Thu, Nov 5 · 8:00 pm")
}

// The beat alone brings the Plan up to now, with no page opened: the snack's lasagne is
// eaten from the freezer, and the dinner's Rice bowl from the fridge, leaving the third
// one frozen.
func TestTheBeatBringsThePlanUpToNow(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)

	s.At("Fri 20:05")
	if got := assumed(t, s, domain.PortionEaten); got != 4 {
		t.Errorf("by the beat alone, %d Portions eaten, want the day's 4", got)
	}
	s.Open("/kitchen")
	s.Sees("lasagne × 2")
	s.DoesNotSee("Count Rice bowl in the fridge")
	s.Sees("Count Rice bowl in the freezer")
	s.Follows("lasagne")
	s.Sees("Eaten 3 → 2 Fri, Nov 6 · 5:00 pm")
	s.Open("/kitchen")
	s.Follows("Rice bowl")
	s.Sees("Prepared 0 → 1 Fri, Nov 6 · 1:00 pm")
}

// Running the step again for a moment it has reached changes nothing.
func TestBringingThePlanUpToNowTwiceChangesNothing(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Fri 13:01")
	var before, after int
	s.Scan(t, &before, `SELECT count(*) FROM movement`)
	s.Open("/kitchen")
	s.Open("/kitchen")
	s.Scan(t, &after, `SELECT count(*) FROM movement`)
	if before != after {
		t.Errorf("the kitchen held %d Movements, then %d once opened twice at the same moment", before, after)
	}
	s.Sees("rice 425 g")
}

// A Count that took hold of the kitchen first, dated after lunch, comes before lunch's
// Prep Session: what it used up lands at the Count's time, and the rice's Movements add
// up to what the Kitchen page shows.
func TestAPrepSessionAfterACountThatRacedTheBeat(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Fri 12:59")
	s.Exec(t, `
		INSERT INTO movement (user_id, happened_at, ingredient_id, storage_location, change, cause_kind, believed, stated)
		SELECT user_id, '2026-11-06 13:00:30+01', ingredient_id, storage_location, -300, 'counted', 800, 500
		FROM movement WHERE cause_kind = 'counted' AND stated = 800`)

	s.At("Fri 13:05")
	s.Open("/kitchen")
	s.Sees("rice 125 g")
	s.Follows("rice")
	s.Sees("Used up 500 g → 125 g Fri, Nov 6 · 1:00 pm Counted 800 g → 500 g Fri, Nov 6 · 1:00 pm")
}

// weekend is a Cycle whose Delivery comes on Thursday 5 November between 6 and 8 pm and
// that ends on Sunday 8: its Slots are Friday's, Saturday's and Sunday's.
var weekend = apptest.Cycle{Delivery: "11/05/2026", From: "6pm", To: "8pm", Ends: "11/08/2026"}

// weekendDraft is a draft Plan of weekend, read from the prompt of `req`: a Rice bowl
// prepared at Friday's dinner for three Portions, eaten then and at Sunday's lunch, with
// the third frozen, and yogurt set out at every other Slot.
func weekendDraft(t *testing.T, req agent.Request) map[string]any {
	t.Helper()
	bowl := idIn(t, req, `Recipe (\d+): Rice bowl`)
	pot := idIn(t, req, `Ingredient (\d+): yogurt`)
	sessions := []map[string]any{
		{"on": "2026-11-06", "kind": "dinner", "recipe_id": bowl, "size": 1, "portions": []map[string]any{
			{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "dinner"},
			{"location": "fridge", "eat_on": "2026-11-08", "eat_kind": "lunch"},
			{"location": "freezer"},
		}},
	}
	for _, day := range []string{"2026-11-06", "2026-11-07", "2026-11-08"} {
		for _, kind := range []string{"breakfast", "lunch", "snack", "dinner"} {
			if (day == "2026-11-06" && kind == "dinner") || (day == "2026-11-08" && kind == "lunch") {
				continue
			}
			sessions = append(sessions, map[string]any{"on": day, "kind": kind, "ingredient_id": pot, "quantity": 150, "portions": []map[string]any{
				{"location": "fridge", "eat_on": day, "eat_kind": kind},
			}})
		}
	}
	return map[string]any{"prep_sessions": sessions}
}

// A dinner Prep Session's second Portion, planned for lunch two days later, is eaten
// then, and the third stays in the freezer.
func TestADinnersSecondPortionIsEatenTwoDaysLater(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", weekendDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Rice bowls over the weekend."),
	)
	s.OpensCycle(weekend)
	s.PlansCycle()
	s.Sees("Planning a Cycle Succeeded")
	s.Open("/plan")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")

	s.At("Sun 12:00")
	s.Open("/kitchen")
	s.Sees("Count Rice bowl in the fridge")
	s.Sees("Count Rice bowl in the freezer")

	s.At("Sun 13:00")
	s.Open("/kitchen")
	s.DoesNotSee("Count Rice bowl in the fridge")
	s.Sees("Count Rice bowl in the freezer")
}

// Only the current Cycle's Plan writes: once the next Cycle's Delivery arrives, at noon
// on Friday, the closing Cycle's later Slots write nothing, and the next Cycle, with no
// Plan, writes nothing either. Friday's breakfast, before it, is still prepared.
func TestOnlyTheCurrentCyclesPlanHappens(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Thu 20:00")
	s.OpensCycle(saturday)
	s.IsConfirmed("Cycle opened.")

	s.At("Fri 21:00")
	s.Open("/kitchen")
	s.Sees("rice 800 g")
	s.Sees("lasagne × 3")
	s.DoesNotSee("Rice bowl")
	if got := assumed(t, s, domain.UsedUp); got != 1 {
		t.Errorf("%d Stock Items used up, want breakfast's yogurt alone", got)
	}
	s.Open("/kitchen/movements?ingredient=2&location=fridge")
	s.Sees("Used up 150 g → 0 g Fri, Nov 6 · 8:00 am")
}
