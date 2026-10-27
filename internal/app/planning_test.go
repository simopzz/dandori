package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// startPlanning starts the app with the Lemon chicken with rice Recipe in
// the Recipe Library and a Profile of 2200 kcal and 150 g protein a day,
// with an Untracked Allowance of 150 kcal and 5 g, and an allergy to
// citrus.
func startPlanning(t *testing.T) *apptest.Harness {
	t.Helper()
	h := startStock(t)
	profile := validProfile()
	profile.Set("allergies", "citrus")
	if res := h.PostForm(t, "/profile", profile); res.StatusCode != http.StatusOK {
		t.Fatalf("save Profile = %d:\n%s", res.StatusCode, text(res.Body))
	}
	return h
}

// openCycle opens a Cycle through the Plan page's form.
func openCycle(t *testing.T, h *apptest.Harness, delivery, ends string) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, "/plan").Body, "open-cycle")
	form.Values.Set("delivery_on", delivery)
	form.Values.Set("ends_on", ends)
	return h.PostForm(t, form.Action, form.Values)
}

// lemonPlan is a Plan for a Cycle delivered on 2026-11-02: Lemon chicken
// with rice cooked for dinner that day, eaten then, at lunch on the 4th and
// frozen for dinner on the 7th, and a lemon eaten as it is for a snack on
// the 3rd.
func lemonPlan(t *testing.T, h *apptest.Harness) string {
	t.Helper()
	recipe := strings.TrimPrefix(libraryRecipe(t, h), "/recipes/")
	return fmt.Sprintf(`{"prep_sessions": [
		{"on": "2026-11-02", "kind": "dinner", "recipe_id": %s, "portions": [
			{"location": "fridge", "eat_on": "2026-11-02", "eat_kind": "dinner"},
			{"location": "fridge", "eat_on": "2026-11-04", "eat_kind": "lunch"},
			{"location": "freezer", "eat_on": "2026-11-07", "eat_kind": "dinner"}]},
		{"on": "2026-11-03", "kind": "snack", "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-03", "eat_kind": "snack"}]}
	]}`, recipe)
}

// proposePlan stores plan as an initial Plan Proposal for the latest
// Cycle, as the planner will, and returns the path of its page.
func proposePlan(t *testing.T, h *apptest.Harness, plan string) string {
	t.Helper()
	before := proposalPaths(t, h)
	h.Exec(t, `
		INSERT INTO plan_proposal (cycle_id, origin, plan)
		SELECT id, 'initial', $1 FROM cycle ORDER BY delivery_on DESC LIMIT 1`, plan)
	after := proposalPaths(t, h)
	if len(after) != len(before)+1 {
		t.Fatalf("the Plan page lists %d Plan Proposals, want %d", len(after), len(before)+1)
	}
	return after[0]
}

// proposalPaths are the Plan Proposals the Plan page links, newest first.
func proposalPaths(t *testing.T, h *apptest.Harness) []string {
	t.Helper()
	var paths []string
	page := h.Get(t, "/plan").Body
	for {
		i := strings.Index(page, `href="/plan-proposals/`)
		if i < 0 {
			return paths
		}
		page = page[i+len(`href="`):]
		paths = append(paths, page[:strings.Index(page, `"`)])
	}
}

func TestCycleIsOpenedWithADeliveryAndEndDate(t *testing.T) {
	h := startPlanning(t)
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "No Cycle yet.") {
		t.Errorf("Plan page does not say there is no Cycle:\n%s", plan)
	}

	res := openCycle(t, h, "2026-11-02", "2026-11-09")
	if res.StatusCode != http.StatusOK || res.Path != "/plan" || res.Method != http.MethodGet {
		t.Fatalf("open Cycle = %d %s %s, want the Plan page after a redirect:\n%s", res.StatusCode, res.Method, res.Path, text(res.Body))
	}
	if plan := text(res.Body); !strings.Contains(plan, "Delivered Mon 2 Nov 2026 Ends Mon 9 Nov 2026") {
		t.Errorf("Plan page does not show the Cycle:\n%s", plan)
	}
}

func TestCycleEndsTenDaysAfterDeliveryByDefault(t *testing.T) {
	h := startPlanning(t)
	if plan := text(openCycle(t, h, "2026-11-02", "").Body); !strings.Contains(plan, "Delivered Mon 2 Nov 2026 Ends Thu 12 Nov 2026") {
		t.Errorf("Plan page does not show the Cycle ending ten days after delivery:\n%s", plan)
	}
}

func TestInvalidCycleIsRejected(t *testing.T) {
	tests := []struct {
		name, delivery, ends, message string
	}{
		{"no delivery date", "", "2026-11-12", "Required"},
		{"delivery not a date", "Monday", "", "Must be a date"},
		{"end not a date", "2026-11-02", "soon", "Must be a date"},
		{"ending before delivery", "2026-11-02", "2026-11-01", "Must be after the delivery date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := startPlanning(t)
			res := openCycle(t, h, tt.delivery, tt.ends)
			if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), tt.message) {
				t.Fatalf("open Cycle = %d, want 422 saying %q:\n%s", res.StatusCode, tt.message, text(res.Body))
			}
			if !hasField(res.Body, "delivery_on", tt.delivery) || !hasField(res.Body, "ends_on", tt.ends) {
				t.Errorf("rejected form does not keep what was typed:\n%s", res.Body)
			}
			if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "No Cycle yet.") {
				t.Errorf("a rejected Cycle was opened:\n%s", plan)
			}
		})
	}
}

func TestCycleDeliveredOnOrBeforeTheCurrentOneIsRejected(t *testing.T) {
	h := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")

	res := openCycle(t, h, "2026-11-02", "")
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Must be after the current Cycle's delivery on Mon 2 Nov 2026") {
		t.Fatalf("open an overlapping Cycle = %d, want 422 saying why:\n%s", res.StatusCode, text(res.Body))
	}
}

func TestPlanProposalShowsSlotsPrepSessionsNutritionAndBrokenRules(t *testing.T) {
	h := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	// The user confirmed a lemon is citrus, which they are allergic to.
	h.Exec(t, `INSERT INTO ingredient_falls_under (ingredient_id, term) SELECT id, 'citrus' FROM ingredient WHERE name = 'lemon'`)
	proposal := proposePlan(t, h, lemonPlan(t, h))

	res := h.Get(t, proposal)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", proposal, res.StatusCode)
	}
	page := text(res.Body)
	// A Portion of Lemon chicken with rice is 589 kcal, 52 g protein, 64 g
	// carbs and 13 g fat; the lemon 20 kcal, 1 g, 6 g and 0 g. Each day adds
	// the Untracked Allowance: 150 kcal, 5 g, 15 g, 5 g.
	for _, want := range []string{
		"Initial Plan Proposal for the Cycle delivered Mon 2 Nov 2026",
		"pending",
		// The calendar of Slots, with each day's nutrition.
		"Mon 2 Nov 739 kcal · 57 g protein · 79 g carbs · 18 g fat",
		"dinner Lemon chicken with rice",
		"Tue 3 Nov 170 kcal · 6 g protein · 21 g carbs · 5 g fat",
		"snack lemon",
		"Wed 4 Nov 739 kcal",
		"lunch Lemon chicken with rice",
		"Thu 5 Nov 150 kcal",
		// The first trailing seven days, 2–8 Nov: 3×589 + 20 + 7×150 kcal.
		"Sun 8 Nov 150 kcal · 5 g protein · 15 g carbs · 5 g fat Seven days: 2837 kcal · 192 g protein",
		// The Prep Sessions.
		"Lemon chicken with rice dinner on Mon 2 Nov 3 Portions fridge, fridge, freezer",
		"lemon snack on Tue 3 Nov 1 Portion fridge",
		// The rules it breaks.
		"The seven days to Sun 8 Nov eat 2837 kcal, outside 14630–16170",
		"The seven days to Sun 8 Nov eat 192 g protein, under 1050",
		"Lemon chicken with rice at dinner on Mon 2 Nov has lemon, which is citrus: an allergy",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Plan Proposal page does not show %q:\n%s", want, page)
		}
	}
	// Only the seven days to 8–12 Nov are on or after the first delivery.
	if got := strings.Count(page, "Seven days"); got != 5 {
		t.Errorf("Plan Proposal page shows %d trailing seven days, want 5 (to 8–12 Nov):\n%s", got, page)
	}
}

func TestApprovedPlanProposalBecomesTheCyclesPlan(t *testing.T) {
	h := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	proposal := proposePlan(t, h, lemonPlan(t, h))
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "No Plan yet") {
		t.Errorf("Plan page shows a Plan before any Plan Proposal is approved:\n%s", plan)
	}

	approve := apptest.Form(t, h.Get(t, proposal).Body, "approve")
	res := h.PostForm(t, approve.Action, approve.Values)
	if res.StatusCode != http.StatusOK || res.Path != "/plan" {
		t.Fatalf("approve = %d at %s, want the Plan page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	plan := text(res.Body)
	for _, want := range []string{"Mon 2 Nov 739 kcal", "dinner Lemon chicken with rice", "snack lemon", "approved"} {
		if !strings.Contains(plan, want) {
			t.Errorf("Plan page does not show the approved Plan's %q:\n%s", want, plan)
		}
	}
	if page := text(h.Get(t, proposal).Body); !strings.Contains(page, "approved") || strings.Contains(page, "Approve") {
		t.Errorf("an approved Plan Proposal is still offered for approval:\n%s", page)
	}
	if res := h.PostForm(t, approve.Action, approve.Values); res.StatusCode != http.StatusConflict {
		t.Errorf("approve twice = %d, want 409", res.StatusCode)
	}
}

func TestApprovalReplacesThePlanAtomically(t *testing.T) {
	h := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	first := proposePlan(t, h, lemonPlan(t, h))
	approve := apptest.Form(t, h.Get(t, first).Body, "approve")
	h.PostForm(t, approve.Action, approve.Values)

	// A Plan Proposal that can't be applied: it puts a Portion that isn't
	// kept in a Slot, after a Prep Session that can.
	broken := proposePlan(t, h, `{"prep_sessions": [
		{"on": "2026-11-05", "kind": "snack", "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-05", "eat_kind": "snack"}]}],
		"kept": [{"portion_id": 999999, "on": "2026-11-06", "kind": "lunch"}]}`)
	if page := text(h.Get(t, broken).Body); !strings.Contains(page, "is not a Portion without a Slot") {
		t.Errorf("Plan Proposal page does not show the unknown Portion:\n%s", page)
	}
	approve = apptest.Form(t, h.Get(t, broken).Body, "approve")
	if res := h.PostForm(t, approve.Action, approve.Values); res.StatusCode != http.StatusConflict {
		t.Fatalf("approve an inapplicable Plan Proposal = %d, want 409:\n%s", res.StatusCode, text(res.Body))
	}
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "dinner Lemon chicken with rice") || strings.Contains(plan, "Thu 5 Nov 170 kcal") {
		t.Errorf("a failed approval changed the Plan:\n%s", plan)
	}

	second := proposePlan(t, h, `{"prep_sessions": [
		{"on": "2026-11-05", "kind": "snack", "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-05", "eat_kind": "snack"}]}]}`)
	approve = apptest.Form(t, h.Get(t, second).Body, "approve")
	plan := text(h.PostForm(t, approve.Action, approve.Values).Body)
	if strings.Contains(plan, "Lemon chicken with rice") || !strings.Contains(plan, "Thu 5 Nov 170 kcal") {
		t.Errorf("the approved Plan Proposal did not replace the Plan:\n%s", plan)
	}
}

func TestRejectedPlanProposalKeepsTheNote(t *testing.T) {
	h := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	proposal := proposePlan(t, h, lemonPlan(t, h))

	reject := apptest.Form(t, h.Get(t, proposal).Body, "reject")
	reject.Values.Set("note", "less chicken, no oven on Tuesday")
	res := h.PostForm(t, reject.Action, reject.Values)
	if res.StatusCode != http.StatusOK || res.Path != proposal {
		t.Fatalf("reject = %d at %s, want the Plan Proposal page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	page := text(res.Body)
	if !strings.Contains(page, "rejected") || !strings.Contains(page, "less chicken, no oven on Tuesday") {
		t.Errorf("rejected Plan Proposal does not show its note:\n%s", page)
	}
	approve := url.Values{}
	if res := h.PostForm(t, proposal+"/approve", approve); res.StatusCode != http.StatusConflict {
		t.Errorf("approve a rejected Plan Proposal = %d, want 409", res.StatusCode)
	}
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "No Plan yet") {
		t.Errorf("a rejected Plan Proposal became the Plan:\n%s", plan)
	}
}

func TestDeliveryBeforeTheCycleEndsClosesItAndCarriesOverLeftovers(t *testing.T) {
	h := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	plan := strings.Replace(lemonPlan(t, h), `]}
	]}`, `]},
		{"on": "2026-11-06", "kind": "snack", "ingredients": [{"name": "lemon", "quantity": 2}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "snack"},
			{"location": "fridge", "eat_on": "2026-11-14", "eat_kind": "snack"}]}
	]}`, 1)
	proposal := proposePlan(t, h, plan)
	approve := apptest.Form(t, h.Get(t, proposal).Body, "approve")
	h.PostForm(t, approve.Action, approve.Values)

	res := openCycle(t, h, "2026-11-05", "")
	if res.StatusCode != http.StatusOK || res.Path != "/plan" {
		t.Fatalf("open Cycle = %d at %s, want the Plan page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	if page := text(res.Body); !strings.Contains(page, "Delivered Thu 5 Nov 2026 Ends Sun 15 Nov 2026") || !strings.Contains(page, "No Plan yet") {
		t.Errorf("Plan page does not show the new Cycle without a Plan:\n%s", page)
	}
	// The Lemon chicken with rice to eat on the 7th was cooked before the
	// delivery: it carries over. The lemons of the 6th were never set out.
	stock := text(h.Get(t, "/stock").Body)
	if strings.Count(stock, "Lemon chicken with rice freezer") != 1 || len(portionIDs(t, h)) != 1 {
		t.Errorf("Stock page does not show exactly the carried-over Portion:\n%s", stock)
	}
}

func TestPlanProposalOfAClosedCycleIsNotApproved(t *testing.T) {
	h := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	first := proposePlan(t, h, lemonPlan(t, h))
	approve := apptest.Form(t, h.Get(t, first).Body, "approve")
	h.PostForm(t, approve.Action, approve.Values)
	stale := proposePlan(t, h, lemonPlan(t, h))
	staleApprove := apptest.Form(t, h.Get(t, stale).Body, "approve")
	openCycle(t, h, "2026-11-05", "")

	res := h.PostForm(t, staleApprove.Action, staleApprove.Values)
	if res.StatusCode != http.StatusConflict || !strings.Contains(res.Body, "Cycle has closed") {
		t.Fatalf("approve a Plan Proposal of a closed Cycle = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "Lemon chicken with rice freezer") {
		t.Errorf("approving a closed Cycle's Plan Proposal lost the carried-over Portion:\n%s", stock)
	}
}

func TestPlanIsLinkedFromTheAppShell(t *testing.T) {
	h := apptest.Start(t)
	if page := h.Get(t, "/").Body; !strings.Contains(page, `href="/plan"`) {
		t.Errorf("app shell does not link to the Plan page:\n%s", page)
	}
}

func TestUnknownPlanProposalIsNotFound(t *testing.T) {
	h := startPlanning(t)
	if res := h.Get(t, "/plan-proposals/999999"); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET an unknown Plan Proposal = %d, want 404", res.StatusCode)
	}
}
