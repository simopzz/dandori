package app_test

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
)

// session is a Prep Session as the planner submits it.
type session = map[string]any

// eat is a Portion kept in location and eaten at kind on the day on.
func eat(location, on, kind string) map[string]any {
	return map[string]any{"location": location, "eat_on": on, "eat_kind": kind}
}

// lemonAt is a lemon set out at kind on the day on, and eaten straight
// away.
func lemonAt(on, kind string) session {
	return session{"on": on, "kind": kind,
		"ingredients": []map[string]any{{"name": "lemon", "quantity": 1}},
		"portions":    []map[string]any{eat("fridge", on, kind)}}
}

// shortCyclePlan is a Plan for the Cycle delivered on 2026-11-02 and
// ending on the 4th that fills every one of its Slots: first, a Prep
// Session cooked for dinner on the 2nd, eaten then and at lunch on the
// 4th, its third Portion frozen for after the Cycle; lemons for the rest.
// The Cycle is too short for a trailing seven days to be checked.
func shortCyclePlan(first session) map[string]any {
	sessions := []session{first}
	for _, on := range []string{"2026-11-02", "2026-11-03", "2026-11-04"} {
		for _, kind := range []string{"breakfast", "lunch", "snack", "dinner"} {
			if on == "2026-11-02" && kind == "dinner" || on == "2026-11-04" && kind == "lunch" {
				continue
			}
			sessions = append(sessions, lemonAt(on, kind))
		}
	}
	return map[string]any{"prep_sessions": sessions}
}

// dinnerOf is a Prep Session of what, a Recipe as {"recipe_id": …} or a
// drafted one as {"draft_id": …}, for shortCyclePlan.
func dinnerOf(what map[string]any) session {
	s := session{"on": "2026-11-02", "kind": "dinner", "portions": []map[string]any{
		eat("fridge", "2026-11-02", "dinner"),
		eat("fridge", "2026-11-04", "lunch"),
		eat("freezer", "2026-11-07", "dinner"),
	}}
	for k, v := range what {
		s[k] = v
	}
	return s
}

// plannerRequests are the requests the planner sent the model, in order:
// those of planning Runs.
func plannerRequests(llm *fakellm.LLM) []agent.Request {
	var planner []agent.Request
	for _, req := range llm.Requests() {
		if strings.HasPrefix(req.System, "You plan a Cycle") {
			planner = append(planner, req)
		}
	}
	return planner
}

// recipeID is the id of the Recipe in the Recipe Library.
func recipeID(t *testing.T, h *apptest.Harness) int64 {
	t.Helper()
	id, err := strconv.ParseInt(strings.TrimPrefix(libraryRecipe(t, h), "/recipes/"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// planCycle plans the Cycle from the page at path through its Plan form,
// waits for the planning Run to end and returns its page.
func planCycle(t *testing.T, h *apptest.Harness, path string) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, path).Body, "plan-cycle")
	res := h.PostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Path, "/runs/") {
		t.Fatalf("plan the Cycle = %d at %s, want the planning Run's page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	h.Stream(t, res.Path+"/events", "").Rest(t)
	return h.Get(t, res.Path)
}

func TestPlanningACycleYieldsAPlanProposalThatCanBeApproved(t *testing.T) {
	h, _, llm := startPlanningWith(t)
	exceptions := openCycle(t, h, "2026-11-02", "2026-11-04").Path
	recipe := recipeID(t, h)
	llm.Script(
		fakellm.CallTools("Looking at what there is.",
			fakellm.Call{Tool: "get_profile", Input: map[string]any{}},
			fakellm.Call{Tool: "list_exceptions", Input: map[string]any{}},
			fakellm.Call{Tool: "list_kitchen", Input: map[string]any{}},
			fakellm.Call{Tool: "search_recipes", Input: map[string]any{}},
			fakellm.Call{Tool: "list_catalog", Input: map[string]any{}},
		),
		fakellm.CallTool("submit_plan", shortCyclePlan(dinnerOf(map[string]any{"recipe_id": recipe}))),
		fakellm.Reply("Planned the Cycle around Lemon chicken with rice."),
	)

	run := text(planCycle(t, h, exceptions).Body)
	if !strings.Contains(run, "Succeeded") {
		t.Fatalf("planning Run did not succeed:\n%s", run)
	}
	asked := plannerRequests(llm)
	prompt := asked[0].Messages[0].Content[0].Text
	for _, want := range []string{"breakfast on Mon 2 Nov", "dinner on Wed 4 Nov"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the planner is not told the Cycle's Slots run to %q:\n%s", want, prompt)
		}
	}
	tools := fmt.Sprint(asked[1].Messages[2].Content)
	for _, want := range []string{"2200 kcal", "citrus", "No Exceptions", "Lemon chicken with rice", "Known Catalog"} {
		if !strings.Contains(tools, want) {
			t.Errorf("the planner's tools do not tell it %q:\n%s", want, tools)
		}
	}

	paths := proposalPaths(t, h)
	if len(paths) != 1 {
		t.Fatalf("the Plan page lists %d Plan Proposals, want the planner's one", len(paths))
	}
	page := h.Get(t, paths[0])
	if p := text(page.Body); !strings.Contains(p, "Initial Plan Proposal") || strings.Contains(p, "breaks the rules") {
		t.Errorf("the planner's Plan Proposal is not a valid initial one:\n%s", p)
	}
	approve := apptest.Form(t, page.Body, "approve")
	plan := text(h.PostForm(t, approve.Action, approve.Values).Body)
	for _, want := range []string{"dinner Lemon chicken with rice", "snack lemon", "approved"} {
		if !strings.Contains(plan, want) {
			t.Errorf("Plan page does not show the approved Plan's %q:\n%s", want, plan)
		}
	}
}

// withoutSlot is plan with the lemon set out for the Slot kind on the day
// on dropped, leaving that Slot empty.
func withoutSlot(plan map[string]any, on, kind string) map[string]any {
	var kept []session
	for _, s := range plan["prep_sessions"].([]session) {
		if s["on"] != on || s["kind"] != kind {
			kept = append(kept, s)
		}
	}
	return map[string]any{"prep_sessions": kept}
}

func TestAPlanBreakingTheRulesGoesBackToThePlanner(t *testing.T) {
	h, _, llm := startPlanningWith(t, apptest.WithConfig(func(c *app.Config) { c.PlanAttempts = 2 }))
	exceptions := openCycle(t, h, "2026-11-02", "2026-11-04").Path
	plan := shortCyclePlan(dinnerOf(map[string]any{"recipe_id": recipeID(t, h)}))
	llm.Script(
		fakellm.CallTool("submit_plan", withoutSlot(plan, "2026-11-03", "snack")),
		fakellm.CallTool("submit_plan", plan),
		fakellm.Reply("Planned the Cycle, every Slot filled."),
	)

	run := text(planCycle(t, h, exceptions).Body)
	for _, want := range []string{
		"The Plan breaks the rules: attempt 2 of 2",
		"empty-slot: snack on Tue 3 Nov is empty",
		"Succeeded",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("the planning Run's Trace does not show %q:\n%s", want, run)
		}
	}
	retry := plannerRequests(llm)[1].Messages[2].Content[0]
	if !retry.IsError || !strings.Contains(retry.Text, "snack on Tue 3 Nov is empty") {
		t.Errorf("the planner was not told the rule its Plan broke: %+v", retry)
	}
	if paths := proposalPaths(t, h); len(paths) != 1 {
		t.Errorf("the Plan page lists %d Plan Proposals, want only the Plan that keeps the rules", len(paths))
	}
}

func TestPlansBreakingTheRulesPastTheBoundFailTheRun(t *testing.T) {
	h, _, llm := startPlanningWith(t, apptest.WithConfig(func(c *app.Config) { c.PlanAttempts = 2 }))
	exceptions := openCycle(t, h, "2026-11-02", "2026-11-04").Path
	broken := withoutSlot(shortCyclePlan(dinnerOf(map[string]any{"recipe_id": recipeID(t, h)})), "2026-11-03", "snack")
	llm.Script(
		fakellm.CallTool("submit_plan", broken),
		fakellm.CallTool("submit_plan", broken),
		fakellm.Reply("unreachable"),
	)

	run := text(planCycle(t, h, exceptions).Body)
	if !strings.Contains(run, "Failed") || !strings.Contains(run, "the planner's Plan still broke the rules after 2 attempts: - empty-slot: snack on Tue 3 Nov is empty") {
		t.Errorf("the planning Run does not fail saying the Plan still broke the rules:\n%s", run)
	}
	if got := len(plannerRequests(llm)); got != 2 {
		t.Errorf("the planner was asked %d times, want 2: no turn after the last attempt", got)
	}
	plan := text(h.Get(t, "/plan").Body)
	for _, want := range []string{"No Plan Proposals yet.", "planning Run failed: the planner's Plan still broke the rules", "Plan this Cycle"} {
		if !strings.Contains(plan, want) {
			t.Errorf("the Plan page does not show %q:\n%s", want, plan)
		}
	}
}

func TestAPlannerEndingWithoutAPlanFailsTheRun(t *testing.T) {
	h, _, llm := startPlanningWith(t)
	exceptions := openCycle(t, h, "2026-11-02", "2026-11-04").Path
	llm.Script(fakellm.Reply("I could not plan this."))

	run := text(planCycle(t, h, exceptions).Body)
	if !strings.Contains(run, "Failed") || !strings.Contains(run, "the planner ended without submitting a Plan") {
		t.Errorf("the planning Run does not fail saying no Plan was submitted:\n%s", run)
	}
}

// planWithDraft scripts the planner to draft the Chickpea and chicken curry
// and cook it for dinner on the 2nd of shortCyclePlan.
func planWithDraft(llm *fakellm.LLM) {
	llm.Script(
		fakellm.CallTool("draft_recipe", chickpeaCurryParsed()),
		fakellm.Then(func(req agent.Request) fakellm.Step {
			result := req.Messages[len(req.Messages)-1].Content[0].Text
			_, after, _ := strings.Cut(result, "draft_id ")
			id, err := strconv.ParseInt(strings.TrimRight(strings.Fields(after)[0], ":"), 10, 64)
			if err != nil {
				return fakellm.Reply("no draft id in " + result)
			}
			return fakellm.CallTool("submit_plan", shortCyclePlan(dinnerOf(map[string]any{"draft_id": id})))
		}),
		fakellm.Reply("Planned the Cycle around a new curry."),
	)
}

func TestDraftedRecipesAreInTheRecipeLibraryOnlyOnceAccepted(t *testing.T) {
	tests := []struct {
		name string
		// decide accepts, approves or discards from the Plan Proposal page
		// at proposal, returning the page it lands on.
		decide func(t *testing.T, h *apptest.Harness, proposal string) apptest.Response
		// accepted is whether the curry ends up in the Recipe Library.
		accepted bool
		want     string
	}{
		{
			name: "accepted on its own",
			decide: func(t *testing.T, h *apptest.Harness, proposal string) apptest.Response {
				draft := h.Get(t, apptest.Link(t, h.Get(t, proposal).Body, "Chickpea and chicken curry"))
				form := apptest.Form(t, draft.Body, "recipe")
				form.Values.Set("name", "Chickpea curry")
				res := h.PostForm(t, form.Action, form.Values)
				if res.Path != proposal || !strings.Contains(text(res.Body), "Chickpea curry accepted") {
					t.Fatalf("accept the Recipe Draft = %d at %s, want its Plan Proposal showing it accepted:\n%s", res.StatusCode, res.Path, text(res.Body))
				}
				approve := apptest.Form(t, res.Body, "approve")
				return h.PostForm(t, approve.Action, approve.Values)
			},
			accepted: true, want: "dinner Chickpea curry",
		},
		{
			name: "accepted as drafted by approving the Plan Proposal",
			decide: func(t *testing.T, h *apptest.Harness, proposal string) apptest.Response {
				approve := apptest.Form(t, h.Get(t, proposal).Body, "approve")
				return h.PostForm(t, approve.Action, approve.Values)
			},
			accepted: true, want: "dinner Chickpea and chicken curry",
		},
		{
			name: "discarded",
			decide: func(t *testing.T, h *apptest.Harness, proposal string) apptest.Response {
				draft := h.Get(t, apptest.Link(t, h.Get(t, proposal).Body, "Chickpea and chicken curry"))
				discard := apptest.Form(t, draft.Body, "discard")
				return h.PostForm(t, discard.Action, discard.Values)
			},
			want: "This Plan Proposal can't be read: there is no Recipe Draft",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, llm := startPlanningWith(t)
			exceptions := openCycle(t, h, "2026-11-02", "2026-11-04").Path
			planWithDraft(llm)
			if run := text(planCycle(t, h, exceptions).Body); !strings.Contains(run, "Succeeded") {
				t.Fatalf("planning Run did not succeed:\n%s", run)
			}
			if library := text(h.Get(t, "/recipes").Body); strings.Contains(library, "Chickpea") {
				t.Fatalf("a drafted Recipe is in the Recipe Library before it is accepted:\n%s", library)
			}
			proposal := proposalPaths(t, h)[0]
			page := text(h.Get(t, proposal).Body)
			if !strings.Contains(page, "Recipe Drafts") || !strings.Contains(page, "Chickpea and chicken curry to check") ||
				!strings.Contains(page, "Chickpea and chicken curry dinner on Mon 2 Nov 3 Portions") || strings.Contains(page, "breaks the rules") {
				t.Fatalf("the Plan Proposal does not show its drafted Recipe for acceptance:\n%s", page)
			}

			if res := text(tt.decide(t, h, proposal).Body); !strings.Contains(res, tt.want) {
				t.Errorf("deciding lands on a page without %q:\n%s", tt.want, res)
			}
			if library := text(h.Get(t, "/recipes").Body); strings.Contains(library, "Chickpea") != tt.accepted {
				t.Errorf("the Recipe Library lists the drafted Recipe: %t, want %t:\n%s", !tt.accepted, tt.accepted, library)
			}
		})
	}
}

// rejectWith rejects the Plan Proposal at proposal with note, returning
// the page it lands on.
func rejectWith(t *testing.T, h *apptest.Harness, proposal, note string) apptest.Response {
	t.Helper()
	reject := apptest.Form(t, h.Get(t, proposal).Body, "reject")
	reject.Values.Set("note", note)
	res := h.PostForm(t, reject.Action, reject.Values)
	if res.StatusCode != http.StatusOK || res.Path != proposal {
		t.Fatalf("reject = %d at %s, want the Plan Proposal page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	return res
}

func TestRejectingWithANoteStartsAPlanningRunTakingItIntoAccount(t *testing.T) {
	h, _, llm := startPlanningWith(t)
	exceptions := openCycle(t, h, "2026-11-02", "2026-11-04").Path
	plan := shortCyclePlan(dinnerOf(map[string]any{"recipe_id": recipeID(t, h)}))
	propose := func() {
		llm.Script(fakellm.CallTool("submit_plan", plan), fakellm.Reply("Planned."))
	}
	propose()
	planCycle(t, h, exceptions)

	for _, note := range []string{"less chicken", "no oven on Tuesday"} {
		propose()
		res := rejectWith(t, h, proposalPaths(t, h)[0], note)
		if !strings.Contains(text(res.Body), "A new planning Run takes the note into account") {
			t.Fatalf("rejecting with a note does not start a planning Run:\n%s", text(res.Body))
		}
		replan := apptest.Link(t, res.Body, "follow it")
		h.Stream(t, replan+"/events", "").Rest(t)
		if run := text(h.Get(t, replan).Body); !strings.Contains(run, "Succeeded") {
			t.Fatalf("the new planning Run did not succeed:\n%s", run)
		}
	}
	asked := plannerRequests(llm)
	prompt := asked[len(asked)-2].Messages[0].Content[0].Text
	if !strings.Contains(prompt, "notes, newest first. Take them into account:\n- no oven on Tuesday\n- less chicken") {
		t.Errorf("the last planning Run is not told both notes, newest first:\n%s", prompt)
	}
	if paths := proposalPaths(t, h); len(paths) != 3 {
		t.Errorf("the Plan page lists %d Plan Proposals, want 3", len(paths))
	}

	res := rejectWith(t, h, proposalPaths(t, h)[0], "")
	if strings.Contains(text(res.Body), "A new planning Run") {
		t.Errorf("rejecting without a note started a planning Run:\n%s", text(res.Body))
	}
	if plan := text(h.Get(t, "/plan").Body); !strings.Contains(plan, "Plan this Cycle") {
		t.Errorf("the Plan page does not offer to plan the Cycle again:\n%s", plan)
	}
}

func TestACycleBeingPlannedIsNotPlannedTwice(t *testing.T) {
	h, _, llm := startPlanningWith(t)
	exceptions := openCycle(t, h, "2026-11-02", "2026-11-04").Path
	llm.Script(fakellm.Hang())
	form := apptest.Form(t, h.Get(t, exceptions).Body, "plan-cycle")
	first := h.PostForm(t, form.Action, form.Values)

	if again := h.PostForm(t, form.Action, form.Values); again.Path != first.Path {
		t.Errorf("planning again went to %s, want the planning Run going, %s", again.Path, first.Path)
	}
	plan := h.Get(t, "/plan")
	if !strings.Contains(text(plan.Body), "Being planned: follow the planning Run") || strings.Contains(text(plan.Body), "Plan this Cycle") {
		t.Errorf("the Plan page does not show the Cycle being planned:\n%s", text(plan.Body))
	}
	if link := apptest.Link(t, plan.Body, "follow the planning Run"); link != first.Path {
		t.Errorf("the Plan page follows %s, want %s", link, first.Path)
	}
}
