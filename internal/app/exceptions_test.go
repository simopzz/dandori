package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
)

// exceptionsPath is the path of the Exceptions page of the Cycle the Plan
// page shows last: the upcoming one when there is one.
func exceptionsPath(t *testing.T, h *apptest.Harness) string {
	t.Helper()
	page := h.Get(t, "/plan").Body
	i := strings.LastIndex(page, `/exceptions"`)
	if i < 0 {
		t.Fatalf("Plan page links no Exceptions page:\n%s", text(page))
	}
	return page[strings.LastIndex(page[:i], `"`)+1 : i+len(`/exceptions`)]
}

// logException fills the form of the Slot kind on day, as YYYY-MM-DD, on
// the Exceptions page at path with fields and posts it, as htmx when
// htmx is set.
func logException(t *testing.T, h *apptest.Harness, path, day, kind string, fields map[string]string, htmx bool) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, path).Body, "exception-"+day+"-"+kind)
	for k, v := range fields {
		form.Values.Set(k, v)
	}
	if htmx {
		return h.HTMXPostForm(t, form.Action, form.Values)
	}
	return h.PostForm(t, form.Action, form.Values)
}

func TestOpeningACycleLeadsToLoggingItsExceptions(t *testing.T) {
	h, _ := startPlanning(t)
	res := openCycle(t, h, "2026-11-02", "")
	if res.StatusCode != http.StatusOK || !strings.HasSuffix(res.Path, "/exceptions") {
		t.Fatalf("open Cycle = %d at %s, want its Exceptions page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	page := text(res.Body)
	// The Cycle's Slots, from breakfast on its delivery day to dinner on its
	// end date, all from the Plan until logged otherwise: 11 days of 4.
	for _, want := range []string{"Mon 2 Nov breakfast from the Plan", "Thu 12 Nov breakfast from the Plan"} {
		if !strings.Contains(page, want) {
			t.Errorf("Exceptions page does not show %q:\n%s", want, page)
		}
	}
	if got := strings.Count(res.Body, `id="exception-`); got != 44 || !strings.Contains(res.Body, `id="exception-2026-11-12-dinner"`) {
		t.Errorf("Exceptions page has %d Slots to log, want 44 to dinner on Thu 12 Nov", got)
	}

	res = logException(t, h, res.Path, "2026-11-07", "lunch", map[string]string{"exception": "meal-out"}, false)
	if res.StatusCode != http.StatusOK || !strings.HasSuffix(res.Path, "/exceptions") || res.Method != http.MethodGet {
		t.Fatalf("log a Meal Out = %d %s %s, want the Exceptions page after a redirect:\n%s", res.StatusCode, res.Method, res.Path, text(res.Body))
	}
	// The Profile's default estimate.
	if page := text(res.Body); !strings.Contains(page, "lunch Meal Out · 900 kcal · 35 g protein") {
		t.Errorf("Exceptions page does not show the Meal Out with the default estimate:\n%s", page)
	}
}

func TestExceptionsLoggedBeforePlanningAreWhatThePlanningRunSees(t *testing.T) {
	h, _, llm := planningRun(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	logException(t, h, exceptions, "2026-11-07", "lunch", map[string]string{"exception": "meal-out", "estimate_kcal": "1000"}, false)
	res := logException(t, h, exceptions, "2026-11-03", "snack", map[string]string{"exception": "skipped"}, true)
	if res.StatusCode != http.StatusOK || res.Method != http.MethodPost || !isRow(res.Body) || !strings.Contains(text(res.Body), "snack skipped") {
		t.Fatalf("HTMX skip a Slot = %d %s, want 200 and its row alone, skipped:\n%s", res.StatusCode, res.Method, res.Body)
	}

	// From six days before the Cycle's first day, all its trailing seven
	// days reach: the Meal Out with its own calories and the default for
	// the rest.
	seesExceptions(t, h, llm, "Exceptions from Tue 27 Oct to Thu 12 Nov: - snack on Tue 3 Nov: skipped, counts nothing. - lunch on Sat 7 Nov: Meal Out, estimated 1000 kcal, 35 g protein, 90 g carbs, 40 g fat.")

	// The lemon is set out and eaten at the skipped snack.
	page := text(h.Get(t, proposePlan(t, h, lemonPlan(t, h))).Body)
	for _, want := range []string{
		"lemon is prepared for snack on Tue 3 Nov, which is skipped",
		"lemon is eaten at snack on Tue 3 Nov, which is skipped",
		// The Untracked Allowance, the Meal Out and the frozen Lemon chicken:
		// 150 + 1000 + 589 kcal, 5 + 35 + 52 g protein, 15 + 90 + 64 g carbs,
		// 5 + 40 + 13 g fat.
		"Sat 7 Nov 1739 kcal · 92 g protein · 169 g carbs · 58 g fat lunch Meal Out dinner Lemon chicken with rice",
		// The lemon on the skipped snack isn't eaten: the Untracked Allowance
		// alone.
		"Tue 3 Nov 150 kcal · 5 g protein · 15 g carbs · 5 g fat snack skipped, not lemon",
		// TestPlanProposalShowsSlotsPrepSessionsNutritionAndBrokenRules's
		// 2837 kcal and 192 g, with the Meal Out's 1000 kcal and 35 g, less
		// the lemon's 20 kcal and 1 g.
		"Sun 8 Nov 150 kcal · 5 g protein · 15 g carbs · 5 g fat Seven days: 3817 kcal · 226 g protein",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Plan Proposal page does not show %q:\n%s", want, page)
		}
	}
}

// planningRun starts the app as startPlanning does, with a "plan" Run kind
// that lists the Exceptions, returning the fake LLM to script it with.
func planningRun(t *testing.T) (*apptest.Harness, *clock, *fakellm.LLM) {
	t.Helper()
	return startPlanningWith(t, apptest.WithAppJob("plan", func(a *app.App) agent.Job {
		return agent.Job{System: "You plan.", Prompt: "Plan the upcoming Cycle.", Tools: []agent.Tool{a.ExceptionsTool()}}
	}))
}

// seesExceptions runs a planning Run that lists the Exceptions and checks
// it was told want.
func seesExceptions(t *testing.T, h *apptest.Harness, llm *fakellm.LLM, want string) {
	t.Helper()
	llm.Script(fakellm.CallTool("list_exceptions", map[string]any{}), fakellm.Reply("Planned."))
	run, _ := finish(t, h, "plan")
	if trace := text(h.Get(t, run).Body); !strings.Contains(trace, "Tool result list_exceptions "+want+" ") {
		t.Errorf("the planning Run is not told %q:\n%s", want, trace)
	}
}

func TestExceptionOutlivesAMovedDeliveryAndADroppedCycle(t *testing.T) {
	h, _, llm := planningRun(t)
	logException(t, h, openCycle(t, h, "2026-11-02", "").Path, "2026-11-07", "lunch", map[string]string{"exception": "meal-out"}, false)

	if res := changeDelivery(t, h, map[string]string{"delivery_on": "2026-11-04"}); res.StatusCode != http.StatusOK {
		t.Fatalf("move Delivery = %d:\n%s", res.StatusCode, text(res.Body))
	}
	// The Plan page links the moved Cycle's Exceptions.
	if page := text(h.Get(t, exceptionsPath(t, h)).Body); !strings.Contains(page, "lunch Meal Out · 900 kcal") {
		t.Errorf("the Meal Out did not outlive moving the Delivery:\n%s", page)
	}

	drop := apptest.Form(t, h.Get(t, "/plan").Body, "drop")
	if res := h.PostForm(t, drop.Action, drop.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("drop Cycle = %d:\n%s", res.StatusCode, text(res.Body))
	}
	// Its day is now between Cycles, but still in the trailing seven days
	// of the next one.
	openCycle(t, h, "2026-11-09", "2026-11-12")
	seesExceptions(t, h, llm, "Exceptions from Tue 3 Nov to Thu 12 Nov: - lunch on Sat 7 Nov: Meal Out, estimated 900 kcal, 35 g protein, 90 g carbs, 40 g fat.")
}

func TestSkippingTheExceptionsLeavesNoneKnown(t *testing.T) {
	h, _, llm := startPlanningWith(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	// The planner reads the Exceptions, then stops without a Plan.
	llm.Script(fakellm.CallTool("list_exceptions", map[string]any{}), fakellm.Reply("Stopping here."))
	trace := text(planCycle(t, h, exceptions).Body)
	if want := "Tool result list_exceptions No Exceptions from Tue 27 Oct to Thu 12 Nov. "; !strings.Contains(trace, want) {
		t.Errorf("the planning Run is not told %q:\n%s", want, trace)
	}
}

func TestExceptionAPlanReliesOnNeedsANewPlanProposal(t *testing.T) {
	h, _ := startPlanning(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	// No Plan relies on it yet: it is only input to the planning Run.
	logException(t, h, exceptions, "2026-11-06", "lunch", map[string]string{"exception": "skipped"}, false)
	if page := text(h.Get(t, "/plan").Body); strings.Contains(page, "needs a new Plan Proposal") {
		t.Fatalf("an Exception before any Plan needs a new Plan Proposal:\n%s", page)
	}
	approvePlan(t, h, lemonPlan(t, h))
	pending := proposePlan(t, h, `{"prep_sessions": []}`)

	logException(t, h, exceptions, "2026-11-05", "dinner", map[string]string{"exception": "meal-out"}, false)
	if page := text(h.Get(t, "/plan").Body); !strings.Contains(page, "An Exception changed: this Plan needs a new Plan Proposal.") {
		t.Errorf("Plan page does not say the Plan needs a new Plan Proposal:\n%s", page)
	}
	// The one made without the Exception is replaced, saying by what.
	if page := text(h.Get(t, pending).Body); !strings.Contains(page, "replaced") || !strings.Contains(page, "Replaced by the Exception on Thu 5 Nov, dinner") {
		t.Errorf("the pending Plan Proposal was not replaced by the Exception:\n%s", page)
	}
}

func TestAnExceptionReplacesThePendingPlanProposalWithoutAPlanningNote(t *testing.T) {
	h, today, llm := startPlanningWith(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	today.set(t, "2026-11-03")
	// The Cycle is current and has no Plan yet.
	proposal := proposePlan(t, h, lemonPlan(t, h))

	logException(t, h, exceptions, "2026-11-06", "dinner", map[string]string{"exception": "meal-out"}, false)
	page := text(h.Get(t, proposal).Body)
	if !strings.Contains(page, "replaced") || !strings.Contains(page, "Replaced by the Exception on Fri 6 Nov, dinner") || strings.Contains(page, "Note:") {
		t.Errorf("the pending Plan Proposal was not replaced by the Exception, apart from any Planning Note:\n%s", page)
	}

	llm.Script(fakellm.Reply("Stopping here."))
	planCycle(t, h, exceptions)
	if prompt := plannerRequests(llm)[0].Messages[0].Content[0].Text; strings.Contains(prompt, "Replaced") || strings.Contains(prompt, "notes") {
		t.Errorf("the next planning Run is told the replacement as a Planning Note:\n%s", prompt)
	}
}

func TestAnExceptionOnTheCurrentCycleReplacesEitherOnesPlanProposal(t *testing.T) {
	h, today := startPlanning(t)
	current := openCycle(t, h, "2026-11-02", "").Path
	today.set(t, "2026-11-04")
	upcoming := openCycleIn(t, h, "2026-11-08", "18:00", "20:00", "").Path

	// What the current Cycle eats changes what carries over to the next.
	proposal := proposePlanFor(t, h, "2026-11-08", `{"prep_sessions": []}`)
	logException(t, h, current, "2026-11-05", "dinner", map[string]string{"exception": "meal-out"}, false)
	if page := text(h.Get(t, proposal).Body); !strings.Contains(page, "Replaced by the Exception on Thu 5 Nov, dinner") {
		t.Errorf("an Exception on the current Cycle did not replace the upcoming one's Plan Proposal:\n%s", page)
	}

	// What the upcoming Cycle eats changes nothing of the current one.
	proposal = proposePlanFor(t, h, "2026-11-02", `{"from": {"on": "2026-11-05", "kind": "breakfast"}, "prep_sessions": []}`)
	logException(t, h, upcoming, "2026-11-10", "lunch", map[string]string{"exception": "skipped"}, false)
	if page := h.Get(t, proposal).Body; !strings.Contains(text(page), "pending") || !strings.Contains(page, `id="approve"`) {
		t.Errorf("an Exception on the upcoming Cycle replaced the current one's Plan Proposal:\n%s", text(page))
	}
}

func TestAPastSlotIsCorrectedAfterItHappenedAndThePlanReplanned(t *testing.T) {
	h, today := startPlanning(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	approvePlan(t, h, lemonPlan(t, h))
	logException(t, h, exceptions, "2026-11-06", "dinner", map[string]string{"exception": "meal-out"}, false)
	// Replanned around the Meal Out.
	approvePlan(t, h, lemonPlan(t, h))
	pending := proposePlan(t, h, `{"prep_sessions": []}`)

	// The dinner out fell through: the morning after, it was eaten at home.
	today.setAt(t, "2026-11-07 09:00")
	res := logException(t, h, exceptions, "2026-11-06", "dinner", map[string]string{"exception": ""}, true)
	if res.StatusCode != http.StatusOK || !isRow(res.Body) || !strings.Contains(text(res.Body), "dinner from the Plan") {
		t.Fatalf("HTMX correct a past Slot = %d, want 200 and its row from the Plan:\n%s", res.StatusCode, res.Body)
	}
	// What was eaten may have been meant for a Slot still to come.
	if page := text(h.Get(t, "/plan").Body); !strings.Contains(page, "An Exception changed: this Plan needs a new Plan Proposal.") {
		t.Errorf("Plan page does not say the Plan needs a new Plan Proposal:\n%s", page)
	}
	if page := text(h.Get(t, pending).Body); !strings.Contains(page, "Replaced by the Exception on Fri 6 Nov, dinner") {
		t.Errorf("the pending Plan Proposal was not replaced by the Exception:\n%s", page)
	}
}

func TestASlotNoLongerInTheCycleIsNotLogged(t *testing.T) {
	h, _ := startPlanning(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	breakfast := apptest.Form(t, h.Get(t, exceptions).Body, "exception-2026-11-02-breakfast")
	breakfast.Values.Set("exception", "skipped")
	changeDelivery(t, h, map[string]string{"delivery_on": "2026-11-04"})

	res := h.PostForm(t, breakfast.Action, breakfast.Values)
	if res.StatusCode != http.StatusConflict || !strings.Contains(text(res.Body), "That Slot is no longer one of this Cycle's") {
		t.Fatalf("log a Slot the Cycle no longer holds = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	res = h.HTMXPostForm(t, breakfast.Action, breakfast.Values)
	if res.StatusCode != http.StatusConflict || !isRow(res.Body) || strings.Contains(res.Body, "<form") || !strings.Contains(text(res.Body), "That Slot is no longer one of this Cycle's") {
		t.Fatalf("HTMX log a Slot the Cycle no longer holds = %d, want 409 and its row, without a form, saying why:\n%s", res.StatusCode, res.Body)
	}
	if page := text(h.Get(t, "/plan").Body); strings.Contains(page, "Exceptions between Cycles") {
		t.Errorf("a refused Exception was logged:\n%s", page)
	}
}

func TestAClosedCyclesSlotIsCorrectedAndTheCurrentPlanReplanned(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	today.set(t, "2026-11-04")
	openCycle(t, h, "2026-11-07", "")
	approvePlan(t, h, `{"prep_sessions": []}`)
	// Saturday's Delivery arrived, closing the Cycle that holds Friday.
	today.setAt(t, "2026-11-07 09:00")

	previous := apptest.Link(t, h.Get(t, "/plan").Body, "Exceptions of the previous Cycle")
	res := logException(t, h, previous, "2026-11-06", "dinner", map[string]string{"exception": "skipped"}, false)
	if res.StatusCode != http.StatusOK || !strings.Contains(text(res.Body), "dinner skipped") {
		t.Fatalf("correct a closed Cycle's Slot = %d, want its Exceptions page showing it:\n%s", res.StatusCode, text(res.Body))
	}
	if page := text(h.Get(t, "/plan").Body); !strings.Contains(page, "An Exception changed: this Plan needs a new Plan Proposal.") {
		t.Errorf("the current Plan does not need a new Plan Proposal:\n%s", page)
	}
}

func TestMealOutEstimateIsCheckedAndKeptAsTyped(t *testing.T) {
	h, _ := startPlanning(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	fields := map[string]string{"exception": "meal-out", "estimate_kcal": "-5", "estimate_protein_g": "lots", "estimate_carbs_g": "3000000000"}

	res := logException(t, h, exceptions, "2026-11-07", "lunch", fields, true)
	if res.StatusCode != http.StatusUnprocessableEntity || !isRow(res.Body) || !strings.Contains(text(res.Body), "Can't be negative") || !strings.Contains(text(res.Body), "Must be a whole number") || !strings.Contains(text(res.Body), "Too large") {
		t.Fatalf("HTMX invalid estimate = %d, want 422 and its row saying why:\n%s", res.StatusCode, res.Body)
	}
	if !hasField(res.Body, "estimate_kcal", "-5") || !hasField(res.Body, "estimate_protein_g", "lots") {
		t.Errorf("rejected row does not keep what was typed:\n%s", res.Body)
	}

	res = logException(t, h, exceptions, "2026-11-07", "lunch", fields, false)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Can't be negative") || !hasField(res.Body, "estimate_kcal", "-5") {
		t.Fatalf("invalid estimate = %d, want 422 and the page keeping what was typed:\n%s", res.StatusCode, text(res.Body))
	}
	if page := text(h.Get(t, exceptions).Body); strings.Contains(page, "Meal Out ·") {
		t.Errorf("an invalid Meal Out was logged:\n%s", page)
	}
}

func TestExceptionIsChangedAndRemoved(t *testing.T) {
	h, _ := startPlanning(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	logException(t, h, exceptions, "2026-11-07", "lunch", map[string]string{"exception": "skipped"}, false)

	res := logException(t, h, exceptions, "2026-11-07", "lunch", map[string]string{"exception": "meal-out", "estimate_kcal": "1200", "estimate_protein_g": "60"}, false)
	if page := text(res.Body); !strings.Contains(page, "lunch Meal Out · 1200 kcal · 60 g protein") {
		t.Errorf("the skipped Slot did not become a Meal Out with its own estimate:\n%s", page)
	}
	if !hasField(res.Body, "estimate_kcal", "1200") || !hasField(res.Body, "estimate_carbs_g", "") {
		t.Errorf("the Meal Out's form does not show its own estimate and blanks for the default:\n%s", res.Body)
	}

	res = logException(t, h, exceptions, "2026-11-07", "lunch", map[string]string{"exception": ""}, false)
	if page := text(res.Body); strings.Contains(page, "Meal Out ·") || strings.Contains(page, "lunch skipped") {
		t.Errorf("the Exception was not removed:\n%s", page)
	}
}

func TestAnExceptedSlotBeforeAPlanProposalCountsOnlyItsException(t *testing.T) {
	h, today := startPlanning(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	approvePlan(t, h, lemonPlan(t, h))
	// Wednesday's lunch, the Lemon chicken with rice, is skipped after all.
	today.set(t, "2026-11-04")
	logException(t, h, exceptions, "2026-11-04", "lunch", map[string]string{"exception": "skipped"}, false)

	today.set(t, "2026-11-05")
	page := text(h.Get(t, proposePlan(t, h, `{"from": {"on": "2026-11-05", "kind": "breakfast"}, "prep_sessions": []}`)).Body)
	// Seven Untracked Allowances, Monday's Lemon chicken with rice and
	// Tuesday's lemon: 1050 + 589 + 20 kcal, 35 + 52 + 1 g protein. The
	// skipped lunch's Portion counts nothing.
	if want := "Sun 8 Nov 150 kcal · 5 g protein · 15 g carbs · 5 g fat Seven days: 1659 kcal · 88 g protein"; !strings.Contains(page, want) {
		t.Errorf("Plan Proposal page does not show %q:\n%s", want, page)
	}
}

func TestPlanProposalFillingAnExceptedSlotIsNotApproved(t *testing.T) {
	h, _ := startPlanning(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	logException(t, h, exceptions, "2026-11-03", "snack", map[string]string{"exception": "skipped"}, false)
	// Stored without the planner, which would have seen the skipped snack.
	proposal := proposePlan(t, h, lemonPlan(t, h))

	approve := apptest.Form(t, h.Get(t, proposal).Body, "approve")
	res := h.PostForm(t, approve.Action, approve.Values)
	if res.StatusCode != http.StatusConflict || !strings.Contains(text(res.Body), "fills a Slot logged as a Meal Out or skipped, so it was not approved") {
		t.Fatalf("approve a Plan Proposal filling an excepted Slot = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "No Plan yet") {
		t.Errorf("a Plan Proposal filling an excepted Slot became the Plan:\n%s", plan)
	}
}

func TestAnExceptionBetweenCyclesIsShownAndRemoved(t *testing.T) {
	h, _, llm := planningRun(t)
	logException(t, h, openCycle(t, h, "2026-11-02", "").Path, "2026-11-07", "lunch", map[string]string{"exception": "meal-out"}, false)
	if page := text(h.Get(t, "/plan").Body); strings.Contains(page, "Exceptions between Cycles") {
		t.Fatalf("the Plan page shows an Exception its Cycle holds as between Cycles:\n%s", page)
	}

	drop := apptest.Form(t, h.Get(t, "/plan").Body, "drop")
	page := h.PostForm(t, drop.Action, drop.Values).Body
	if want := "count them. lunch on Sat 7 Nov Meal Out · 900 kcal · 35 g protein Remove"; !strings.Contains(text(page), want) {
		t.Fatalf("Plan page does not show %q:\n%s", want, text(page))
	}
	remove := apptest.Form(t, page, "remove-exception-2026-11-07-lunch")
	res := h.PostForm(t, remove.Action, remove.Values)
	if res.StatusCode != http.StatusOK || res.Path != "/plan" || strings.Contains(text(res.Body), "Exceptions between Cycles") {
		t.Fatalf("remove an Exception between Cycles = %d at %s, want the Plan page without it:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	openCycle(t, h, "2026-11-09", "2026-11-12")
	seesExceptions(t, h, llm, "No Exceptions from Tue 3 Nov to Thu 12 Nov.")
}

func TestAMealOutOverAPlannedSlotCountsInsteadOfItsPortion(t *testing.T) {
	h, _ := startPlanning(t)
	exceptions := openCycle(t, h, "2026-11-02", "").Path
	approvePlan(t, h, lemonPlan(t, h))
	logException(t, h, exceptions, "2026-11-07", "dinner", map[string]string{"exception": "meal-out"}, false)

	// The Untracked Allowance and the Meal Out, not the frozen Lemon chicken
	// with rice: 150 + 900 kcal, 5 + 35 g protein, 15 + 90 g carbs, 5 + 40 g
	// fat.
	if page, want := text(h.Get(t, "/plan").Body), "Sat 7 Nov 1050 kcal · 40 g protein · 105 g carbs · 45 g fat dinner Meal Out, not Lemon chicken with rice Sun 8 Nov"; !strings.Contains(page, want) {
		t.Errorf("Plan page does not show %q:\n%s", want, page)
	}
}

func TestWithNoCycleUpcomingThePlanningRunSeesTheCurrentOnesExceptions(t *testing.T) {
	h, today, llm := planningRun(t)
	logException(t, h, openCycle(t, h, "2026-11-02", "").Path, "2026-11-07", "lunch", map[string]string{"exception": "skipped"}, false)
	// Its Delivery has arrived: the Cycle is current, and none is upcoming.
	today.set(t, "2026-11-03")
	seesExceptions(t, h, llm, "Exceptions from Tue 27 Oct to Thu 12 Nov: - lunch on Sat 7 Nov: skipped, counts nothing.")
}

// dayLine is the markup of the day labelled date on an Exceptions page.
func dayLine(t *testing.T, page, date string) string {
	t.Helper()
	i := strings.Index(page, "<h3>"+date+"</h3>")
	if i < 0 {
		t.Fatalf("Exceptions page has no day %s:\n%s", date, text(page))
	}
	line := page[i+len("<h3>"):]
	if j := strings.Index(line, "<h3>"); j >= 0 {
		line = line[:j]
	}
	return line
}

// On desktop, a day's Slots are one line with a column per Slot kind. See
// #69.
func TestExceptionsPageKeepsEachSlotInItsColumnAndOneOpenADay(t *testing.T) {
	h, _ := startPlanning(t)
	// Delivered before dinner: Monday holds dinner alone, in the last
	// column, after empty cells for the Slots before it.
	res := openCycleIn(t, h, "2026-11-02", "17:30", "19:00", "")
	if monday := dayLine(t, res.Body, "Mon 2 Nov"); strings.Count(monday, `class="slot-gap"`) != 3 || !strings.Contains(monday, `id="exception-2026-11-02-dinner"`) {
		t.Errorf("Monday is not three empty cells and dinner:\n%s", monday)
	}
	if tuesday := dayLine(t, res.Body, "Tue 3 Nov"); strings.Contains(tuesday, "slot-gap") {
		t.Errorf("a full day has empty cells:\n%s", tuesday)
	}
	// Opening a Slot closes the day's open one.
	if got := strings.Count(res.Body, `<details name="slots-2026-11-03"`); got != 4 {
		t.Errorf("Tuesday has %d Slots opening one at a time, want 4", got)
	}
	res = logException(t, h, res.Path, "2026-11-03", "lunch", map[string]string{"exception": "skipped"}, true)
	if !isRow(res.Body) || !strings.Contains(res.Body, `<details name="slots-2026-11-03"`) {
		t.Errorf("HTMX row does not stay one of its day's Slots:\n%s", res.Body)
	}
}
