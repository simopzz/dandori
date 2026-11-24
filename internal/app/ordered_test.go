package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// Marking the Delivery ordered from the Grocery List fixes the list: an approved replan
// after it, needing less rice, leaves the list as it was ordered. The Cycle page and the
// list say ordered.
func TestMarkingTheDeliveryOrderedFixesTheGroceryList(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	s.Follows("Grocery List")
	s.Sees("Delivery Sat, Nov 7, 6:00 pm to 8:00 pm Tentative")
	s.Submits("mark-ordered")
	s.IsConfirmed("Delivery marked ordered.")
	s.IsAt("/plan/cycles/2/grocery-list")
	s.Sees("Delivery Sat, Nov 7, 6:00 pm to 8:00 pm Ordered")
	s.DoesNotSee("Tentative")
	s.Sees("01 To buy rice (25 g) yogurt (250 g) 02 Check you still have olive oil")

	plansSunday(t, s, 1.25)
	s.Open("/plan/cycles/2")
	s.Sees("Delivery Sat, Nov 7, 6:00 pm to 8:00 pm Ordered")
	s.AsksForChanges("Smaller Portions.")
	s.Settle()
	s.Open("/plan/cycles/2")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Follows("Grocery List")
	s.Sees("01 To buy rice (25 g) yogurt (250 g) 02 Check you still have olive oil")
}

// Marking the Delivery ordered is refused, with the reason, while the Cycle has no Plan,
// while a Plan Proposal of it is pending, and while a planning Run of it is going; the
// Delivery stays tentative.
func TestMarkingOrderedIsRefusedUntilThePlanIsDecided(t *testing.T) {
	t.Run("no Plan", func(t *testing.T) {
		s := plannerOf(t, "ada@example.com")
		s.OpensCycle(friday)
		got := s.PostForm(t, "/plan/cycles/1/ordered", nil)
		refusedWith(t, got, "The Cycle has no Plan yet, so there's nothing to buy.")
	})
	for _, c := range []struct {
		name string
		// replan asks for changes to the Plan, leaving a Plan Proposal pending or a
		// planning Run going.
		replan func(t *testing.T, s *apptest.Scenario)
		says   string
	}{
		{"a Plan Proposal pending", func(t *testing.T, s *apptest.Scenario) {
			plansSunday(t, s, 1.25)
			s.AsksForChanges("Smaller Portions.")
			s.Settle()
		}, "Decide on the Cycle's Plan Proposal first: approving it would change the list."},
		{"a planning Run going", func(t *testing.T, s *apptest.Scenario) {
			s.Model.Script(fakellm.Hang())
			s.AsksForChanges("Smaller Portions.")
		}, "Wait for the Cycle to be planned first: its Plan Proposal could change the list."},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := sundayAfterFriday(t, 1.5)
			s.Follows("Grocery List")
			form := apptest.Form(t, s.Page().Body, "mark-ordered")
			s.Open("/plan/cycles/2")
			c.replan(t, s)
			refusedWith(t, s.PostForm(t, form.Action, form.Values), c.says)
			s.Open("/plan/cycles/2/grocery-list")
			s.Sees("Tentative")
		})
	}
}

// refusedWith checks `got` was refused, answered with 422, saying `why`.
func refusedWith(t *testing.T, got apptest.Response, why string) {
	t.Helper()
	if got.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apptest.Text(got.Body), why) {
		t.Errorf("answered %d, want 422 saying %q:\n%s", got.StatusCode, why, apptest.Text(got.Body))
	}
}

// Undoing ordered makes the Delivery tentative again: the list follows the Plan again, so
// the replan approved while it was ordered, needing no more rice than Friday leaves, now
// changes it, and the Cycle can be dropped again. While it is ordered the Cycle page
// offers no drop, and dropping is refused, saying why.
func TestUndoingOrderedMakesTheDeliveryTentativeAgain(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	s.Follows("Grocery List")
	s.Submits("mark-ordered")
	s.Open("/plan/cycles/2")
	s.DoesNotSee("Drop the Cycle")
	refusedWith(t, s.PostForm(t, "/plan/cycles/2/drop", nil), "The Cycle's Delivery is marked ordered: undo that on its Grocery List to drop the Cycle.")

	plansSunday(t, s, 1.25)
	s.Open("/plan/cycles/2")
	s.AsksForChanges("Smaller Portions.")
	s.Settle()
	s.Open("/plan/cycles/2")
	s.Submits("approve")
	s.Follows("Grocery List")
	s.Sees("01 To buy rice (25 g) yogurt (250 g)")
	s.Submits("undo-ordered")
	s.IsConfirmed("Delivery tentative again.")
	s.IsAt("/plan/cycles/2/grocery-list")
	s.Sees("Delivery Sat, Nov 7, 6:00 pm to 8:00 pm Tentative")
	s.Sees("01 To buy yogurt (250 g) 02 Check you still have")
	s.DoesNotSee("rice (")
	s.DropsCycle()
	s.IsConfirmed("Cycle dropped.")
}

// Today says "Order the next Delivery", linking to the Grocery List, while the upcoming
// Cycle has a Plan with nothing pending or going and its Delivery is tentative: not while
// a replan is pending, and not once the Delivery is ordered.
func TestTodayRemindsOfOrderingTheNextDelivery(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	s.Open("/")
	s.Follows("Order the next Delivery")
	s.IsAt("/plan/cycles/2/grocery-list")

	plansSunday(t, s, 1.25)
	s.Open("/plan/cycles/2")
	s.AsksForChanges("Smaller Portions.")
	s.Settle()
	s.Open("/")
	s.Sees("A Plan Proposal for the next Cycle is waiting")
	s.DoesNotSee("Order the next Delivery")

	s.Open("/plan/cycles/2")
	s.Submits("approve")
	s.Open("/")
	s.Sees("Order the next Delivery")
	s.Follows("Order the next Delivery")
	s.Submits("mark-ordered")
	s.Open("/")
	s.DoesNotSee("Order the next Delivery")
}

// Another User can neither mark ada's Delivery ordered nor undo it.
func TestOnlyItsUserMarksTheDeliveryOrdered(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	s.Invite("bob@example.com")
	s.SwitchesTo("bob@example.com")
	s.ChoosesLanguage("English")
	if got := s.PostForm(t, "/plan/cycles/2/ordered", nil); got.StatusCode != http.StatusNotFound {
		t.Errorf("another User marking the Delivery ordered answers %d, want 404", got.StatusCode)
	}
	s.SwitchesTo("ada@example.com")
	s.Open("/plan/cycles/2/grocery-list")
	s.Sees("Tentative")
	s.Submits("mark-ordered")

	s.SwitchesTo("bob@example.com")
	if got := s.PostForm(t, "/plan/cycles/2/tentative", nil); got.StatusCode != http.StatusNotFound {
		t.Errorf("another User undoing ordered answers %d, want 404", got.StatusCode)
	}
	s.SwitchesTo("ada@example.com")
	s.Open("/plan/cycles/2/grocery-list")
	s.Sees("Ordered")
}

// Through a browser: marking the Delivery ordered from the Grocery List confirms it, and
// the list then says ordered, offering to undo it.
func TestBrowserMarksTheDeliveryOrdered(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/plan/cycles/2/grocery-list")
	b.SeesInTurn(t, "Tentative", "01 To buy", "rice (25 g)", "Mark ordered")
	b.ShotPage(t, "grocery-list-tentative")
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Mark ordered"}).Click(); err != nil {
		t.Fatalf("mark the Delivery ordered: %v", err)
	}
	b.IsConfirmed(t, "Delivery marked ordered.")
	b.IsAt(t, "/plan/cycles/2/grocery-list")
	b.SeesInTurn(t, "Ordered", "01 To buy", "rice (25 g)", "yogurt (250 g)", "Undo ordered")
	b.ShotPage(t, "grocery-list-ordered")
}

// Once the Delivery is ordered, a replan makes do with what was ordered and what carries
// over: Sunday at 1.25× needs no more rice than the 425 g Friday leaves, so only yogurt
// is ordered, which the planner is told as stock arriving. check_plan reports a draft at
// 1.5× needing 450 g of rice, which wasn't ordered, and one at 1.25× becomes a
// Plan Proposal.
func TestAReplanAfterOrderingMakesDoWithWhatWasOrdered(t *testing.T) {
	s := sundayAfterFriday(t, 1.25)
	s.Follows("Grocery List")
	s.Sees("01 To buy yogurt (250 g)")
	s.Submits("mark-ordered")

	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", sundayDraft(t, req, 1.5)) }),
		fakellm.Reply("More rice."),
	)
	s.Open("/plan/cycles/2")
	s.AsksForChanges("Bigger Portions.")
	s.Settle()
	told(t, s, []string{
		"Stock Items:\n- 425 g rice (Ingredient",
		"Its Delivery is ordered, so it brings only these, put away where they go, besides the Stock Items above:\n- 250 g yogurt (Ingredient",
	})
	s.Open(s.Page().Path)
	s.Sees("Draft 1 breaks these hard rules: - more-than-held: Oiled rice at lunch on Sun 8 Nov needs 450 g rice, and the kitchen will hold only 425 g")

	plansSunday(t, s, 1.25)
	s.Open("/plan/cycles/2")
	s.Submits("plan-again")
	s.Settle()
	s.Open("/plan/cycles/2")
	s.Sees("01 Plan Proposal")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
}
