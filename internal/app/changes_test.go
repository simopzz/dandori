package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// planned plans friday into its Plan, approved, through the Kitchen Check.
func planned(t *testing.T, s *apptest.Scenario) {
	t.Helper()
	plansFriday(t, s, "Rice bowls for Friday.")
	s.Open("/plan")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
}

// snackChanged is a draft of friday from its snack, read from the prompt of req: yogurt
// set out at the snack, and the Rice bowl prepared at lunch eaten at dinner again.
func snackChanged(t *testing.T, req agent.Request) map[string]any {
	t.Helper()
	pot := idIn(t, req, `Ingredient (\d+): yogurt`)
	bowl := idIn(t, req, `Portion (-\d+): Rice bowl, fridge`)
	return map[string]any{
		"prep_sessions": []map[string]any{
			{"on": "2026-11-06", "kind": "snack", "ingredient_id": pot, "quantity": 150, "portions": []map[string]any{
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "snack"},
			}},
		},
		"kept": []map[string]any{{"portion_id": bowl, "on": "2026-11-06", "kind": "dinner"}},
	}
}

// snackChangedPlanned scripts a planning Run into a pending Plan Proposal of
// snackChanged summed up as summary.
func snackChangedPlanned(t *testing.T, s *apptest.Scenario, summary string) {
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", snackChanged(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply(summary),
	)
}

// Asking for changes on Friday afternoon starts a planning Run with no Kitchen Check,
// told the Planning Note, from the first Slot not yet prepared: the snack. It is told
// what of the Plan it replaces, and the Portions the Rice bowl prepared at lunch frees
// to put in Slots again. The Plan stays as it is until a Plan Proposal is approved;
// rejecting that with a note plans again, and approving it changes the Plan from the
// snack on.
func TestAskingForChangesReplansFromTheFirstSlotNotYetPrepared(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	planned(t, s)
	s.At("Fri 14:00")
	s.Counts("yogurt", "", "500", "")

	snackChangedPlanned(t, s, "Yogurt at the snack.")
	s.Open("/plan")
	s.AsksForChanges("No lasagne at the snack.")
	s.IsAt("/runs/2")
	s.Settle()
	told(t, s, []string{
		"Plan every Slot from snack on Fri 6 Nov on.",
		"<note>\nNo lasagne at the snack.\n</note>",
		": Rice bowl, fridge, each",
		": Rice bowl, freezer, each",
		"This Cycle is current: its Delivery has arrived",
		"The Cycle has a Plan, which this Plan Proposal replaces from snack on Fri 6 Nov on",
		"- snack on Fri 6 Nov: Portion 1, lasagne\n- dinner on Fri 6 Nov: Rice bowl at 1.25×, prepared at lunch on Fri 6 Nov",
	}, "breakfast on Fri 6 Nov: 150 g yogurt", "lunch on Fri 6 Nov: Rice bowl")

	s.Open("/plan")
	s.Sees("01 Plan Proposal")
	s.Sees("Yogurt at the snack.")
	s.Rejects("")
	s.Sees("lasagne 620 kcal · 32 g protein Snack already in the kitchen")
	s.Sees("Rice bowl 1.25× · 438 kcal · 9 g protein Dinner cooked")

	snackChangedPlanned(t, s, "Yogurt at the snack, again.")
	s.AsksForChanges("No lasagne at the snack.")
	s.Settle()
	snackChangedPlanned(t, s, "Yogurt at the snack, and less rice.")
	s.Open("/plan")
	s.Rejects("Less rice.")
	s.IsAt("/runs/4")
	s.Settle()
	told(t, s, []string{"Plan every Slot from snack on Fri 6 Nov on.", "<note>\nLess rice.\n</note>\n<note>\nNo lasagne at the snack.\n</note>"})

	s.Open("/plan")
	s.Sees("Yogurt at the snack, and less rice.")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Sees("yogurt 90 kcal · 15 g protein Breakfast in the fridge")
	s.Sees("Rice bowl 1.25× · 438 kcal · 9 g protein Lunch cooked")
	s.Sees("yogurt 90 kcal · 15 g protein Snack in the fridge")
	s.Sees("Rice bowl 1.25× · 438 kcal · 9 g protein Dinner cooked")
	s.DoesNotSee("lasagne 620 kcal")
}

// A Plan Proposal from asking for changes marks what it changes of the Plan: one line
// in Section 01 with the days, each changed day's row, and in it each changed Slot and
// Prep Session; the rest is unmarked.
func TestAPlanProposalMarksWhatItChangesOfThePlan(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	planned(t, s)
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			d := fridayDraft(t, req)
			d["prep_sessions"].([]map[string]any)[1]["size"] = 1
			return fakellm.CallTool("check_plan", d)
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Smaller Rice bowls."),
	)
	s.AsksForChanges("Smaller bowls.")
	s.Settle()
	told(t, s, []string{"Plan every Slot from breakfast on Fri 6 Nov on.", "- breakfast on Fri 6 Nov: 150 g yogurt, set out at breakfast on Fri 6 Nov"})

	s.Open("/plan")
	s.Sees("Changes from the Plan: Fri, Nov 6 1 day more than 25% off")
	s.Sees("Friday, November 6 changed")
	s.Sees("yogurt 90 kcal · 15 g protein Breakfast in the fridge")
	s.Sees("Rice bowl changed 1× · 350 kcal · 7 g protein Lunch cooked")
	s.Sees("lasagne 620 kcal · 32 g protein Snack already in the kitchen")
	s.Sees("Rice bowl changed 1× · 350 kcal · 7 g protein Dinner cooked")
	s.Sees("Rice bowl changed 3 at 1×")
	if n := strings.Count(apptest.Text(s.Page().Body), "changed"); n != 4 {
		t.Errorf("the Plan Proposal marks %d things changed, want Friday, its lunch, dinner and Rice bowl", n)
	}
}

// A Plan Proposal from asking for changes before the Delivery marks the freezer splits it
// changes of the Plan: each it names at another quantity, and the Delivery's day. One
// naming none, where the Plan has some, says nothing goes to the freezer.
func TestAPlanProposalMarksTheFreezerSplitsItChanges(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridayWithFreezerSplit(t, s)
	s.Open("/plan")
	s.Submits("approve")

	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			d := fridayDraft(t, req)
			d["freezer_splits"] = []map[string]any{{"ingredient_id": idIn(t, req, `Ingredient (\d+): chicken breast`), "quantity": 300}}
			return fakellm.CallTool("check_plan", d)
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Less chicken to freeze."),
	)
	s.AsksForChanges("Less chicken in the freezer.")
	s.Settle()
	s.Open("/plan")
	s.Sees("Changes from the Plan: Thu, Nov 5")
	s.Sees("Thursday, November 5 changed 1 to the freezer at put-away To the freezer at put-away chicken breast changed 300 g Friday, November 6")
	if n := strings.Count(apptest.Text(s.Page().Body), "changed"); n != 2 {
		t.Errorf("the Plan Proposal marks %d things changed, want Thursday and its chicken breast", n)
	}

	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("No chicken to freeze."),
	)
	s.Rejects("None in the freezer.")
	s.Settle()
	s.Open("/plan")
	s.Sees("Changes from the Plan: Thu, Nov 5")
	s.Sees("Thursday, November 5 changed To the freezer at put-away Nothing to the freezer Friday, November 6")
}

// Asking for changes with an empty Planning Note is refused on its field, the step left
// open, and starts no Run; as htmx it answers the same, and with a note lands on the Run.
func TestAskingForChangesNeedsAPlanningNote(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	planned(t, s)
	s.AsksForChanges("  ")
	s.IsRefused()
	s.Sees("Ask for changes " + askHint + " Planning Note Say what to change.")
	if !strings.Contains(s.Page().Body, `<details class="button-fold" open`) {
		t.Error("the refused request for changes is folded away")
	}
	if got := len(s.Model.Requests()); got != 3 {
		t.Errorf("the model was asked %d times, want only the first Run's 3", got)
	}

	form := apptest.Form(t, s.Page().Body, "ask-for-changes")
	if got := s.HTMXPostForm(t, form.Action, form.Values); got.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apptest.Text(got.Body), "Say what to change.") {
		t.Errorf("an empty note as htmx answers %d, want 422 saying so", got.StatusCode)
	}
	s.Model.Script(fakellm.Hang())
	form.Values.Set("note", "Smaller bowls.")
	if got := s.HTMXPostForm(t, form.Action, form.Values); got.Path != "/runs/2" {
		t.Errorf("asking for changes as htmx lands on %s, want /runs/2", got.Path)
	}
}

// askHint is what opening Ask for changes says it does.
const askHint = "Dandori plans the Cycle again from the next Slot, told your note. The Plan stays as it is until you approve the new Plan Proposal."

// Ask for changes is there only on a Cycle with a Plan while no Plan Proposal is
// pending and no planning Run is going, for any Cycle.
func TestAskForChangesIsAbsentWhileAnythingIsPendingOrGoing(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.OpensCycle(friday)
	check := s.Page().Path
	s.Open("/plan")
	s.DoesNotSee("Ask for changes")
	fridayPlanned(t, s, "Rice bowls for Friday.")
	s.Open(check)
	s.PlansCycle()
	s.Open("/plan")
	s.DoesNotSee("Ask for changes")

	s.Submits("approve")
	s.Sees("03 Upcoming Cycle Ask for changes")
	s.Model.Script(fakellm.Hang())
	s.AsksForChanges("Smaller bowls.")
	s.Open("/plan")
	s.DoesNotSee("Ask for changes")
	s.Open("/runs/2")
	s.CancelsRun(apptest.Form(t, s.Page().Body, "cancel-run"))

	s.At("Thu 20:00")
	s.OpensCycle(afterFriday)
	s.Model.Script(fakellm.Hang())
	s.StartsPlanning()
	s.Open("/plan")
	s.Sees("Cycle · Thu, Nov 5 – Fri, Nov 6")
	s.DoesNotSee("Ask for changes")
}

// While asking for changes plans the Cycle again, the Plan page says it is being planned,
// linking to the Run; once that Run fails, it says why, links to its Trace and offers
// Plan again in place of Ask for changes, which plans again from the same Slot.
func TestThePlanPageShowsAReplanGoingOrFailed(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	planned(t, s)
	s.Model.Script(fakellm.Hang())
	s.AsksForChanges("Smaller bowls.")
	s.Open("/plan")
	s.Sees("03 Upcoming Cycle Being planned See progress Drop the Cycle")
	if got := apptest.Link(t, s.Page().Body, "See progress"); got != "/runs/2" {
		t.Errorf("See progress leads to %q, want /runs/2", got)
	}
	s.Open("/runs/2")
	s.CancelsRun(apptest.Form(t, s.Page().Body, "cancel-run"))

	s.Open("/plan")
	s.Sees("03 Upcoming Cycle Ask for changes")
	s.Model.Script(fakellm.Reply("No smaller bowls."))
	s.AsksForChanges("Smaller bowls.")
	s.Settle()
	s.Open("/plan")
	s.Sees("02 Days")
	s.Sees("03 Upcoming Cycle Planning failed: The model ended without a Plan Proposal. No smaller bowls.")
	if got := apptest.Link(t, s.Page().Body, "See its Trace"); got != "/runs/3" {
		t.Errorf("the failed Run's Trace link leads to %q, want /runs/3", got)
	}
	s.DoesNotSee("Ask for changes")

	s.Model.Script(fakellm.Reply("Still no smaller bowls."))
	s.Submits("plan-again")
	s.IsAt("/runs/4")
	s.Settle()
	told(t, s, []string{"Plan every Slot from breakfast on Fri 6 Nov on.", "<note>\nSmaller bowls.\n</note>"})
}

// Asking for changes once the Delivery has arrived keeps the Plan's freezer splits: the
// Delivery is put away by them. The planner is told so, and check_plan refuses a split.
func TestAReplanAfterTheDeliveryKeepsThePlansFreezerSplits(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridayWithFreezerSplit(t, s)
	s.Open("/plan")
	s.Submits("approve")
	s.At("Thu 21:00")
	s.Counts("yogurt", "", "500", "")

	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			d := fridayDraft(t, req)
			d["freezer_splits"] = []map[string]any{{"ingredient_id": idIn(t, req, `Ingredient (\d+): chicken breast`), "quantity": 300}}
			return fakellm.CallTool("check_plan", d)
		}),
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 2}),
		fakellm.Reply("No more chicken to freeze."),
	)
	s.Open("/plan")
	s.AsksForChanges("Less chicken in the freezer.")
	s.Settle()
	told(t, s, []string{"This Cycle is current", "Its Delivery is put away too, so name no freezer split."})
	s.Open(s.Page().Path)
	s.Sees("Draft 1 breaks these hard rules: - split-after-put-away: 300 g chicken breast is split to the freezer, and the Delivery is already put away")

	s.Open("/plan")
	s.DoesNotSee("Nothing to the freezer")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Sees("To the freezer at put-away chicken breast 400 g")
}

// On the current Cycle check_plan reports a Prep Session needing more of an Ingredient
// than the kitchen will hold: no Delivery brings the rest.
func TestCheckPlanReportsMoreThanTheKitchenHoldsOnTheCurrentCycle(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	planned(t, s)
	s.At("Fri 14:00")
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", snackChanged(t, req)) }),
		fakellm.Reply("No yogurt."),
	)
	s.AsksForChanges("No lasagne at the snack.")
	s.Settle()
	s.Open(s.Page().Path)
	s.Sees("Draft 1 breaks these hard rules: - more-than-held: yogurt at snack on Fri 6 Nov needs 150 g yogurt, and the kitchen will hold only 0 g")
}
