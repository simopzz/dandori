package app_test

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// A kitchen enough to plan ten days: breakfast, three batch dinners eaten again at lunch,
// and snacks. Rice doesn't know its fibre or sugar, so days with it know them only as
// "at least".
var (
	oats      = apptest.Ingredient{Name: "oats", Unit: "g", Usual: "pantry", Kcal: "380", Protein: "13", Carbs: "60", Fat: "7", Fibre: "10", Sugar: "1"}
	greek     = apptest.Ingredient{Name: "greek yogurt", Unit: "g", Usual: "fridge", Kcal: "97", Protein: "9", Carbs: "4", Fat: "5", Fibre: "0", Sugar: "4"}
	apple     = apptest.Ingredient{Name: "apple", Unit: "piece", Usual: "pantry", Kcal: "80", Protein: "0.4", Carbs: "21", Fat: "0.3", Fibre: "4", Sugar: "16"}
	chicken   = apptest.Ingredient{Name: "chicken breast", Unit: "g", Usual: "fridge", Kcal: "120", Protein: "23", Carbs: "0", Fat: "2.6", Fibre: "0", Sugar: "0"}
	courgette = apptest.Ingredient{Name: "courgette", Unit: "g", Usual: "fridge", Kcal: "17", Protein: "1.2", Carbs: "3", Fat: "0.3", Fibre: "1", Sugar: "2.5"}
	oil       = apptest.Ingredient{Name: "olive oil", Unit: "ml", Usual: "pantry", Kcal: "820", Protein: "0", Carbs: "0", Fat: "91", Fibre: "0", Sugar: "0"}
	lentils   = apptest.Ingredient{Name: "red lentils", Unit: "g", Usual: "pantry", Kcal: "330", Protein: "24", Carbs: "50", Fat: "1.5", Fibre: "11", Sugar: "2"}
	passata   = apptest.Ingredient{Name: "passata", Unit: "g", Usual: "pantry", Kcal: "30", Protein: "1.3", Carbs: "5", Fat: "0.2", Fibre: "1.5", Sugar: "4"}
	pasta     = apptest.Ingredient{Name: "pasta", Unit: "g", Usual: "pantry", Kcal: "355", Protein: "12", Carbs: "72", Fat: "1.5", Fibre: "3", Sugar: "3"}
	chickpeas = apptest.Ingredient{Name: "chickpeas", Unit: "g", Usual: "pantry", Kcal: "120", Protein: "7", Carbs: "16", Fat: "2.5", Fibre: "6", Sugar: "0.5"}
)

// row is quantity of the Ingredient named name, as a Recipe's row.
func row(name, quantity string) apptest.RecipeRow {
	return apptest.RecipeRow{Ingredient: apptest.Ingredient{Name: name}, Quantity: quantity}
}

// tenDayRecipes are the Recipe Library ten days are planned from.
var tenDayRecipes = []apptest.Recipe{
	{Name: "Porridge", Portions: "2", Active: "5", Total: "10", Appliances: []string{"hob"}, Slots: []string{"breakfast"},
		Steps: []string{"Simmer the oats.", "Top with yogurt and apple."}, Ingredients: []apptest.RecipeRow{row("oats", "100"), row("greek yogurt", "300"), row("apple", "2")}},
	{Name: "Chicken with rice", Portions: "4", Active: "15", Total: "35", Appliances: []string{"hob"}, Slots: []string{"lunch", "dinner"},
		Steps: []string{"Cook the rice.", "Sear the chicken and courgette."}, Ingredients: []apptest.RecipeRow{row("chicken breast", "600"), row("rice", "320"), row("courgette", "600"), row("olive oil", "20")}},
	{Name: "Lentil soup", Portions: "4", Active: "10", Total: "40", Appliances: []string{"hob"}, Slots: []string{"lunch", "dinner"},
		Steps: []string{"Simmer everything until soft."}, Ingredients: []apptest.RecipeRow{row("red lentils", "300"), row("passata", "400"), row("courgette", "300"), row("olive oil", "20")}},
	{Name: "Pasta with chickpeas", Portions: "4", Active: "10", Total: "20", Appliances: []string{"hob"}, Slots: []string{"lunch", "dinner"},
		Steps: []string{"Boil the pasta.", "Warm the chickpeas in the passata."}, Ingredients: []apptest.RecipeRow{row("pasta", "360"), row("chickpeas", "400"), row("passata", "400"), row("olive oil", "20")}},
	{Name: "Roasted chickpeas", Portions: "2", Active: "5", Total: "30", Appliances: []string{"oven"}, Slots: []string{"snack"},
		Steps: []string{"Roast with the oil."}, Ingredients: []apptest.RecipeRow{row("chickpeas", "400"), row("olive oil", "10")}},
}

// twoDays is a Cycle whose Delivery comes on Monday 2 November between 6 and 8 pm and
// that ends on Wednesday 4: its Slots are Tuesday's and Wednesday's.
var twoDays = apptest.Cycle{Delivery: "11/02/2026", From: "6pm", To: "8pm", Ends: "11/04/2026"}

// tenDays is a Cycle whose Delivery comes on Thursday 5 November between 10 am and noon
// and that ends on Saturday 14: ten days, the first planned from lunch on.
var tenDays = apptest.Cycle{Delivery: "11/05/2026", From: "10am", To: "12pm", Ends: "11/14/2026"}

// tenDayKitchen signs up email, speaking language, with their Targets set, the
// Ingredients and Recipes ten days are planned from, and lasagne in the freezer.
func tenDayKitchen(t *testing.T, email, language string) *apptest.Scenario {
	t.Helper()
	s := apptest.NewScenario(t)
	s.Invite(email)
	s.SignsIn(email)
	s.ChoosesLanguage(language)
	s.Open("/profile")
	s.SavesTargets(targets)
	s.Open("/ingredients")
	for _, ing := range []apptest.Ingredient{oats, greek, apple, chicken, rice, courgette, oil, lentils, passata, pasta, chickpeas} {
		s.AddsIngredient(ing)
	}
	for _, r := range tenDayRecipes {
		s.WritesRecipe(r)
	}
	s.AddsPortions(lasagne)
	return s
}

// approvesTwoDays opens c, twoDays as typed, plans it with twoDayDraft and approves the
// Plan Proposal, then moves the clock past its Delivery, Monday at 9 pm: c is current.
func approvesTwoDays(t *testing.T, s *apptest.Scenario, c apptest.Cycle) {
	t.Helper()
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", twoDayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Two days to start: porridge, a chicken dinner eaten again at lunch, and lentil soup."),
	)
	s.OpensCycle(c)
	s.PlansCycle()
	s.Open("/plan")
	s.Submits("approve")
	s.At("2026-11-02 21:00")
}

// tenDaySummary is what the planner sums tenDayDraft up as.
const tenDaySummary = "Ten days around three batch dinners (chicken with rice, lentil soup and pasta with " +
	"chickpeas), each eaten again at lunch two days later. Porridge every morning, and the lasagne in the freezer " +
	"covers the first two lunches."

// plansTenDays opens c, tenDays as typed, and plans it with tenDayDraft into a pending
// Plan Proposal.
func plansTenDays(t *testing.T, s *apptest.Scenario, c apptest.Cycle) {
	t.Helper()
	s.Model.Script(tenDayScript(t)...)
	s.OpensCycle(c)
	s.PlansCycle()
}

// tenDayScript is the planner checking and submitting tenDayDraft, then summing it up.
func tenDayScript(t *testing.T) []fakellm.Step {
	return []fakellm.Step{
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", tenDayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply(tenDaySummary),
	}
}

// idsIn are the ids the pattern's group finds in the prompt of the first request of the
// Run req belongs to.
func idsIn(t *testing.T, req agent.Request, pattern string) []int64 {
	t.Helper()
	var ids []int64
	for _, m := range regexp.MustCompile(pattern).FindAllStringSubmatch(req.Messages[0].Content[0].Text, -1) {
		id, _ := strconv.ParseInt(m[1], 10, 64)
		ids = append(ids, id)
	}
	return ids
}

// eatenOn is a Portion kept in location and eaten at the kind of Slot on the day.
func eatenOn(day, kind, location string) map[string]any {
	return map[string]any{"location": location, "eat_on": day, "eat_kind": kind}
}

// prep is a Prep Session of the Recipe id at the kind of Slot on the day, of size,
// yielding portions.
func prep(day, kind string, id int64, size float64, portions ...map[string]any) map[string]any {
	return map[string]any{"on": day, "kind": kind, "recipe_id": id, "size": size, "portions": portions}
}

// anApple is a Prep Session of an apple, set out as it is, eaten at the snack on the day.
func anApple(day string, id int64) map[string]any {
	return map[string]any{"on": day, "kind": "snack", "ingredient_id": id, "quantity": 1,
		"portions": []map[string]any{eatenOn(day, "snack", "pantry")}}
}

// twoDayDraft is a draft Plan of twoDays, read from the prompt of req: porridge every
// breakfast, an apple every snack, pasta at Tuesday's lunch, chicken with rice at
// Tuesday's dinner and again at Wednesday's lunch, and lentil soup at Wednesday's dinner.
func twoDayDraft(t *testing.T, req agent.Request) map[string]any {
	t.Helper()
	porridge, apple := idIn(t, req, `Recipe (\d+): Porridge`), idIn(t, req, `Ingredient (\d+): apple`)
	tue, wed := "2026-11-03", "2026-11-04"
	return map[string]any{"prep_sessions": []map[string]any{
		prep(tue, "breakfast", porridge, 1.25, eatenOn(tue, "breakfast", "fridge")),
		prep(wed, "breakfast", porridge, 1.25, eatenOn(wed, "breakfast", "fridge")),
		anApple(tue, apple), anApple(wed, apple),
		prep(tue, "lunch", idIn(t, req, `Recipe (\d+): Pasta with chickpeas`), 1.25, eatenOn(tue, "lunch", "fridge")),
		prep(tue, "dinner", idIn(t, req, `Recipe (\d+): Chicken with rice`), 1.25, eatenOn(tue, "dinner", "fridge"), eatenOn(wed, "lunch", "fridge")),
		prep(wed, "dinner", idIn(t, req, `Recipe (\d+): Lentil soup`), 1.25, eatenOn(wed, "dinner", "fridge")),
	}}
}

// tenDayDraft is a draft Plan of tenDays, read from the prompt of req. Thursday 5 is
// planned from lunch on. Porridge every breakfast; roasted chickpeas and an apple take
// turns at the snack; a dinner each day, of the three batch Recipes in turn, eaten again
// at lunch two days later. Two exceptions: Sunday's chicken is eaten again at Monday's
// lunch, back to back, and the last two dinners' second Portions go in the freezer
// unused. Two Portions of lasagne fill the first two lunches; the third is left unused.
func tenDayDraft(t *testing.T, req agent.Request) map[string]any {
	t.Helper()
	porridge, roast := idIn(t, req, `Recipe (\d+): Porridge`), idIn(t, req, `Recipe (\d+): Roasted chickpeas`)
	dinners := []int64{
		idIn(t, req, `Recipe (\d+): Chicken with rice`),
		idIn(t, req, `Recipe (\d+): Lentil soup`),
		idIn(t, req, `Recipe (\d+): Pasta with chickpeas`),
	}
	apple, lasagne := idIn(t, req, `Ingredient (\d+): apple`), idsIn(t, req, `Portion (\d+): lasagne`)
	on := func(i int) string { return fmt.Sprintf("2026-11-%02d", 5+i) }
	lunchOf := map[int]int{0: 2, 1: 3, 2: 5, 3: 4, 4: 6, 5: 7, 6: 8, 7: 9} // the dinner's day, its lunch's
	var sessions []map[string]any
	for i := range 10 {
		if i > 0 {
			sessions = append(sessions, prep(on(i), "breakfast", porridge, 1.25, eatenOn(on(i), "breakfast", "fridge")))
		}
		if i%2 == 0 {
			sessions = append(sessions, prep(on(i), "snack", roast, 1, eatenOn(on(i), "snack", "pantry")))
		} else {
			sessions = append(sessions, anApple(on(i), apple))
		}
		again := map[string]any{"location": "freezer"}
		if lunch, ok := lunchOf[i]; ok {
			again = eatenOn(on(lunch), "lunch", "fridge")
		}
		sessions = append(sessions, prep(on(i), "dinner", dinners[i%3], 1.25, eatenOn(on(i), "dinner", "fridge"), again))
	}
	return map[string]any{
		"prep_sessions": sessions,
		"kept": []map[string]any{
			{"portion_id": lasagne[0], "on": on(0), "kind": "lunch"},
			{"portion_id": lasagne[1], "on": on(1), "kind": "lunch"},
		},
	}
}

// Through a browser: a ten-day Plan Proposal after an approved Plan, its first day
// planned only from lunch on, shows its soft misses, the seven days before it with the
// approved Plan's two, and every day it plans.
func TestBrowserShowsATenDayPlanProposal(t *testing.T) {
	s := tenDayKitchen(t, "ada@example.com", "English")
	approvesTwoDays(t, s, twoDays)
	plansTenDays(t, s, tenDays)
	s.Sees("Planning a Cycle Succeeded")

	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/plan")
	b.Sees(t, "Chicken with rice eaten back to back, Sun, Nov 8 and Mon, Nov 9")
	b.Sees(t, "lasagne left unused in the freezer")
	b.Sees(t, "Tue, Nov 3")
	b.Sees(t, "Thu, Nov 5, partly planned")
	b.Sees(t, "Saturday, November 14")
	for _, width := range []int{390, 1440} {
		if err := b.SetViewportSize(width, 900); err != nil {
			t.Fatalf("resize to %d: %v", width, err)
		}
		b.ShotPage(t, fmt.Sprintf("ten-days-%d", width))
	}
}
