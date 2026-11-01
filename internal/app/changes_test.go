package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// askForChanges asks for changes to the Plan of the first Cycle on the
// Plan page that offers it, with note, returning the page it lands on.
func askForChanges(t *testing.T, h *apptest.Harness, note string) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, "/plan").Body, "ask-for-changes")
	form.Values.Set("note", note)
	return h.PostForm(t, form.Action, form.Values)
}

// changesAction is where changes to the Plan of the Cycle whose Exceptions
// page is at exceptions are asked for.
func changesAction(exceptions string) string {
	return strings.TrimSuffix(exceptions, "/exceptions") + "/changes"
}

func TestAskingForChangesStartsAPlanningRunFromTheFirstSlotNotYetPrepared(t *testing.T) {
	h, today, llm := startPlanningWith(t)
	openCycle(t, h, "2026-11-02", "2026-11-04")
	recipe := recipeID(t, h)
	approvePlan(t, h, shortCyclePlan(dinnerOf(map[string]any{"recipe_id": recipe})))
	today.set(t, "2026-11-03")
	// Cooked on Monday, for lunch on Wednesday and frozen for after the
	// Cycle: they stay, for the Plan Proposal to put in Slots again.
	wednesday := portionFor(t, h, "lunch on Wed 4 Nov")
	frozen := portionWithoutSlot(t, h)
	cooked := func() int {
		return strings.Count(section(t, h.Get(t, "/plan").Body, "Plan"), "dinner Lemon chicken with rice")
	}
	if n := cooked(); n != 1 {
		t.Fatalf("the Plan cooks Lemon chicken with rice for %d dinners, want 1:\n%s", n, text(h.Get(t, "/plan").Body))
	}

	changed := keeping(planOf(lemonAt("2026-11-04", "breakfast"), lemonAt("2026-11-04", "snack")), wednesday, "2026-11-04", "lunch")
	llm.Script(fakellm.CallTool("submit_plan", keeping(changed, frozen, "2026-11-04", "dinner")), fakellm.Reply("Lemon chicken for dinner on Wednesday too."))
	res := askForChanges(t, h, "Lemon chicken again on Wednesday")
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Path, "/runs/") {
		t.Fatalf("ask for changes = %d at %s, want the planning Run's page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	h.Stream(t, res.Path+"/events", "").Rest(t)
	if run := text(h.Get(t, res.Path).Body); !strings.Contains(run, "Succeeded") {
		t.Fatalf("the planning Run did not succeed:\n%s", run)
	}

	prompt := plannerRequests(llm)[0].Messages[0].Content[0].Text
	var missing []string
	for _, want := range []string{
		"Plan every Slot from breakfast on Wed 4 Nov on.",
		"- Lemon chicken again on Wednesday",
		// The Plan it changes, from that Slot on.
		"The Cycle already has a Plan. Yours replaces it from breakfast on Wed 4 Nov on",
		fmt.Sprintf("- lunch on Wed 4 Nov: Portion %s, Lemon chicken with rice", wednesday),
		"- lemon at dinner on Wed 4 Nov, eaten at dinner on Wed 4 Nov",
		"Change only what the notes ask for",
	} {
		if !strings.Contains(prompt, want) {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		t.Errorf("the planning Run is not told %q:\n%s", missing, prompt)
	}

	if n := cooked(); n != 1 {
		t.Errorf("the Plan changed before its Plan Proposal was approved: %d dinners of Lemon chicken with rice", n)
	}
	page := h.Get(t, proposalPaths(t, h)[0])
	if p := text(page.Body); !strings.Contains(p, "Changes Plan Proposal") || !strings.Contains(p, "pending") {
		t.Fatalf("the planning Run's Plan Proposal is not a pending Changes Plan Proposal:\n%s", p)
	}
	approve := apptest.Form(t, page.Body, "approve")
	if res := h.PostForm(t, approve.Action, approve.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d:\n%s", res.StatusCode, text(res.Body))
	}
	if n := cooked(); n != 2 {
		t.Errorf("the approved Plan cooks Lemon chicken with rice for %d dinners, want 2:\n%s", n, text(h.Get(t, "/plan").Body))
	}
}

// portionWithoutSlot is the id of the Portion the Stock page shows
// without a Slot, the first when there are several.
func portionWithoutSlot(t *testing.T, h *apptest.Harness) string {
	t.Helper()
	page := h.Get(t, "/stock").Body
	i := strings.Index(page, "Portions without a Slot")
	if i < 0 {
		t.Fatalf("the Stock page has no Portions without a Slot:\n%s", text(page))
	}
	rest := page[i:]
	j := strings.Index(rest, `id="portion-`)
	if j < 0 {
		t.Fatalf("the Stock page shows no Portion without a Slot:\n%s", text(page))
	}
	rest = rest[j+len(`id="portion-`):]
	id, _, ok := strings.Cut(rest, `"`)
	if !ok {
		t.Fatalf("the Stock page's Portion without a Slot has no id:\n%s", text(page))
	}
	return id
}

func TestAskingForChangesNeedsANote(t *testing.T) {
	h, _ := startPlanning(t)
	openCycle(t, h, "2026-11-02", "2026-11-04")
	approvePlan(t, h, shortCyclePlan(dinnerOf(map[string]any{"recipe_id": recipeID(t, h)})))

	res := askForChanges(t, h, "  ")
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("ask for changes without a note = %d, want 422", res.StatusCode)
	}
	if plan := section(t, res.Body, "Plan"); !strings.Contains(plan, "Required: say what you'd change") {
		t.Errorf("the form does not say the note is required:\n%s", plan)
	}
	if runs := text(h.Get(t, "/runs").Body); strings.Contains(runs, "planning") {
		t.Errorf("a planning Run started without a note:\n%s", runs)
	}
}

func TestAskingForChangesIsAbsentWhilePlanningOrOnceClosed(t *testing.T) {
	h, today, llm := startPlanningWith(t)
	current, _ := twoCycles(t, h, today)
	approve := apptest.Form(t, h.Get(t, proposePlanFor(t, h, "2026-11-02", planFrom("2026-11-05", "breakfast", lemonAt("2026-11-05", "breakfast")))).Body, "approve")
	if res := h.PostForm(t, approve.Action, approve.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d:\n%s", res.StatusCode, text(res.Body))
	}
	offered := func() bool { return strings.Contains(h.Get(t, "/plan").Body, `id="ask-for-changes"`) }
	if !offered() {
		t.Fatalf("asking for changes is not offered on the current Cycle's Plan:\n%s", text(h.Get(t, "/plan").Body))
	}

	pending := proposePlanFor(t, h, "2026-11-08", planOf())
	if offered() {
		t.Errorf("asking for changes is offered while the upcoming Cycle's Plan Proposal is pending")
	}
	res := h.PostForm(t, changesAction(current), map[string][]string{"note": {"more fish"}})
	if res.StatusCode != http.StatusConflict || !strings.Contains(text(res.Body), "No changes can be asked for: a Plan Proposal for the upcoming Cycle is pending: approve or reject it.") {
		t.Errorf("ask for changes while a Plan Proposal is pending = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	rejectWith(t, h, pending, "")

	llm.Script(fakellm.Hang())
	run := askForChanges(t, h, "more fish").Path
	if offered() {
		t.Errorf("asking for changes is offered while a planning Run is going")
	}
	if link := apptest.Link(t, h.Get(t, "/plan").Body, "follow the planning Run"); link != run {
		t.Errorf("the Plan page follows %s, want the planning Run going, %s", link, run)
	}

	today.setAt(t, "2026-11-08 20:00")
	h.Get(t, "/plan")
	h.Stream(t, run+"/events", "").Rest(t)
	res = h.PostForm(t, changesAction(current), map[string][]string{"note": {"more fish"}})
	if res.StatusCode != http.StatusConflict || !strings.Contains(text(res.Body), "the next Delivery has arrived, so it is closed") {
		t.Errorf("ask for changes on a closed Cycle = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	if n := planningRunsGoing(t, h); n != 0 {
		t.Errorf("%d planning Runs going for a closed Cycle", n)
	}
}

func TestEveryLaterPlanningRunIsToldTheNotesOfRejectionsAndChangesNewestFirst(t *testing.T) {
	h, _, llm := startPlanningWith(t)
	exceptions := openCycle(t, h, "2026-11-02", "2026-11-04").Path
	plan := shortCyclePlan(dinnerOf(map[string]any{"recipe_id": recipeID(t, h)}))
	propose := func() { llm.Script(fakellm.CallTool("submit_plan", plan), fakellm.Reply("Planned.")) }
	propose()
	planCycle(t, h, exceptions)
	propose()
	replan := apptest.Link(t, rejectWith(t, h, proposalPaths(t, h)[0], "more pasta").Body, "follow it")
	h.Stream(t, replan+"/events", "").Rest(t)
	approve := apptest.Form(t, h.Get(t, proposalPaths(t, h)[0]).Body, "approve")
	h.PostForm(t, approve.Action, approve.Values)

	llm.Script(fakellm.Reply("Nothing to change."))
	run := askForChanges(t, h, "less pasta").Path
	h.Stream(t, run+"/events", "").Rest(t)
	asked := plannerRequests(llm)
	if prompt := asked[len(asked)-1].Messages[0].Content[0].Text; !strings.Contains(prompt, "newest first. Take them into account; where two disagree, the newer one wins:\n- less pasta\n- more pasta") {
		t.Errorf("the planning Run is not told both notes, newest first:\n%s", prompt)
	}
}

func TestChangesToAnOrderedDeliverysCycleAreToldWhatIsOrdered(t *testing.T) {
	h, _, llm := startGroceriesWith(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))
	changeDelivery(t, h, map[string]string{"ordered": "yes"})

	llm.Script(fakellm.CallTool("list_kitchen", map[string]any{}), fakellm.Reply("Nothing to change."))
	run := askForChanges(t, h, "more chicken").Path
	h.Stream(t, run+"/events", "").Rest(t)
	asked := plannerRequests(llm)
	if len(asked) < 2 {
		t.Fatalf("the planner sent %d requests, want the kitchen's answer in the second", len(asked))
	}
	kitchen := fmt.Sprint(asked[1].Messages)
	if !strings.Contains(kitchen, "Ordered, arriving with the Delivery on Mon 2 Nov 2026:") || !strings.Contains(kitchen, "640 g chicken breast") {
		t.Errorf("the planner is not told what the ordered Delivery brings:\n%s", kitchen)
	}
}
