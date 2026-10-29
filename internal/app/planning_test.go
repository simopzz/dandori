package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
)

// clock is the app's clock in a test; set it to the day it is.
type clock struct{ now time.Time }

// set sets the clock to noon on day.
func (c *clock) set(t *testing.T, day string) {
	t.Helper()
	c.setAt(t, day+" 12:00")
}

// setAt sets the clock to moment, as YYYY-MM-DD HH:MM.
func (c *clock) setAt(t *testing.T, moment string) {
	t.Helper()
	d, err := time.Parse(time.DateOnly+" 15:04", moment)
	if err != nil {
		t.Fatal(err)
	}
	c.now = d
}

// startPlanning starts the app with the Lemon chicken with rice Recipe in
// the Recipe Library and a Profile of 2200 kcal and 150 g protein a day,
// with an Untracked Allowance of 150 kcal and 5 g, and an allergy to
// citrus. It is 2026-11-01 until the test sets the clock.
func startPlanning(t *testing.T) (*apptest.Harness, *clock) {
	t.Helper()
	h, today, _ := startPlanningWith(t)
	return h, today
}

// startPlanningWith is startPlanning with opts, returning the fake LLM
// too.
func startPlanningWith(t *testing.T, opts ...apptest.Option) (*apptest.Harness, *clock, *fakellm.LLM) {
	t.Helper()
	h, llm := startRecipes(t)
	acceptFirstRecipe(t, h, llm)
	today := &clock{}
	today.set(t, "2026-11-01")
	h.Restart(t, append(opts, apptest.WithConfig(func(c *app.Config) { c.Now = func() time.Time { return today.now } }))...)
	profile := validProfile()
	profile.Set("allergies", "citrus")
	if res := h.PostForm(t, "/profile", profile); res.StatusCode != http.StatusOK {
		t.Fatalf("save Profile = %d:\n%s", res.StatusCode, text(res.Body))
	}
	return h, today, llm
}

// openCycle opens a Cycle through the Plan page's form, its Delivery
// arriving before breakfast.
func openCycle(t *testing.T, h *apptest.Harness, delivery, ends string) apptest.Response {
	t.Helper()
	return openCycleIn(t, h, delivery, "07:00", "07:30", ends)
}

// openCycleIn opens a Cycle delivered in the window from–to.
func openCycleIn(t *testing.T, h *apptest.Harness, delivery, from, to, ends string) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, "/plan").Body, "open-cycle")
	form.Values.Set("delivery_on", delivery)
	form.Values.Set("window_from", from)
	form.Values.Set("window_to", to)
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
	return proposePlanFor(t, h, "", plan)
}

// proposePlanFor stores plan as an initial Plan Proposal for the Cycle
// delivered on delivery, or the latest one when delivery is empty.
func proposePlanFor(t *testing.T, h *apptest.Harness, delivery, plan string) string {
	t.Helper()
	before := proposalPaths(t, h)
	h.Exec(t, `
		INSERT INTO plan_proposal (cycle_id, origin, plan)
		SELECT id, 'initial', $1 FROM cycle
		WHERE $2 = '' OR delivery_on = $2::text::date
		ORDER BY delivery_on DESC LIMIT 1`, plan, delivery)
	after := proposalPaths(t, h)
	if len(after) != len(before)+1 {
		t.Fatalf("the Plan page lists %d Plan Proposals, want %d", len(after), len(before)+1)
	}
	for _, p := range after {
		if !slices.Contains(before, p) {
			return p
		}
	}
	panic("unreachable")
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
	h, _ := startPlanning(t)
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "No Cycle yet.") {
		t.Errorf("Plan page does not say there is no Cycle:\n%s", plan)
	}

	res := openCycle(t, h, "2026-11-02", "2026-11-09")
	if res.StatusCode != http.StatusOK || !strings.HasSuffix(res.Path, "/exceptions") || res.Method != http.MethodGet {
		t.Fatalf("open Cycle = %d %s %s, want its Exceptions page after a redirect:\n%s", res.StatusCode, res.Method, res.Path, text(res.Body))
	}
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "Upcoming Cycle Delivery Mon 2 Nov 2026, 07:00–07:30 tentative Starts breakfast on Mon 2 Nov Ends Mon 9 Nov 2026") {
		t.Errorf("Plan page does not show the Cycle:\n%s", plan)
	}
}

func TestCycleEndsTenDaysAfterDeliveryByDefault(t *testing.T) {
	h, _ := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "Ends Thu 12 Nov 2026") {
		t.Errorf("Plan page does not show the Cycle ending ten days after delivery:\n%s", plan)
	}
}

func TestInvalidCycleIsRejected(t *testing.T) {
	tests := []struct {
		name, delivery, from, to, ends, message string
	}{
		{"no delivery date", "", "18:00", "20:00", "2026-11-12", "Required"},
		{"delivery not a date", "Monday", "18:00", "20:00", "", "Must be a date"},
		{"end not a date", "2026-11-02", "18:00", "20:00", "soon", "Must be a date"},
		{"ending before delivery", "2026-11-02", "18:00", "20:00", "2026-11-01", "Must be after the delivery date"},
		{"no window", "2026-11-02", "", "20:00", "", "Required"},
		{"window not a time", "2026-11-02", "18:00", "evening", "", "Must be a time"},
		{"window ending before it starts", "2026-11-02", "18:00", "17:00", "", "Must be after the start of the window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := startPlanning(t)
			res := openCycleIn(t, h, tt.delivery, tt.from, tt.to, tt.ends)
			if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), tt.message) {
				t.Fatalf("open Cycle = %d, want 422 saying %q:\n%s", res.StatusCode, tt.message, text(res.Body))
			}
			if !hasField(res.Body, "delivery_on", tt.delivery) || !hasField(res.Body, "window_to", tt.to) || !hasField(res.Body, "ends_on", tt.ends) {
				t.Errorf("rejected form does not keep what was typed:\n%s", res.Body)
			}
			if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "No Cycle yet.") {
				t.Errorf("a rejected Cycle was opened:\n%s", plan)
			}
		})
	}
}

func TestCycleDeliveredOnOrBeforeTheCurrentOneIsRejected(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	today.set(t, "2026-11-03")

	res := openCycle(t, h, "2026-11-02", "")
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Must be after the current Cycle's delivery on Mon 2 Nov 2026") {
		t.Fatalf("open an overlapping Cycle = %d, want 422 saying why:\n%s", res.StatusCode, text(res.Body))
	}
}

func TestPlanProposalShowsSlotsPrepSessionsNutritionAndBrokenRules(t *testing.T) {
	h, _ := startPlanning(t)
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
	h, _ := startPlanning(t)
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
	h, _ := startPlanning(t)
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
	h, _ := startPlanning(t)
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

func TestOpeningACycleLeavesTheCurrentOneUntilItsDeliveryArrives(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonsPlan(t, h))

	today.set(t, "2026-11-04")
	if res := openCycleIn(t, h, "2026-11-05", "10:00", "12:00", ""); res.StatusCode != http.StatusOK {
		t.Fatalf("open Cycle = %d:\n%s", res.StatusCode, text(res.Body))
	}
	page := text(h.Get(t, "/plan").Body)
	for _, want := range []string{
		"01 Cycle Delivery Mon 2 Nov 2026, 07:00–07:30 tentative Starts breakfast on Mon 2 Nov Ends Thu 12 Nov 2026",
		"dinner Lemon chicken with rice",
		// The Delivery ends before lunch: lunch is the new Cycle's first Slot.
		"04 Upcoming Cycle Delivery Thu 5 Nov 2026, 10:00–12:00 tentative Starts lunch on Thu 5 Nov Ends Sun 15 Nov 2026",
		"Upcoming Plan No Plan yet",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Plan page does not show %q:\n%s", want, page)
		}
	}
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "Lemon chicken with rice freezer for dinner on Sat 7 Nov") {
		t.Errorf("opening a Cycle carried over the current one's Portions:\n%s", stock)
	}

	today.setAt(t, "2026-11-05 11:59")
	if page := text(h.Get(t, "/plan").Body); !strings.Contains(page, "Upcoming Cycle") {
		t.Errorf("the Cycle is current before its Delivery window ends:\n%s", page)
	}

	// The Lemon chicken with rice to eat on the 7th was cooked before the
	// Delivery: it carries over. The lemons of the 6th were never set out.
	today.setAt(t, "2026-11-05 12:00")
	stock := text(h.Get(t, "/stock").Body)
	if strings.Count(stock, "Lemon chicken with rice freezer") != 1 || strings.Contains(stock, "Sat 7 Nov") || len(portionIDs(t, h)) != 1 {
		t.Errorf("Stock page does not show exactly the carried-over Portion, without a Slot:\n%s", stock)
	}
	page = text(h.Get(t, "/plan").Body)
	if !strings.Contains(page, "01 Cycle Delivery Thu 5 Nov 2026") || strings.Contains(page, "Upcoming Cycle") || strings.Contains(page, "Mon 2 Nov 2026") {
		t.Errorf("Plan page does not show the arrived Cycle as the current one:\n%s", page)
	}
}

func TestPlanProposalOfAClosedCycleIsNotApproved(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	first := proposePlan(t, h, lemonPlan(t, h))
	approve := apptest.Form(t, h.Get(t, first).Body, "approve")
	h.PostForm(t, approve.Action, approve.Values)
	stale := proposePlan(t, h, lemonPlan(t, h))
	staleApprove := apptest.Form(t, h.Get(t, stale).Body, "approve")
	today.set(t, "2026-11-05")
	openCycle(t, h, "2026-11-05", "")

	page := text(h.Get(t, stale).Body)
	if !strings.Contains(page, "rejected") || !strings.Contains(page, "Note: Its Cycle closed: the next Delivery arrived on Thu 5 Nov.") {
		t.Errorf("a closed Cycle's pending Plan Proposal was not rejected, saying why:\n%s", page)
	}
	res := h.PostForm(t, staleApprove.Action, staleApprove.Values)
	if res.StatusCode != http.StatusConflict || !strings.Contains(res.Body, "already decided") {
		t.Fatalf("approve a Plan Proposal of a closed Cycle = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "Lemon chicken with rice freezer") {
		t.Errorf("approving a closed Cycle's Plan Proposal lost the carried-over Portion:\n%s", stock)
	}
}

// lemonsPlan is lemonPlan with two lemons set out for a snack on the 6th:
// one eaten then, the other on the 14th, after the Cycle.
func lemonsPlan(t *testing.T, h *apptest.Harness) string {
	t.Helper()
	return strings.Replace(lemonPlan(t, h), `]}
	]}`, `]},
		{"on": "2026-11-06", "kind": "snack", "ingredients": [{"name": "lemon", "quantity": 2}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "snack"},
			{"location": "fridge", "eat_on": "2026-11-14", "eat_kind": "snack"}]}
	]}`, 1)
}

// approvePlan proposes plan for the latest Cycle and approves it.
func approvePlan(t *testing.T, h *apptest.Harness, plan string) {
	t.Helper()
	approve := apptest.Form(t, h.Get(t, proposePlan(t, h, plan)).Body, "approve")
	if res := h.PostForm(t, approve.Action, approve.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d:\n%s", res.StatusCode, text(res.Body))
	}
}

// portionFor is the id of the Portion the Stock page shows as waiting for
// slot, e.g. "dinner on Sat 7 Nov".
func portionFor(t *testing.T, h *apptest.Harness, slot string) string {
	t.Helper()
	page := h.Get(t, "/stock").Body
	i := strings.Index(page, "for "+slot)
	if i < 0 {
		t.Fatalf("the Stock page shows no Portion for %s:\n%s", slot, text(page))
	}
	rest := page[i:]
	rest = rest[strings.Index(rest, `id="portion-`)+len(`id="portion-`):]
	return rest[:strings.Index(rest, `"`)]
}

func TestStockShowsPortionsOnceCookedUntilEaten(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonsPlan(t, h))

	// Nothing is cooked before the delivery.
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "No Portions waiting for their Slot.") || !strings.Contains(stock, "No Portions without a Slot.") {
		t.Errorf("Stock page shows Portions not yet cooked:\n%s", stock)
	}

	today.set(t, "2026-11-03")
	stock := text(h.Get(t, "/stock").Body)
	for _, want := range []string{
		"Lemon chicken with rice fridge for lunch on Wed 4 Nov",
		"Lemon chicken with rice freezer for dinner on Sat 7 Nov",
		"Ingredients as they are fridge for snack on Tue 3 Nov",
		"No Portions without a Slot.",
	} {
		if !strings.Contains(stock, want) {
			t.Errorf("Stock page on 3 Nov does not show %q:\n%s", want, stock)
		}
	}
	if strings.Contains(stock, "Mon 2 Nov") {
		t.Errorf("Stock page on 3 Nov shows the Portion eaten on 2 Nov:\n%s", stock)
	}

	// The Portion for Saturday will keep in the fridge after all.
	move := apptest.Form(t, h.Get(t, "/stock").Body, "portion-"+portionFor(t, h, "dinner on Sat 7 Nov"))
	move.Values.Set("location", "fridge")
	if stock := text(h.PostForm(t, move.Action, move.Values).Body); !strings.Contains(stock, "Lemon chicken with rice fridge for dinner on Sat 7 Nov") {
		t.Errorf("a Portion waiting for its Slot was not moved:\n%s", stock)
	}

	today.set(t, "2026-11-06")
	stock = text(h.Get(t, "/stock").Body)
	if strings.Contains(stock, "Wed 4 Nov") || strings.Contains(stock, "Tue 3 Nov") {
		t.Errorf("Stock page on 6 Nov shows Portions already eaten:\n%s", stock)
	}
	// The lemons are set out today: one for today's snack, one eaten after
	// the Cycle, without a Slot.
	if !strings.Contains(stock, "Ingredients as they are fridge for snack on Fri 6 Nov") || strings.Count(stock, "Ingredients as they are fridge") != 2 {
		t.Errorf("Stock page on 6 Nov does not show both lemons set out:\n%s", stock)
	}
}

func TestApprovalMidCycleKeepsWhatWasPrepared(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))
	today.set(t, "2026-11-04")
	saturday := portionFor(t, h, "dinner on Sat 7 Nov")

	// From Thursday on: a lemon for a snack, and the Lemon chicken with rice
	// already in the freezer moved from Saturday to Sunday.
	proposal := proposePlan(t, h, fmt.Sprintf(`{"from": {"on": "2026-11-05", "kind": "breakfast"},
		"prep_sessions": [
			{"on": "2026-11-05", "kind": "snack", "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
				{"location": "fridge", "eat_on": "2026-11-05", "eat_kind": "snack"}]}],
		"kept": [{"portion_id": %s, "on": "2026-11-08", "kind": "dinner"}]}`, saturday))
	if page := text(h.Get(t, proposal).Body); strings.Contains(page, "left without a Slot") || strings.Contains(page, "before the Plan Proposal starts") || strings.Contains(page, "is not a Portion without a Slot") {
		t.Errorf("Plan Proposal page does not offer the cooked Portion to the Plan Proposal:\n%s", page)
	}
	approve := apptest.Form(t, h.Get(t, proposal).Body, "approve")
	res := h.PostForm(t, approve.Action, approve.Values)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d:\n%s", res.StatusCode, text(res.Body))
	}
	plan := text(res.Body)
	for _, want := range []string{
		// What was prepared and eaten before Thursday stays.
		"Mon 2 Nov 739 kcal", "Tue 3 Nov 170 kcal", "Wed 4 Nov 739 kcal",
		"Thu 5 Nov 170 kcal", "Sat 7 Nov 150 kcal", "Sun 8 Nov 739 kcal",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("Plan page does not show %q:\n%s", want, plan)
		}
	}
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "Lemon chicken with rice freezer for dinner on Sun 8 Nov") {
		t.Errorf("the cooked Portion was not kept for its new Slot:\n%s", stock)
	}
}

func TestApprovalStartingTodayKeepsTodaysPrepSessions(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))
	today.set(t, "2026-11-02")

	// From tonight on, nothing: the Lemon chicken with rice cooked today
	// stays, its Portions without a Slot; tomorrow's lemon is dropped.
	approvePlan(t, h, `{"from": {"on": "2026-11-02", "kind": "dinner"}, "prep_sessions": []}`)
	stock := text(h.Get(t, "/stock").Body)
	if n := strings.Count(stock, "Lemon chicken with rice"); n != 3 || !strings.Contains(stock, "No Portions waiting for their Slot.") {
		t.Errorf("Stock page shows %d Portions of today's Prep Session, want 3 without a Slot:\n%s", n, stock)
	}
	today.set(t, "2026-11-03")
	if stock := text(h.Get(t, "/stock").Body); strings.Contains(stock, "Ingredients as they are") {
		t.Errorf("tomorrow's Prep Session was not replaced:\n%s", stock)
	}
}

func TestPlanProposalStartingInThePastIsNotApproved(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	proposal := proposePlan(t, h, lemonPlan(t, h))
	today.set(t, "2026-11-03")

	if page := text(h.Get(t, proposal).Body); !strings.Contains(page, "The Plan Proposal changes the Plan from breakfast on Mon 2 Nov, which is past") {
		t.Errorf("Plan Proposal page does not say it starts in the past:\n%s", page)
	}
	approve := apptest.Form(t, h.Get(t, proposal).Body, "approve")
	res := h.PostForm(t, approve.Action, approve.Values)
	if res.StatusCode != http.StatusConflict || !strings.Contains(res.Body, "starts in the past, so it was not approved") {
		t.Fatalf("approve a Plan Proposal starting in the past = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	if !strings.Contains(text(res.Body), "Initial Plan Proposal for the Cycle delivered") {
		t.Errorf("a refused approval does not show the Plan Proposal again:\n%s", text(res.Body))
	}
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "No Plan yet") {
		t.Errorf("a Plan Proposal starting in the past became the Plan:\n%s", plan)
	}
}

func TestDecidedPlanProposalIsNotCheckedAgain(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	proposal := proposePlan(t, h, lemonPlan(t, h))
	approve := apptest.Form(t, h.Get(t, proposal).Body, "approve")
	h.PostForm(t, approve.Action, approve.Values)
	today.set(t, "2026-11-03")

	if page := text(h.Get(t, proposal).Body); strings.Contains(page, "breaks the rules") {
		t.Errorf("an approved Plan Proposal is checked against today:\n%s", page)
	}
}

func TestUnreadablePlanProposalSaysWhyAndCanBeRejected(t *testing.T) {
	h, _ := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	proposal := proposePlan(t, h, `{"prep_sessions": [
		{"on": "2026-11-03", "kind": "snack", "ingredients": [{"name": "dragon fruit", "quantity": 1}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-03", "eat_kind": "snack"}]}]}`)

	res := h.Get(t, proposal)
	if page := text(res.Body); res.StatusCode != http.StatusOK || !strings.Contains(page, `can't be read: there is no Ingredient "dragon fruit"`) {
		t.Fatalf("GET an unreadable Plan Proposal = %d, want it saying why:\n%s", res.StatusCode, page)
	}
	if strings.Contains(res.Body, `id="approve"`) {
		t.Errorf("an unreadable Plan Proposal can be approved:\n%s", text(res.Body))
	}
	reject := apptest.Form(t, res.Body, "reject")
	if page := text(h.PostForm(t, reject.Action, reject.Values).Body); !strings.Contains(page, "rejected") {
		t.Errorf("an unreadable Plan Proposal was not rejected:\n%s", page)
	}
}

func TestPlanProposalNamesIngredientsHoweverWritten(t *testing.T) {
	h, _ := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, `{"prep_sessions": [
		{"on": "2026-11-03", "kind": "snack", "ingredients": [{"name": " Lemon", "quantity": 1}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-03", "eat_kind": "snack"}]}]}`)
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "snack lemon") {
		t.Errorf("Plan page does not show the lemon:\n%s", plan)
	}
}

func TestPlanIsLinkedFromTheAppShell(t *testing.T) {
	h := apptest.Start(t)
	if page := h.Get(t, "/").Body; !strings.Contains(page, `href="/plan"`) {
		t.Errorf("app shell does not link to the Plan page:\n%s", page)
	}
}

func TestUnknownPlanProposalIsNotFound(t *testing.T) {
	h, _ := startPlanning(t)
	if res := h.Get(t, "/plan-proposals/999999"); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET an unknown Plan Proposal = %d, want 404", res.StatusCode)
	}
}

func TestWhileACycleIsUpcomingTheCurrentPlanCanStillChange(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))
	today.set(t, "2026-11-04")
	openCycleIn(t, h, "2026-11-08", "18:00", "20:00", "")

	// The user ate out on Thursday: from Friday on, a lemon for a snack.
	proposal := proposePlanFor(t, h, "2026-11-02", `{"from": {"on": "2026-11-06", "kind": "breakfast"},
		"prep_sessions": [
			{"on": "2026-11-06", "kind": "snack", "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "snack"}]}]}`)
	approve := apptest.Form(t, h.Get(t, proposal).Body, "approve")
	res := h.PostForm(t, approve.Action, approve.Values)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve a current Plan Proposal while a Cycle is upcoming = %d:\n%s", res.StatusCode, text(res.Body))
	}
	if page := text(res.Body); !strings.Contains(page, "Fri 6 Nov 170 kcal") || !strings.Contains(page, "Upcoming Cycle") {
		t.Errorf("Plan page does not show the changed current Plan beside the upcoming Cycle:\n%s", page)
	}
}

// changeDelivery submits the upcoming Cycle's Delivery form with changes.
func changeDelivery(t *testing.T, h *apptest.Harness, changes map[string]string) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, "/plan").Body, "delivery")
	for k, v := range changes {
		form.Values.Set(k, v)
	}
	return h.PostForm(t, form.Action, form.Values)
}

func TestMovingAnUpcomingDeliveryNeedsANewPlanProposal(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "2026-11-10")
	approvePlan(t, h, lemonPlan(t, h))
	today.set(t, "2026-11-07")
	openCycleIn(t, h, "2026-11-10", "18:00", "20:00", "2026-11-20")
	approvePlan(t, h, `{"prep_sessions": [
		{"on": "2026-11-10", "kind": "dinner", "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-10", "eat_kind": "dinner"}]}]}`)
	pending := proposePlan(t, h, `{"prep_sessions": []}`)
	if page := text(h.Get(t, "/plan").Body); strings.Contains(page, "needs a new Plan Proposal") {
		t.Fatalf("a Plan needs a new Plan Proposal before any move:\n%s", page)
	}

	// The supermarket has no slot on the 10th: the 11th, in the morning.
	res := changeDelivery(t, h, map[string]string{"delivery_on": "2026-11-11", "window_from": "09:00", "window_to": "11:00"})
	if res.StatusCode != http.StatusOK || res.Path != "/plan" {
		t.Fatalf("move the Delivery = %d at %s:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	page := text(res.Body)
	for _, want := range []string{
		// A day later, it is to last a day longer.
		"Upcoming Cycle Delivery Wed 11 Nov 2026, 09:00–11:00 tentative Starts lunch on Wed 11 Nov Ends Sat 21 Nov 2026",
		// Both Plans: the upcoming one moved, and the current one ends on
		// the 10th, no longer reaching it.
		"Cycle Delivery Mon 2 Nov 2026, 07:00–07:30 tentative Starts breakfast on Mon 2 Nov Ends Tue 10 Nov 2026 A Delivery moved: this Plan needs a new Plan Proposal.",
		"Ends Sat 21 Nov 2026 A Delivery moved: this Plan needs a new Plan Proposal.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Plan page does not show %q:\n%s", want, page)
		}
	}
	if page := text(h.Get(t, pending).Body); !strings.Contains(page, "rejected") || !strings.Contains(page, "Note: Its Delivery moved to Wed 11 Nov, 09:00–11:00.") {
		t.Errorf("the moved Cycle's pending Plan Proposal was not rejected, saying why:\n%s", page)
	}

	// A new Plan Proposal for the moved Cycle clears it.
	approvePlan(t, h, `{"prep_sessions": [
		{"on": "2026-11-11", "kind": "dinner", "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-11", "eat_kind": "dinner"}]}]}`)
	if page := text(h.Get(t, "/plan").Body); strings.Count(page, "needs a new Plan Proposal") != 1 {
		t.Errorf("an approved Plan Proposal leaves its Plan needing one, or clears another's:\n%s", page)
	}
}

func TestMovingADeliveryIsCheckedLikeOpeningIt(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	today.set(t, "2026-11-08")
	openCycleIn(t, h, "2026-11-12", "18:00", "20:00", "2026-11-20")

	for _, tt := range []struct {
		changes map[string]string
		message string
	}{
		{map[string]string{"delivery_on": "2026-11-02"}, "Must be after the current Cycle's delivery on Mon 2 Nov 2026"},
		{map[string]string{"window_to": "17:00"}, "Must be after the start of the window"},
	} {
		res := changeDelivery(t, h, tt.changes)
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), tt.message) {
			t.Errorf("change the Delivery to %v = %d, want 422 saying %q:\n%s", tt.changes, res.StatusCode, tt.message, text(res.Body))
		}
	}
	if page := text(h.Get(t, "/plan").Body); !strings.Contains(page, "Thu 12 Nov 2026, 18:00–20:00") {
		t.Errorf("a refused change moved the Delivery:\n%s", page)
	}
}

func TestDeliveryIsTentativeUntilMarkedOrdered(t *testing.T) {
	h, _ := startPlanning(t)
	openCycleIn(t, h, "2026-11-02", "18:00", "20:00", "")

	res := changeDelivery(t, h, map[string]string{"ordered": "yes"})
	page := text(res.Body)
	if res.StatusCode != http.StatusOK || !strings.Contains(page, "Mon 2 Nov 2026, 18:00–20:00 ordered") {
		t.Fatalf("mark the Delivery ordered = %d, want it shown ordered:\n%s", res.StatusCode, page)
	}
	if strings.Contains(page, "needs a new Plan Proposal") {
		t.Errorf("marking the Delivery ordered, unmoved, leaves the Plan needing a new Plan Proposal:\n%s", page)
	}
	// Ordered, it can still move, and stays ordered.
	if page := text(changeDelivery(t, h, map[string]string{"window_from": "19:00", "window_to": "21:00"}).Body); !strings.Contains(page, "Mon 2 Nov 2026, 19:00–21:00 ordered") {
		t.Errorf("moving an ordered Delivery does not keep it ordered:\n%s", page)
	}
}

func TestDroppingAnUpcomingCycle(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))
	today.set(t, "2026-11-08")
	openCycleIn(t, h, "2026-11-12", "18:00", "20:00", "")
	proposal := proposePlan(t, h, `{"prep_sessions": [
		{"on": "2026-11-13", "kind": "snack", "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
			{"location": "fridge", "eat_on": "2026-11-13", "eat_kind": "snack"}]}]}`)

	drop := apptest.Form(t, h.Get(t, "/plan").Body, "drop")
	res := h.PostForm(t, drop.Action, drop.Values)
	if res.StatusCode != http.StatusOK || res.Path != "/plan" {
		t.Fatalf("drop the upcoming Cycle = %d at %s:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	if page := text(res.Body); strings.Contains(page, "Upcoming Cycle") || !strings.Contains(page, "Cycle Delivery Mon 2 Nov 2026") {
		t.Errorf("Plan page still shows the dropped Cycle, or lost the current one:\n%s", page)
	}
	if res := h.Get(t, proposal); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET a dropped Cycle's Plan Proposal = %d, want 404", res.StatusCode)
	}
	// The current Cycle's days after the 12th are days between Cycles.
	today.set(t, "2026-11-13")
	if page := text(h.Get(t, "/plan").Body); !strings.Contains(page, "Cycle Delivery Mon 2 Nov 2026") {
		t.Errorf("the dropped Cycle's Delivery date closed the current Cycle:\n%s", page)
	}
}

func TestAnOrderedDeliverysCycleIsNotDropped(t *testing.T) {
	h, _ := startPlanning(t)
	openCycleIn(t, h, "2026-11-02", "18:00", "20:00", "")
	drop := apptest.Form(t, h.Get(t, "/plan").Body, "drop")
	changeDelivery(t, h, map[string]string{"ordered": "yes"})

	if page := h.Get(t, "/plan").Body; strings.Contains(page, `id="drop"`) {
		t.Errorf("an ordered Delivery's Cycle is offered for dropping:\n%s", text(page))
	}
	res := h.PostForm(t, drop.Action, drop.Values)
	if res.StatusCode != http.StatusConflict || !strings.Contains(text(res.Body), "The Delivery was ordered, so its Cycle can't be dropped.") {
		t.Errorf("drop an ordered Delivery's Cycle = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
}

func TestPlanTheNextCycleIsOfferedFromThreeDaysBeforeTheEndDate(t *testing.T) {
	h, today := startPlanning(t)
	openCycle(t, h, "2026-11-02", "")
	today.set(t, "2026-11-08")
	if page := text(h.Get(t, "/plan").Body); strings.Contains(page, "Plan the next Cycle") {
		t.Errorf("Plan the next Cycle is offered four days before the end date:\n%s", page)
	}

	today.set(t, "2026-11-09")
	page := text(h.Get(t, "/plan").Body)
	if !strings.Contains(page, "This Cycle ends on Thu 12 Nov 2026: time to plan the next one") || !strings.Contains(page, "Plan the next Cycle") {
		t.Errorf("Plan the next Cycle is not offered three days before the end date:\n%s", page)
	}

	openCycleIn(t, h, "2026-11-12", "18:00", "20:00", "")
	if page := text(h.Get(t, "/plan").Body); strings.Contains(page, "Plan the next Cycle") {
		t.Errorf("Plan the next Cycle is offered while one is upcoming:\n%s", page)
	}
	if page := h.Get(t, "/plan").Body; strings.Contains(page, `id="open-cycle"`) {
		t.Errorf("a Cycle can be opened while one is upcoming")
	}
}
