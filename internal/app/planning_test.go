package app_test

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
)

// friday is a Cycle whose Delivery comes on Thursday 5 November between 6 and 8 pm and
// that ends on Friday 6: its Slots are Friday's four.
var friday = apptest.Cycle{Delivery: "11/05/2026", From: "6pm", To: "8pm", Ends: "11/06/2026"}

// yogurt is a food set out as it is.
var yogurt = apptest.Ingredient{Name: "yogurt", Unit: "g", Usual: "fridge", Kcal: "60", Protein: "10", Carbs: "4", Fat: "0"}

// plannerWithKitchen signs up email with their Targets set, the Recipe Library holding
// the Rice bowl (two Portions of 200 g rice as written), 800 g of rice in the pantry,
// yogurt, and three Portions of lasagne in the freezer, in an app started with opts.
func plannerWithKitchen(t *testing.T, email string, opts ...apptest.Option) *apptest.Scenario {
	t.Helper()
	s := plannerOf(t, email, opts...)
	stockKitchen(s)
	return s
}

// stockKitchen gives the signed-in User of s the kitchen plannerWithKitchen gives.
func stockKitchen(s *apptest.Scenario) {
	s.Open("/ingredients")
	s.AddsIngredient(rice)
	s.AddsIngredient(yogurt)
	s.WritesRecipe(riceBowl())
	s.Counts("rice", "", "800", "")
	s.AddsPortions(lasagne)
}

// idIn is the id the pattern's group finds in the prompt of the first request of the
// Run req belongs to.
func idIn(t *testing.T, req agent.Request, pattern string) int64 {
	t.Helper()
	prompt := req.Messages[0].Content[0].Text
	m := regexp.MustCompile(pattern).FindStringSubmatch(prompt)
	if m == nil { // not Fatalf: this runs in the Run's goroutine
		t.Errorf("the prompt has no %q:\n%s", pattern, prompt)
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// fridayDraft is a draft Plan of friday, read from the prompt of req: yogurt at
// breakfast, a Rice bowl prepared at lunch for three Portions at 1.25×, eaten at lunch
// and dinner with the third frozen, and a Portion of lasagne at the snack.
func fridayDraft(t *testing.T, req agent.Request) map[string]any {
	t.Helper()
	bowl := idIn(t, req, `Recipe (\d+): Rice bowl`)
	pot := idIn(t, req, `Ingredient (\d+): yogurt`)
	lasagne := idIn(t, req, `Portion (\d+): lasagne`)
	return map[string]any{
		"prep_sessions": []map[string]any{
			{"on": "2026-11-06", "kind": "breakfast", "ingredient_id": pot, "quantity": 150, "portions": []map[string]any{
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "breakfast"},
			}},
			{"on": "2026-11-06", "kind": "lunch", "recipe_id": bowl, "size": 1.25, "portions": []map[string]any{
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "lunch"},
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "dinner"},
				{"location": "freezer"},
			}},
		},
		"kept": []map[string]any{{"portion_id": lasagne, "on": "2026-11-06", "kind": "snack"}},
	}
}

// Opening a Cycle lands on the Kitchen Check, listing what the kitchen holds with a Count
// for each; its button starts a planning Run whose Trace shows, and which ends in a
// pending Plan Proposal at the top of the Cycle page: each Slot with its Portion, each
// Prep Session with its Portions' size, its Ingredients scaled, its minutes and where
// each Portion goes, and the planner's summary.
func TestPlanningACycleEndsInAPendingPlanProposal(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			return fakellm.CallTool("check_plan", fridayDraft(t, req))
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Rice bowls for Friday, with yogurt and the lasagne."),
	)

	s.OpensCycle(friday)
	s.IsConfirmed("Cycle opened.")
	s.Sees("Kitchen Check")
	s.Sees("rice 800 g")
	s.Sees("lasagne × 3")
	s.Sees("Count rice")

	s.PlansCycle()
	s.Sees("Planning a Cycle Succeeded")
	s.Sees("check_plan")
	s.Sees("Rice bowls for Friday, with yogurt and the lasagne.")

	s.Follows("Plan Proposal")
	s.IsAt("/plan")
	s.Sees("01 Plan Proposal")
	s.Sees("Rice bowls for Friday, with yogurt and the lasagne.")
	s.Sees("02 Upcoming Cycle")
	s.Sees("Friday, November 6")
	s.Sees("Breakfast yogurt")
	s.Sees("Lunch Rice bowl 1.25×")
	s.Sees("Snack lasagne")
	s.Sees("Dinner Rice bowl 1.25×")
	s.Sees("Rice bowl: 3 Portions at 1.25×")
	s.Sees("rice 375 g")
	s.Sees("10 active of 25 minutes")
	s.Sees("Lunch on Friday, November 6 In the fridge")
	s.Sees("No Slot In the freezer")
	s.Sees("yogurt: 1 Portion")
	s.Sees("yogurt 150 g")
}

// prompt is what the planning Run s started first was told.
func prompt(t *testing.T, s *apptest.Scenario) string {
	t.Helper()
	reqs := s.Model.Requests()
	if len(reqs) == 0 {
		t.Fatal("the model was asked nothing")
	}
	return reqs[0].Messages[0].Content[0].Text
}

// promptHolds checks the prompt of the planning Run s started first holds each of want.
func promptHolds(t *testing.T, s *apptest.Scenario, want ...string) {
	t.Helper()
	got := prompt(t, s)
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("the planner isn't told %q:\n%s", w, got)
		}
	}
}

// plansFriday opens friday and plans it, through the Kitchen Check, into a pending Plan
// Proposal of fridayDraft summed up as summary.
func plansFriday(t *testing.T, s *apptest.Scenario, summary string) {
	t.Helper()
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply(summary),
	)
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Sees("Planning a Cycle Succeeded")
}

// A Count made on the Kitchen Check is what the planner is told the kitchen holds.
func TestACountOnTheKitchenCheckIsWhatThePlannerIsTold(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(fakellm.Reply("Nothing to plan."))
	s.OpensCycle(friday)
	check := s.Page().Path
	s.Follows("Count rice")
	form := apptest.Form(t, s.Page().Body, "count-form")
	form.Values.Set("stated", "500")
	s.PostForm(t, form.Action, form.Values)

	s.Open(check)
	s.Sees("rice 500 g")
	s.PlansCycle()
	promptHolds(t, s, "- 500 g rice (Ingredient 1), pantry")
}

// The planner is told the Profile, the Recipe Library, the Ingredients, the Planning
// Brief, what the kitchen holds and the User's Language; its system prompt names no
// Language, so it is the same for everyone.
func TestThePlannerIsToldTheProfileTheLibraryTheBriefTheKitchenAndTheLanguage(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Open("/profile")
	s.SavesFoods("cream", "peanuts")
	s.SavesAppliances([]string{"hob", "air_fryer"}, map[string]string{"air_fryer": "Ninja 5 L"})
	s.SavesBrief("Fish twice a week.")
	s.Model.Script(fakellm.Reply("Nothing to plan."))
	s.OpensCycle(friday)
	s.PlansCycle()

	promptHolds(t, s,
		"The user's Language is English.",
		"Its Slots run from breakfast on Fri 6 Nov to dinner on Fri 6 Nov.",
		"Targets a day: 2200 kcal (within 8%), at least 130 g protein, at least 30 g fibre, at most 50 g sugar.",
		"Excludes: cream.", "Allergic to: peanuts.", "Owns: hob, air_fryer (Ninja 5 L).",
		"- Recipe 1: Rice bowl, 2 Portions as written, each 350 kcal, 7 g protein",
		"10 active of 25 total minutes; for lunch, dinner; needs hob, microwave. Ingredients: 200 g rice (Ingredient 1).",
		"- Ingredient 2: yogurt, in g, usually in the fridge",
		"<brief>\nFish twice a week.\n</brief>",
		"- Portion 1: lasagne, freezer, each 620 kcal", "- Portion 3: lasagne",
		"- 800 g rice (Ingredient 1), pantry",
	)
	if sys := s.Model.Requests()[0].System; strings.Contains(sys, "English") || strings.Contains(sys, "Fish") {
		t.Errorf("the system prompt varies with the User:\n%s", sys)
	}
}

// An empty Planning Brief is told as its low-care default, and an Italian User's planner
// is told their Language.
func TestThePlannerIsToldTheLowCareBriefAndTheLanguage(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.ChangesLanguage("Italiano")
	s.Model.Script(fakellm.Reply("Niente da pianificare."))
	s.OpensCycle(apptest.Cycle{Delivery: "05/11/2026", From: "18:00", To: "20:00", Ends: "06/11/2026"})
	s.PlansCycle()
	promptHolds(t, s, "The user's Language is Italian.", "<brief>\nLow-care Recipes, about 15 active minutes.\n</brief>")
}

// brokenDraft is a draft Plan of friday, read from the prompt of req, breaking every
// structural hard rule: the Rice bowl prepared at dinner is eaten at lunch, before it,
// and on Saturday, after the Cycle; lasagne fills breakfast a second time; and Portion
// 999 doesn't exist, leaving the snack empty.
func brokenDraft(t *testing.T, req agent.Request) map[string]any {
	t.Helper()
	bowl := idIn(t, req, `Recipe (\d+): Rice bowl`)
	pot := idIn(t, req, `Ingredient (\d+): yogurt`)
	lasagne := idIn(t, req, `Portion (\d+): lasagne`)
	return map[string]any{
		"prep_sessions": []map[string]any{
			{"on": "2026-11-06", "kind": "breakfast", "ingredient_id": pot, "quantity": 150, "portions": []map[string]any{
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "breakfast"},
			}},
			{"on": "2026-11-06", "kind": "dinner", "recipe_id": bowl, "size": 1, "portions": []map[string]any{
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "lunch"},
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "dinner"},
				{"location": "freezer", "eat_on": "2026-11-07", "eat_kind": "lunch"},
			}},
		},
		"kept": []map[string]any{
			{"portion_id": lasagne, "on": "2026-11-06", "kind": "breakfast"},
			{"portion_id": 999, "on": "2026-11-06", "kind": "snack"},
		},
	}
}

// check_plan reports every structural hard rule a draft breaks, and refuses one that
// can't be read before any rule: a Recipe that doesn't exist, a size outside 0.75–1.5.
func TestCheckPlanReportsEachStructuralHardRule(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", brokenDraft(t, req)) }),
		fakellm.CallTool("check_plan", map[string]any{"prep_sessions": []map[string]any{
			{"on": "2026-11-06", "kind": "lunch", "recipe_id": 999, "size": 1, "portions": []map[string]any{{"location": "fridge"}}},
			{"on": "2026-11-06", "kind": "dinner", "recipe_id": 1, "size": 2, "portions": []map[string]any{{"location": "fridge"}}},
		}}),
		fakellm.Reply("I give up."),
	)
	s.OpensCycle(friday)
	s.PlansCycle()

	s.Sees("Draft 1 breaks these hard rules:")
	s.Sees("eaten-before-prepared: Rice bowl is eaten at lunch on Fri 6 Nov, before it is prepared at dinner on Fri 6 Nov")
	s.Sees("outside-cycle: Rice bowl is eaten at lunch on Sat 7 Nov, outside the Slots planned")
	s.Sees("slot-filled-twice: breakfast on Fri 6 Nov is filled twice: by yogurt and by Portion 1 (lasagne)")
	s.Sees("unknown-portion: Portion 999 put in snack on Fri 6 Nov is not a Portion without a Slot")
	s.Sees("empty-slot: snack on Fri 6 Nov is empty")
	s.Sees("the draft can't be read")
	s.Sees("prep_sessions.0: there is no Recipe 999")
	s.Sees("Rice bowl at dinner on Fri 6 Nov has Portions of size 2, outside 0.75–1.5")
	s.DoesNotSee("Draft 2")
}

// Three submitted drafts that break a hard rule fail the Run, listing what they broke.
func TestThreeSubmitsBreakingHardRulesFailThePlanningRun(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	submit := fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1})
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", brokenDraft(t, req)) }),
		submit, submit, submit,
	)
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Sees("Planning a Cycle Failed")
	s.Sees("submit 2 of 3")
	s.Sees("The planner submitted three Plans breaking hard rules: Draft 1: - eaten-before-prepared")
	s.DoesNotSee("Plan Proposal")

	s.Open("/plan")
	s.DoesNotSee("Plan Proposal")
	s.Sees("Plan again")
}

// A planning Run whose model ends without submitting a Plan fails, its last message as
// the reason.
func TestAPlanningRunEndingWithoutAPlanProposalFailsWithTheModelsLastMessage(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(fakellm.Reply("The kitchen is empty; I can't plan this."))
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Sees("Planning a Cycle Failed")
	s.Sees("The model ended without a Plan Proposal. The kitchen is empty; I can't plan this.")
}

// Once its calls are spent, check_plan says to submit the best draft, which submit_plan
// still stores.
func TestCheckPlansCallsRunOut(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com", apptest.WithConfig(func(c *app.Config) { c.PlanChecks = 2 }))
	check := fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) })
	s.Model.Script(check, check, check, fakellm.CallTool("submit_plan", map[string]any{"draft_id": 2}), fakellm.Reply("Friday."))
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Sees("Draft 2 keeps every hard rule. 0 of 2 check_plan calls left.")
	s.Sees("the check_plan budget of 2 calls is spent: submit the best draft you have with submit_plan")
	s.Sees("Planning a Cycle Succeeded")
}

// A second planning Run is refused while one is going, and while a Plan Proposal is
// pending; cancelling a planning Run leaves no Plan Proposal.
func TestASecondPlanningRunIsRefusedWhileOneIsGoingOrAPlanProposalIsPending(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	asked := make(chan struct{})
	s.Model.Script(asksAgain(asked, fakellm.Hang()))
	s.OpensCycle(friday)
	check := s.Page().Path
	s.StartsPlanning()
	run := s.Page().Path
	<-asked

	s.Open("/plan")
	s.Sees("Being planned: see its Trace")
	s.Open(check)
	s.StartsPlanning()
	if s.Page().StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a second planning Run = %d, want %d", s.Page().StatusCode, http.StatusUnprocessableEntity)
	}
	s.Sees("Another planning Run is going, or a Plan Proposal is waiting for you.")

	s.Open(run)
	s.CancelsRun(apptest.Form(t, s.Page().Body, "cancel-run"))
	s.Sees("Planning a Cycle Cancelled")
	s.Open("/plan")
	s.DoesNotSee("Plan Proposal")
	s.DoesNotSee("Plan again")
	if got := s.PostForm(t, strings.Replace(check, "/check", "/again", 1), nil); got.StatusCode != http.StatusNotFound {
		t.Errorf("planning again after a cancelled Run = %d, want 404: it goes through the Kitchen Check", got.StatusCode)
	}

	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Friday."),
	)
	s.Follows("Plan the Cycle")
	s.PlansCycle()
	s.Open("/plan")
	s.Sees("Plan Proposal")
	s.Sees("Friday.")
	s.DoesNotSee("Plan the Cycle")

	s.Open(check)
	s.StartsPlanning()
	s.Sees("Another planning Run is going, or a Plan Proposal is waiting for you.")
}

// Two Users never see each other's Plan Proposals, and one's pending Plan Proposal
// doesn't keep the other from planning.
func TestUsersSeeOnlyTheirOwnPlanProposals(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Ada's Friday.")

	s.Invite("bea@example.com")
	s.SwitchesTo("bea@example.com")
	s.ChoosesLanguage("English")
	s.Open("/plan")
	s.DoesNotSee("Ada's Friday.")
	s.Open("/profile")
	s.SavesTargets(targets)
	stockKitchen(s)
	plansFriday(t, s, "Bea's Friday.")
	s.Open("/plan")
	s.Sees("Plan Proposal")
	s.Sees("Bea's Friday.")
	s.DoesNotSee("Ada's Friday.")

	s.SwitchesTo("ada@example.com")
	s.Open("/plan")
	s.Sees("Plan Proposal")
	s.Sees("Ada's Friday.")
	s.DoesNotSee("Bea's Friday.")
}

// Through a browser: opening a Cycle lands on the Kitchen Check, whose button starts the
// planning Run on its live Trace, which goes on to the Cycle page and its pending Plan
// Proposal once the Run ends; approving it shows the Plan.
func TestBrowserPlansACycleThroughTheKitchenCheck(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	fill := func(selector, value string) {
		t.Helper()
		if err := b.Locator(selector).Fill(value); err != nil {
			t.Fatalf("fill %s: %v", selector, err)
		}
	}
	choose := func(selector, value string) {
		t.Helper()
		if _, err := b.Locator(selector).SelectOption(playwright.SelectOptionValues{Values: &[]string{value}}); err != nil {
			t.Fatalf("choose %s: %v", selector, err)
		}
	}
	b.Open(t, "/profile")
	for field, v := range map[string]string{"kcal": "2200", "protein": "130", "fibre": "30", "sugar": "50"} {
		fill("#profile-targets [name="+field+"]", v)
	}
	if err := b.Locator("#profile-targets button[type=submit]").Click(); err != nil {
		t.Fatalf("save the Targets: %v", err)
	}
	b.IsConfirmed(t, "Profile saved.")
	b.Open(t, "/ingredients/new")
	fill("#ingredient-add [name=name]", "yogurt")
	choose("#ingredient-add [name=unit]", "g")
	choose("#ingredient-add [name=usual]", "fridge")
	for field, v := range map[string]string{"kcal": "60", "protein": "10", "carbs": "4", "fat": "0"} {
		fill("#ingredient-add [name="+field+"]", v)
	}
	if err := b.Locator("#ingredient-add button[type=submit]").Click(); err != nil {
		t.Fatalf("add yogurt: %v", err)
	}
	b.IsConfirmed(t, "Added yogurt.")

	eaten := func(kind string) map[string]any {
		return map[string]any{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": kind}
	}
	release := make(chan struct{})
	h.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			return fakellm.CallTool("check_plan", map[string]any{"prep_sessions": []map[string]any{{
				"on": "2026-11-06", "kind": "breakfast", "ingredient_id": idIn(t, req, `Ingredient (\d+): yogurt`), "quantity": 150,
				"portions": []map[string]any{eaten("breakfast"), eaten("lunch"), eaten("snack"), eaten("dinner")},
			}}})
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}).After(release),
		fakellm.Reply("Yogurt all Friday."),
	)
	b.Open(t, "/plan/open")
	fill("[name=delivery]", "11/05/2026")
	fill("[name=from]", "6pm")
	fill("[name=to]", "8pm")
	fill("[name=ends]", "11/06/2026")
	b.Click(t, "Open the Cycle")
	b.IsAt(t, "/plan/cycles/1/check")
	b.Sees(t, "Kitchen Check")
	b.Shot(t, "kitchen-check")

	b.Click(t, "Confirm the kitchen and plan")
	b.IsAt(t, "/runs/1")
	b.Sees(t, "check_plan")
	b.Shot(t, "planning-run")
	close(release)
	b.IsAt(t, "/plan")
	b.Sees(t, "Plan Proposal")
	b.Sees(t, "Yogurt all Friday.")
	b.Sees(t, "yogurt: 4 Portions")
	b.ShotPage(t, "plan-proposal")

	b.Click(t, "Approve")
	b.IsConfirmed(t, "Plan approved.")
	b.Sees(t, "8:00 am Breakfast yogurt")
	b.ShotPage(t, "plan")
}

// Cancelling a planning Run once its Plan is submitted, as the model sums it up, leaves
// no Plan Proposal: it is stored only with the Run's success.
func TestCancellingAPlanningRunAfterItsSubmitLeavesNoPlanProposal(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	asked := make(chan struct{})
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		asksAgain(asked, fakellm.Hang()),
	)
	s.OpensCycle(friday)
	s.StartsPlanning()
	run := s.Page().Path
	<-asked
	s.Open(run)
	s.Sees("is stored as the Plan Proposal")
	s.CancelsRun(apptest.Form(t, s.Page().Body, "cancel-run"))
	s.Sees("Planning a Cycle Cancelled")
	s.Open("/plan")
	s.DoesNotSee("Plan Proposal")
	s.Sees("Plan the Cycle")
}

// Dropping a Cycle while its planning Run is going cancels the Run, which then stores no
// Plan Proposal.
func TestDroppingACycleCancelsItsPlanningRun(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	release := make(chan struct{})
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Friday.").After(release),
	)
	s.OpensCycle(friday)
	s.StartsPlanning()
	run := s.Page().Path
	s.DropsCycle()
	close(release)
	s.Settle()
	s.Open(run)
	s.Sees("Planning a Cycle Cancelled")
	s.Open("/plan")
	s.Sees("No Cycle is upcoming.")
}
