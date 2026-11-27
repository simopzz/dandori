package app_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/apptest"
)

// frozeRice approves friday as approvesFriday does, and moves 600 g of the pantry's rice
// to the freezer on Thursday morning: of the 375 g Friday's lunch needs, the pantry holds
// 200 g, so 175 g is thawed on Thursday evening, with the lasagne filling the snack.
func frozeRice(t *testing.T) *apptest.Scenario {
	t.Helper()
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Thu 09:00")
	s.Moves("Move rice in the pantry", "600", "freezer", "")
	return s
}

// After the previous day's dinner, the Kitchen page shows the frozen food the next day
// needs in the fridge, each with its Movement saying thawed.
func TestTheEveningBeforeThawsIntoTheFridge(t *testing.T) {
	s := frozeRice(t)
	s.At("Thu 19:59")
	s.Open("/kitchen")
	s.Sees("Freezer lasagne × 3")
	s.DoesNotSee("Count rice in the fridge")
	s.At("Thu 20:05")
	s.Open("/kitchen")
	s.Sees("Fridge lasagne × 1")
	s.Sees("Freezer lasagne × 2")
	s.Sees("rice 175 g")
	s.Open("/kitchen/movements?location=fridge&ingredient=" + ingredientID(t, s, "rice"))
	s.Sees("Thawed 0 g → 175 g Thu, Nov 5 · 8:00 pm")
	s.Open("/kitchen/movements?location=freezer&ingredient=" + ingredientID(t, s, "rice"))
	s.Sees("Thawed 600 g → 425 g Thu, Nov 5 · 8:00 pm")
	s.Open("/kitchen")
	s.Follows("lasagne")
	s.Sees("lasagne · Fridge Thawed 0 → 1 Thu, Nov 5 · 8:00 pm")
}

// Through the day Today lists tonight's thaws, what, how much and the Slot each is for:
// worked out on the kitchen as it holds now, and once dinner has passed, those made. With
// no Plan for the day after, it lists none.
func TestTodayListsTonightsThaws(t *testing.T) {
	s := frozeRice(t)
	const tonight = "Out of the freezer tonight rice 175 g for lunch tomorrow lasagne × 1 for snack tomorrow"
	s.Open("/")
	s.Sees(tonight)
	s.At("Thu 21:00")
	s.Open("/")
	s.Sees(tonight)
	s.At("Fri 09:00")
	s.Open("/")
	s.DoesNotSee("Out of the freezer tonight")
}

// On the current Cycle's last evening the thaw is from the upcoming Cycle's Plan, before
// its Delivery arrives: friday ends on Friday, and afterFriday's Plan keeps a lasagne for
// Saturday's lunch, which Friday evening thaws and Today lists that day.
func TestTheLastEveningThawsForTheUpcomingCycle(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Thu 20:00")
	approves(t, s, func(req agent.Request) map[string]any {
		return map[string]any{
			"prep_sessions": yogurtAt(idIn(t, req, `Ingredient (\d+): yogurt`), []string{"2026-11-07", "2026-11-08"}, "2026-11-07 lunch"),
			"kept":          []map[string]any{{"portion_id": idIn(t, req, `Portion (\d+): lasagne`), "on": "2026-11-07", "kind": "lunch"}},
		}
	}, func() { s.OpensCycle(afterFriday); s.PlansCycle() })

	s.At("Fri 18:00") // the snack ate the lasagne Thursday thawed
	s.Open("/")
	s.Sees("Out of the freezer tonight lasagne × 1 for lunch tomorrow")
	s.At("Fri 20:05")
	s.Open("/kitchen")
	s.Sees("Fridge lasagne × 1")
	s.Sees("Freezer lasagne × 1")
}
