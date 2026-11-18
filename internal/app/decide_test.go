package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// Approving the pending Plan Proposal makes it the Cycle's Plan: the Cycle page shows it
// by day, each Slot at its time, and no Plan Proposal waiting or way to plan again.
func TestApprovingAPlanProposalShowsThePlanByDay(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")

	s.Open("/plan")
	s.Submits("approve")
	s.IsAt("/plan")
	s.IsConfirmed("Plan approved.")
	s.Sees("01 Plan Friday, November 6")
	s.Sees("yogurt 90 kcal · 15 g protein Breakfast 8:00 am")
	s.Sees("Rice bowl 1.25× · 438 kcal · 9 g protein Lunch 1:00 pm")
	s.Sees("Rice bowl: 3 Portions at 1.25×")
	s.Sees("02 Upcoming Cycle")
	s.DoesNotSee("Plan Proposal")
	s.DoesNotSee("Rice bowls for Friday.")
	s.DoesNotSee("Plan the Cycle")
}

// lastPrompt is what the planning Run s started last was told.
func lastPrompt(t *testing.T, s *apptest.Scenario) string {
	t.Helper()
	reqs := s.Model.Requests()
	for i := len(reqs) - 1; i >= 0; i-- {
		if len(reqs[i].Messages) == 1 {
			return reqs[i].Messages[0].Content[0].Text
		}
	}
	t.Fatal("the model was asked nothing")
	return ""
}

// told checks the prompt of the planning Run s started last holds each of want, and
// none of absent.
func told(t *testing.T, s *apptest.Scenario, want []string, absent ...string) {
	t.Helper()
	got := lastPrompt(t, s)
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("the planner isn't told %q:\n%s", w, got)
		}
	}
	for _, a := range absent {
		if strings.Contains(got, a) {
			t.Errorf("the planner is told %q:\n%s", a, got)
		}
	}
}

// fridayPlanned scripts a planning Run of friday into a pending Plan Proposal summed up
// as summary.
func fridayPlanned(t *testing.T, s *apptest.Scenario, summary string) {
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply(summary),
	)
}

// Once the Plan Proposal's first Slot has passed, approving it is refused, saying to
// reject it with a Planning Note or plan again; it stays pending.
func TestApprovalIsRefusedOnceTheFirstSlotHasPassed(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")

	s.At("Fri 08:00")
	s.Open("/plan")
	s.Sees("01 Plan Proposal")
	s.Sees("Rice bowls for Friday.")
	s.Submits("approve")
	if s.Page().StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("approving a Plan Proposal starting in the past = %d, want %d", s.Page().StatusCode, http.StatusUnprocessableEntity)
	}
	s.Sees("This Plan Proposal starts at a Slot that has passed, so it can't be approved. Reject it with a Planning Note to have it planned again, or reject it and plan the Cycle again.")
	s.Sees("01 Plan Proposal")
	s.Sees("Rice bowls for Friday.")
	s.Sees("02 Current Cycle")
}

// A changed Slot time moves the Plan's Slots: a breakfast moved later is still to come,
// so its Plan Proposal can be approved, and the Plan shows it at its new time.
func TestAChangedSlotTimeMovesThePlansSlots(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")

	s.At("Fri 08:30")
	s.Open("/plan")
	s.Submits("approve")
	s.Sees("starts at a Slot that has passed")

	s.Open("/profile")
	s.SavesTimesOfDay(apptest.TimesOfDay{Breakfast: "9:00", Lunch: "13:00", Snack: "17:00", Dinner: "20:00"})
	s.Open("/plan")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Sees("yogurt 90 kcal · 15 g protein Breakfast 9:00 am")
}

// Rejecting without a Planning Note starts nothing: the Cycle can be planned again
// through the Kitchen Check.
func TestRejectingWithoutANoteStartsNothing(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")

	s.Open("/plan")
	s.Rejects("  ")
	s.IsAt("/plan")
	s.IsConfirmed("Plan Proposal rejected.")
	s.DoesNotSee("Rice bowls for Friday.")
	s.DoesNotSee("Being planned")
	s.Sees("Plan the Cycle")
	if got := len(s.Model.Requests()); got != 3 {
		t.Errorf("the model was asked %d times, want only the first Run's 3", got)
	}
}

// Rejecting with a Planning Note starts a planning Run with no Kitchen Check, from the
// rejected Plan Proposal's first Slot, told every Planning Note of the Cycle, newest
// first; once that Slot has passed, from the first Slot still to come. Planning again
// is told them too.
func TestRejectingWithANotePlansAgainToldEveryNoteNewestFirst(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")

	fridayPlanned(t, s, "Less rice on Friday.")
	s.Open("/plan")
	s.Rejects("Less rice, please.")
	s.IsAt("/runs/2")
	s.Settle()
	told(t, s, []string{
		"Plan every Slot from breakfast on Fri 6 Nov on.",
		"The user's Planning Notes on this Cycle, newest first. Keep to every one: a newer note overrides an older one only where the two contradict each other.\n<note>\nLess rice, please.\n</note>",
	})

	s.Model.Script(fakellm.Reply("I can't."))
	s.At("Fri 08:00")
	s.Open("/plan")
	s.Sees("Plan Proposal")
	s.Sees("Less rice on Friday.")
	s.Rejects("No lasagne.")
	s.IsAt("/runs/3")
	s.Settle()
	told(t, s, []string{
		"Its Slots run from breakfast on Fri 6 Nov to dinner on Fri 6 Nov. Plan every Slot from lunch on Fri 6 Nov on.",
		"<note>\nNo lasagne.\n</note>\n<note>\nLess rice, please.\n</note>",
	})

	s.Model.Script(fakellm.Reply("Still can't."))
	s.Open("/plan")
	s.Submits("plan-again")
	s.Settle()
	told(t, s, []string{
		"Plan every Slot from lunch on Fri 6 Nov on.",
		"<note>\nNo lasagne.\n</note>\n<note>\nLess rice, please.\n</note>",
	})
}

// A failed planning Run shows its reason on the Cycle page, with a link to its Trace and
// Plan again, which starts another from the same Slot.
func TestAFailedPlanningRunOffersToPlanAgain(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(fakellm.Reply("The kitchen is empty."))
	s.OpensCycle(friday)
	s.PlansCycle()

	s.Open("/plan")
	s.Sees("Planning failed: The model ended without a Plan Proposal. The kitchen is empty.")
	if got := apptest.Link(t, s.Page().Body, "See its Trace"); got != "/runs/1" {
		t.Errorf("the failed Run's Trace link leads to %q, want /runs/1", got)
	}
	s.DoesNotSee("Plan the Cycle")

	fridayPlanned(t, s, "Friday after all.")
	s.Submits("plan-again")
	s.IsAt("/runs/2")
	s.Settle()
	told(t, s, []string{"Plan every Slot from breakfast on Fri 6 Nov on."})
	s.Open("/plan")
	s.Sees("01 Plan Proposal")
	s.Sees("Friday after all.")
	s.DoesNotSee("Planning failed")
}

// Each decision answers htmx as it does a plain post: approving and rejecting land on
// the Cycle page, rejecting with a note and planning again on the Run, and a refused
// approval is the Cycle page with 422.
func TestDecisionsPostedAsHTMX(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	page := s.Get(t, "/plan").Body

	fridayPlanned(t, s, "Friday again.")
	reject := apptest.Form(t, page, "reject")
	reject.Values.Set("note", "Less rice.")
	if got := s.HTMXPostForm(t, reject.Action, reject.Values); got.Path != "/runs/2" {
		t.Errorf("rejecting with a note as htmx lands on %s, want /runs/2", got.Path)
	}
	s.Settle()

	approve := apptest.Form(t, s.Get(t, "/plan").Body, "approve")
	s.At("Fri 08:00")
	if got := s.HTMXPostForm(t, approve.Action, approve.Values); got.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apptest.Text(got.Body), "can't be approved") {
		t.Errorf("a refused approval as htmx answers %d, want 422 saying why", got.StatusCode)
	}
	reject = apptest.Form(t, s.Get(t, "/plan").Body, "reject")
	if got := s.HTMXPostForm(t, reject.Action, reject.Values); got.Path != "/plan" || !strings.Contains(apptest.Text(got.Body), "Plan the Cycle") {
		t.Errorf("rejecting as htmx lands on %s, want the Cycle page", got.Path)
	}
	if got := s.HTMXPostForm(t, reject.Action, reject.Values); got.StatusCode != http.StatusNotFound {
		t.Errorf("rejecting a decided Plan Proposal = %d, want 404", got.StatusCode)
	}
}

// Once every Slot has passed, rejecting with a Planning Note is refused with 422, the
// note still typed, and the Plan Proposal stays pending.
func TestARefusedRejectionKeepsItsPlanningNote(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")

	s.At("Fri 23:00")
	s.Open("/plan")
	s.Rejects("No lasagne.")
	if s.Page().StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("rejecting with nothing left to plan = %d, want %d", s.Page().StatusCode, http.StatusUnprocessableEntity)
	}
	if got := apptest.Form(t, s.Page().Body, "reject").Values.Get("note"); got != "No lasagne." {
		t.Errorf("the refused rejection's note = %q, want it as typed", got)
	}
	s.Sees("01 Plan Proposal")
	s.Sees("Rice bowls for Friday.")
}

// Planning again and approving, posted as htmx, land where a plain post does: the Run,
// then the Cycle page with its Plan.
func TestPlanningAgainAndApprovingPostedAsHTMX(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(fakellm.Reply("The kitchen is empty."))
	s.OpensCycle(friday)
	s.PlansCycle()

	fridayPlanned(t, s, "Friday after all.")
	again := apptest.Form(t, s.Get(t, "/plan").Body, "plan-again")
	if got := s.HTMXPostForm(t, again.Action, again.Values); got.Path != "/runs/2" {
		t.Errorf("planning again as htmx lands on %s, want /runs/2", got.Path)
	}
	s.Settle()

	approve := apptest.Form(t, s.Get(t, "/plan").Body, "approve")
	if got := s.HTMXPostForm(t, approve.Action, approve.Values); got.Path != "/plan" || !slices.Equal(apptest.Notices(t, got.Body), []string{"Plan approved."}) {
		t.Errorf("approving as htmx lands on %s, want the Cycle page saying so", got.Path)
	}
}
