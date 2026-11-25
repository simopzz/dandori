package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// Approving the pending Plan Proposal makes it the Cycle's Plan: the Cycle page shows it
// by day, each Slot by its kind and no time of day, which only Today shows, and no Plan
// Proposal waiting or way to plan again, only to ask for changes.
func TestApprovingAPlanProposalShowsThePlanByDay(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")

	s.Open("/plan")
	s.Submits("approve")
	s.IsAt("/plan/cycles/1")
	s.IsConfirmed("Plan approved.")
	s.Sees("Cycle · Thu, Nov 5 – Fri, Nov 6 Delivery Thu, Nov 5, 6:00 pm to 8:00 pm Tentative")
	s.Sees("01 Plan Average " + heads + " calories 1586 under protein 65 under")
	s.Sees("02 Days " + heads + " Friday, November 6 calories 1586 under protein 65 under")
	s.Sees("Slots yogurt 90 kcal · 15 g protein Breakfast in the fridge")
	s.Sees("Rice bowl 1.25× · 438 kcal · 9 g protein Lunch cooked")
	s.Sees("Rice bowl 3 at 1.25× 25 min · 10 active")
	s.Sees("03 Upcoming Cycle Ask for changes")
	s.Sees("Ask for changes Drop the Cycle")
	s.DoesNotSee("01 Plan Proposal")
	s.DoesNotSee("Rice bowls for Friday.")
	s.DoesNotSee("Plan the Cycle")
}

// lastPrompt is what the planning Run `s` started last was told.
func lastPrompt(t *testing.T, s *apptest.Scenario) string {
	t.Helper()
	reqs := s.Model.Asked()
	for i := len(reqs) - 1; i >= 0; i-- {
		if len(reqs[i].Messages) == 1 {
			return firstTurn(reqs[i])
		}
	}
	t.Fatal("the model was asked nothing")
	return ""
}

// told checks the prompt of the planning Run `s` started last holds each of `want`, and
// none of `absent`.
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
// as `summary`.
func fridayPlanned(t *testing.T, s *apptest.Scenario, summary string) {
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply(summary),
	)
}

// Once the Plan Proposal's first Slot has passed, approving it is refused, saying to
// reject it with a Planning Note or plan again, with Reject open; it stays pending.
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
	s.Sees("02 Days")
	if !strings.Contains(s.Page().Body, `<details class="button-fold" open`) {
		t.Error("the refused approval says to reject with a Planning Note, but Reject is closed")
	}
}

// A Slot moved later by its Slot time is still to come: a Plan Proposal refused once its
// breakfast had passed can be approved after the breakfast moves later.
func TestASlotMovedLaterLetsItsPlanProposalBeApproved(t *testing.T) {
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
	s.Sees("yogurt 90 kcal · 15 g protein Breakfast in the fridge")
}

// Rejecting without a Planning Note starts nothing: the Cycle can be planned again
// through the Kitchen Check.
func TestRejectingWithoutANoteStartsNothing(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")

	s.Open("/plan")
	s.Rejects("  ")
	s.IsAt("/plan/cycles/1")
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
// first; once that Slot has passed, from the first Slot still to come. Planning again is
// told them too.
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

// Reject opens to the Cycle's earlier Planning Notes, newest first, then the line saying
// what a note does, then the note: none are listed before the first.
func TestRejectShowsTheEarlierPlanningNotesNewestFirst(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.Open("/plan")
	s.Sees("Reject With a note, Dandori plans again. Without one, it only rejects. Planning Note (optional)")
	s.DoesNotSee("Earlier Planning Notes")

	fridayPlanned(t, s, "Less rice on Friday.")
	s.Rejects("Less rice, please.")
	s.Settle()
	fridayPlanned(t, s, "No lasagne on Friday.")
	s.Open("/plan")
	s.Rejects("No lasagne.")
	s.Settle()

	s.Open("/plan")
	s.Sees("No lasagne on Friday.")
	s.Sees("Reject Earlier Planning Notes No lasagne. Less rice, please. " +
		"With a note, Dandori plans again. Without one, it only rejects. Planning Note (optional)")
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

// Each decision answers htmx as it does a plain post: approving and rejecting land on the
// Cycle page, rejecting with a note and planning again on the Run, and a refused approval
// is the Cycle page with 422.
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
	if got := s.HTMXPostForm(t, reject.Action, reject.Values); got.Path != "/plan/cycles/1" || !strings.Contains(apptest.Text(got.Body), "Plan the Cycle") {
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
	if got := s.HTMXPostForm(t, approve.Action, approve.Values); got.Path != "/plan/cycles/1" || !slices.Equal(apptest.Notices(t, got.Body), []string{"Plan approved."}) {
		t.Errorf("approving as htmx lands on %s, want the Cycle page saying so", got.Path)
	}
}

// Plan again with a Planning Note tells the planner the note, and keeps it among the
// Cycle's notes for every later Run; without one it plans as before.
func TestPlanAgainWithAPlanningNoteSendsItToThePlanner(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(fakellm.Reply("The kitchen is empty."))
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Open("/plan")
	s.Sees("Plan again Check the kitchen first")

	fridayPlanned(t, s, "Friday, no lasagne.")
	form := apptest.Form(t, s.Page().Body, "plan-again")
	form.Values.Set("note", "No lasagne.")
	s.PostForm(t, form.Action, form.Values)
	s.Settle()
	told(t, s, []string{"<note>\nNo lasagne.\n</note>"})

	s.Open("/plan")
	s.Sees("Earlier Planning Notes No lasagne.")
}

// A failed planning Run's Trace links back to its Cycle and offers Plan again, which a
// going one doesn't; both name the Cycle, and the Cycle page links to a going Run as
// being planned, with a filled Plan the Cycle button before Drop.
func TestPlanningTracesNameTheirCycle(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.OpensCycle(friday)
	s.Open("/plan")
	if !strings.Contains(s.Page().Body, `class="btn" href="/plan/cycles/1/check"`) {
		t.Errorf("Plan the Cycle isn't a button:\n%s", s.Page().Body)
	}
	if !before(s.Page().Body, "Plan the Cycle", "Drop the Cycle") {
		t.Errorf("Plan the Cycle isn't before Drop the Cycle:\n%s", s.Page().Body)
	}

	s.Model.Script(fakellm.Reply("The kitchen is empty."))
	s.Follows("Plan the Cycle")
	s.StartsPlanning()
	s.Sees("Planning a Cycle")
	if got := apptest.Link(t, s.Page().Body, "Planning the Cycle of Thu, Nov 5 – Fri, Nov 6"); got != "/plan/cycles/1" {
		t.Errorf("the Trace's Cycle links to %q, want its page", got)
	}

	s.Settle()
	s.Open("/runs/1")
	s.Sees("Plan again")
	s.Sees("Planning the Cycle of Thu, Nov 5 – Fri, Nov 6")
	s.Open("/plan")
	s.Sees("Planning failed")
	if !before(s.Page().Body, "Plan again", "Drop the Cycle") {
		t.Errorf("Plan again isn't before Drop the Cycle:\n%s", s.Page().Body)
	}
}

// No Cycle: Open a Cycle is a button.
func TestOpeningACycleIsAButton(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.Open("/plan")
	if !strings.Contains(s.Page().Body, `class="btn" href="/plan/open"`) {
		t.Errorf("Open a Cycle isn't a button:\n%s", s.Page().Body)
	}
}

// Through a browser, a planning Run's page marks the Plan tab as current.
func TestBrowserMarksThePlanTabOnAPlanningRun(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(fakellm.Reply("The kitchen is empty."))
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Settle()

	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/runs/1")
	b.Sees(t, "Planning the Cycle of Thu, Nov 5 – Fri, Nov 6")
	got, err := b.Locator("a[aria-current=page] span").First().InnerText()
	if err != nil || got != "Plan" {
		t.Errorf("the current tab on a planning Run is %q (%v), want Plan", got, err)
	}
}

// Through a browser, Plan again after a failed planning Run is a filled button, on the
// Cycle page and on the Run's Trace, opening to the hint and the Planning Note; planning
// from it tells the planner the note.
func TestBrowserPlanAgainIsAFilledButton(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(fakellm.Reply("The kitchen is empty."))
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Settle()

	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/runs/1")
	filled(t, b, "Plan again")
	b.Opens(t, "Plan again")
	b.Sees(t, planAgainHint)
	b.Open(t, "/plan")
	filled(t, b, "Plan again")
	b.Shot(t, "plan-again")

	b.Opens(t, "Plan again")
	b.Sees(t, planAgainHint)
	b.Shot(t, "plan-again-open")
	if err := b.GetByLabel("Planning Note").Fill("No lasagne."); err != nil {
		t.Fatalf("type the Planning Note: %v", err)
	}
	fridayPlanned(t, s, "Friday, no lasagne.")
	if err := b.Locator("#plan-again").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Plan again"}).Click(); err != nil {
		t.Fatalf("plan again: %v", err)
	}
	b.FollowsRun(t)
	told(t, s, []string{"<note>\nNo lasagne.\n</note>"})
}

// planAgainHint is what opening Plan again says a Planning Note does.
const planAgainHint = "Dandori plans again, with or without a note. A note tells it what to change."

// filled checks the fold that opens on `label` is a filled button, as Plan the Cycle's
// button is, not underlined text.
func filled(t *testing.T, b *apptest.Browser, label string) {
	t.Helper()
	summary := b.Locator("summary").Filter(playwright.LocatorFilterOptions{HasText: label}).First()
	bg, err := summary.Evaluate("el => getComputedStyle(el).backgroundColor", nil)
	if err != nil {
		t.Fatalf("no fold %q on %s: %v", label, b.URL(), err)
	}
	fill, err := b.Evaluate("() => { const e = document.createElement('a'); e.className = 'btn'; document.body.append(e); const c = getComputedStyle(e).backgroundColor; e.remove(); return c }")
	if err != nil {
		t.Fatalf("read a Button's fill: %v", err)
	}
	if bg != fill {
		t.Errorf("%s on %s isn't a filled button: background %v, a Button's %v", label, b.URL(), bg, fill)
	}
}

// before is whether both `a` and `b` are in `body`, `a` first.
func before(body, a, b string) bool {
	i, j := strings.Index(body, a), strings.Index(body, b)
	return i >= 0 && j >= 0 && i < j
}
