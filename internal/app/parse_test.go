package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
)

// pastedLasagne is a recipe as a User pastes it.
const pastedLasagne = `Lasagne for 4
500g beef mince, 250g lasagne sheets, 1 tin chopped tomatoes, a splash of olive oil.
Brown the mince. Layer with the sheets and tomatoes. Bake 40 minutes.`

// submittedLasagne is what the model submits for pastedLasagne: rice is known, the rest
// is new.
func submittedLasagne() map[string]any {
	return map[string]any{
		"name": "Lasagne", "portions": 4, "active_minutes": 30, "total_minutes": 70,
		"slot_kinds": []string{"lunch", "dinner"},
		"appliances": []string{"oven", "hob"},
		"steps":      []string{"Brown the mince.", "Layer with the sheets and tomatoes.", "Bake 40 minutes."},
		"ingredients": []map[string]any{
			{"name": "beef mince", "quantity": 500, "unit": "g", "new": map[string]any{
				"kcal": 250, "protein_g": 26, "carbs_g": 0, "fat_g": 17, "fibre_g": 0, "sugar_g": 0,
				"usual": "fridge", "staple": false}},
			{"name": "lasagne sheets", "quantity": 250, "unit": "g", "new": map[string]any{
				"kcal": 350, "protein_g": 12, "carbs_g": 70, "fat_g": 1.5, "fibre_g": nil, "sugar_g": nil,
				"usual": "pantry", "staple": false}},
			{"name": "chopped tomatoes", "quantity": 240, "unit": "g", "new": map[string]any{
				"kcal": 20, "protein_g": 1, "carbs_g": 3.5, "fat_g": 0.2, "fibre_g": 1, "sugar_g": 3,
				"usual": "pantry", "staple": false}},
			{"name": "olive oil", "quantity": 10, "unit": "ml", "new": map[string]any{
				"kcal": 820, "protein_g": 0, "carbs_g": 0, "fat_g": 91, "fibre_g": 0, "sugar_g": 0,
				"usual": "pantry", "staple": true}},
			{"name": "rice", "quantity": 100, "unit": "g"},
		},
	}
}

// A pasted recipe is read by a parse Run, whose page shows its Trace, into a Recipe
// Draft showing what the model submitted: the known rice in its own unit, and each new
// Ingredient's proposed values, fibre and sugar unknown where the model couldn't tell.
func TestAPastedRecipeBecomesARecipeDraftThroughARun(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Model.Script(
		fakellm.CallTool("submit_recipe", submittedLasagne()),
		fakellm.Reply("Parsed the lasagne."),
	)

	s.PastesRecipe(pastedLasagne)
	s.Sees("Reading a Recipe")
	s.Sees("Succeeded")
	s.Sees("500g beef mince, 250g lasagne sheets")
	s.Sees("submit_recipe")
	s.Sees("Parsed the lasagne.")

	s.Follows("Recipe Draft")
	s.Sees("Lasagne")
	s.Sees("Makes 4 Portions Active 30 min Total 70 min Eaten at lunch, dinner Needs oven, hob")
	s.Sees("rice 100 g")
	s.Sees("beef mince 500 g New 250 kcal protein 26 g carbs 0 g fat 17 g fibre 0 g sugar 0 g fridge")
	s.Sees("lasagne sheets 250 g New 350 kcal protein 12 g carbs 70 g fat 1.5 g fibre – unknown sugar – unknown pantry")
	s.Sees("olive oil 10 ml New 820 kcal protein 0 g carbs 0 g fat 91 g fibre 0 g sugar 0 g pantry Staple")
	s.Sees("Brown the mince. Layer with the sheets and tomatoes. Bake 40 minutes.")
}

// What submit_recipe can't take goes back to the model, said so it can fix it: here the
// known rice in millilitres, and a new Ingredient without its values. The model fixes
// both, and the Recipe Draft is what it submitted second.
func TestSubmitRecipesProblemsGoBackToTheModelWhichFixesThem(t *testing.T) {
	s := kitchenWith(t, rice)
	wrong := submittedLasagne()
	ings := wrong["ingredients"].([]map[string]any)
	ings[4] = map[string]any{"name": "Rice", "quantity": 100, "unit": "ml"}
	delete(ings[0], "new")
	s.Model.Script(
		fakellm.CallTool("submit_recipe", wrong),
		fakellm.Then(func(req agent.Request) fakellm.Step {
			refused := lastToolResult(req)
			for _, want := range []string{
				"ingredients.0.new: beef mince is not a known Ingredient; propose its values in new",
				"ingredients.4.unit: rice is a known Ingredient measured in g; give its quantity in g",
			} {
				if !refused.IsError || !strings.Contains(refused.Text, want) {
					t.Errorf("the model was told %q, want it told %q", refused.Text, want)
				}
			}
			return fakellm.CallTool("submit_recipe", submittedLasagne())
		}),
		fakellm.Reply("Fixed the lasagne."),
	)

	s.PastesRecipe(pastedLasagne)
	s.Sees("submit_recipe refuses")
	s.Sees("submit_recipe answers")
	s.Follows("Recipe Draft")
	s.Sees("rice 100 g")
	s.Sees("beef mince 500 g New 250 kcal")
}

// lastToolResult is the last tool result the model was sent.
func lastToolResult(req agent.Request) agent.Block {
	last := req.Messages[len(req.Messages)-1]
	return last.Content[len(last.Content)-1]
}

// italianLasagne is what the model submits for pastedLasagne in Italian.
func italianLasagne() map[string]any {
	return map[string]any{
		"name": "Lasagne", "portions": 4, "active_minutes": 30, "total_minutes": 70,
		"slot_kinds": []string{"dinner"}, "appliances": []string{"forno"},
		"steps": []string{"Rosola la carne.", "Alterna sfoglia e pomodoro.", "Cuoci 40 minuti."},
		"ingredients": []map[string]any{
			{"name": "carne macinata", "quantity": 500, "unit": "g", "new": map[string]any{
				"kcal": 250, "protein_g": 26, "carbs_g": 0, "fat_g": 17, "fibre_g": nil, "sugar_g": nil,
				"usual": "fridge", "staple": false}},
		},
	}
}

// The parse Run's system prompt names the User's Language, so a recipe pasted in English
// by a User speaking Italian becomes a Recipe Draft in Italian.
func TestARecipeInAnotherLanguageBecomesARecipeDraftInTheUsersLanguage(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("bea@example.com")
	s.SignsIn("bea@example.com")
	s.ChoosesLanguage("Italiano")
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			if !strings.Contains(req.System, "Write the Recipe name, Ingredient names, steps and appliances in Italian.") {
				t.Errorf("the system prompt does not name Italian:\n%s", req.System)
			}
			return fakellm.CallTool("submit_recipe", italianLasagne())
		}),
		fakellm.Reply("Ho letto la lasagna."),
	)

	s.PastesRecipe(pastedLasagne)
	s.Sees("Lettura di una Ricetta")
	s.Follows("Bozza di Ricetta")
	s.Sees("Dosi 4 Porzioni Attivi 30 min Totale 70 min Si mangia a cena Richiede forno")
	s.Sees("carne macinata 500 g Nuovo 250 kcal proteine 26 g carboidrati 0 g grassi 17 g fibre – non noto zuccheri – non noto frigo")
	s.Sees("Rosola la carne.")
}

// The system prompt is the same for every User and paste of one Language, so it is
// cached; what varies, the User's known Ingredients and the pasted text, comes after it.
func TestTheParseSystemPromptVariesOnlyByLanguage(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Invite("bob@example.com")
	s.Model.Script(fakellm.Reply("Not a recipe."), fakellm.Reply("Not a recipe."))
	s.PastesRecipe(pastedLasagne)
	s.SwitchesTo("bob@example.com")
	s.ChoosesLanguage("English")
	s.PastesRecipe("Two eggs, boiled.")

	reqs := s.Model.Requests()
	if reqs[0].System != reqs[1].System {
		t.Errorf("two Users' parse Runs had different system prompts:\n%s\n---\n%s", reqs[0].System, reqs[1].System)
	}
	for i, want := range [][]string{{"- rice (g)", pastedLasagne}, {"(none yet)", "Two eggs, boiled."}} {
		prompt := reqs[i].Messages[0].Content[0].Text
		for _, w := range want {
			if !strings.Contains(prompt, w) {
				t.Errorf("parse Run %d's prompt does not hold %q:\n%s", i+1, w, prompt)
			}
		}
	}
}

// A Trace's fixed lines are in the User's Language, whichever it is when they look, and
// what the model said is as it said it.
func TestATracesFixedLinesAreInTheUsersLanguageAndTheModelsTextVerbatim(t *testing.T) {
	s := kitchenWith(t)
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Parsed *the* lasagne."))
	s.PastesRecipe(pastedLasagne)
	run := s.Page().Path
	s.Sees("Reading a Recipe Succeeded")
	s.Sees("Started with " + runs.DefaultModel)
	s.Sees("Asking the model (turn 1)")
	s.Sees("Calls submit_recipe")
	s.Sees("The model says Parsed the lasagne.")
	s.Sees("Ended: Succeeded")
	if !strings.Contains(s.Page().Body, "Parsed <em>the</em> lasagne.") {
		t.Errorf("the model's Markdown is not shown as it wrote it:\n%s", s.Page().Body)
	}

	s.Open("/profile")
	s.ChangesLanguage("Italiano")
	s.Open(run)
	s.Sees("Lettura di una Ricetta Riuscita")
	s.Sees("Iniziata con " + runs.DefaultModel)
	s.Sees("Chiedo al modello (turno 1)")
	s.Sees("Chiama submit_recipe")
	s.Sees("Il modello dice Parsed the lasagne.")
	s.Sees("Finita: Riuscita")
}

// Several parse Runs can go at once, each making its own Recipe Draft: both are asking
// the model before either is answered.
func TestSeveralParseRunsGoAtOnceEachMakingItsOwnRecipeDraft(t *testing.T) {
	s := kitchenWith(t, rice)
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	submit := fakellm.Then(func(req agent.Request) fakellm.Step {
		arrived <- struct{}{}
		r := submittedLasagne()
		if strings.Contains(req.Messages[0].Content[0].Text, "Moussaka") {
			r["name"] = "Moussaka"
		}
		return fakellm.CallTool("submit_recipe", r).After(release)
	})
	s.Model.Script(submit, submit, fakellm.Reply("Done."), fakellm.Reply("Done."))

	var runs []string
	for _, text := range []string{pastedLasagne, "Moussaka for 4\n" + pastedLasagne} {
		form := apptest.Form(t, s.Get(t, "/recipes").Body, "paste-form")
		form.Values.Set("text", text)
		runs = append(runs, s.PostForm(t, form.Action, form.Values).Path)
	}
	<-arrived
	<-arrived
	close(release)
	s.Settle()

	for i, name := range []string{"Lasagne", "Moussaka"} {
		s.Open(runs[i])
		s.Follows("Recipe Draft")
		s.Sees(name)
	}
}

// A User sees neither another's Runs nor their Recipe Drafts.
func TestTwoUsersNeverSeeEachOthersRunsOrRecipeDrafts(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Invite("bob@example.com")
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Done."))
	s.PastesRecipe(pastedLasagne)
	run := s.Page().Path
	draft := apptest.Link(t, s.Page().Body, "Recipe Draft")

	s.SwitchesTo("bob@example.com")
	s.ChoosesLanguage("English")
	for _, path := range []string{run, draft} {
		if res := s.Get(t, path); res.StatusCode != http.StatusNotFound {
			t.Errorf("another User opening %s got %d, want 404", path, res.StatusCode)
		}
	}
}

// A Recipe Draft shows only once its parse Run has ended, since the model may still
// change it.
func TestARecipeDraftShowsOnlyOnceItsRunHasEnded(t *testing.T) {
	s := kitchenWith(t, rice)
	release := make(chan struct{})
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Done.").After(release))
	form := apptest.Form(t, s.Get(t, "/recipes").Body, "paste-form")
	form.Values.Set("text", pastedLasagne)
	s.PostForm(t, form.Action, form.Values)
	var id int64
	for id == 0 { // the draft is saved before the model's last turn, which waits on release
		s.Scan(t, &id, `SELECT coalesce(max(id), 0) FROM recipe_draft`)
	}

	draft := fmt.Sprintf("/recipe-drafts/%d", id)
	if res := s.Get(t, draft); res.StatusCode != http.StatusNotFound {
		t.Errorf("the Recipe Draft of a Run going opened with %d, want 404", res.StatusCode)
	}
	close(release)
	s.Settle()
	s.Open(draft)
	s.Sees("beef mince 500 g")
}

// A Run's page folds what the User gave it, the paste, at its top, and its Trace reads as
// a list of timestamped lines: what the model was sent and what it and its tools said is
// folded under each, closed until the User opens it.
func TestBrowserARunsInputAndTraceDataAreFoldedUntilOpened(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	// Chevrons turn at once, so a shot shows where they end.
	if err := b.EmulateMedia(playwright.PageEmulateMediaOptions{ReducedMotion: playwright.ReducedMotionReduce}); err != nil {
		t.Fatal(err)
	}
	r := submittedLasagne()
	r["ingredients"] = r["ingredients"].([]map[string]any)[:4] // Ada has no rice
	h.Model.Script(fakellm.CallTool("submit_recipe", r), fakellm.Reply("Parsed the lasagne."))

	b.Open(t, "/recipes")
	if err := b.GetByLabel("Paste a recipe").Fill(pastedLasagne); err != nil {
		t.Fatal(err)
	}
	b.Click(t, "Read it")
	b.FollowsRun(t)
	expect := playwright.NewPlaywrightAssertions()
	paste := b.Locator("pre", playwright.PageLocatorOptions{HasText: "Lasagne for 4"}).First()
	prompt := b.GetByText("Known Ingredients:")
	input := b.GetByText("\"active_minutes\": 30")
	for _, hidden := range []playwright.Locator{paste, prompt, input} {
		if err := expect.Locator(hidden).ToBeHidden(); err != nil {
			t.Errorf("a Run's page shows what is folded before it is opened: %v", err)
		}
	}
	b.Shot(t, "run-closed")

	for _, line := range []string{"Pasted recipe", "Asking the model (turn 1)", "Calls submit_recipe"} {
		if err := b.GetByText(line, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).Click(); err != nil {
			t.Fatal(err)
		}
	}
	for _, shown := range []playwright.Locator{paste, prompt, input} {
		if err := expect.Locator(shown).ToBeVisible(); err != nil {
			t.Errorf("a Run's page doesn't show what is folded once it is opened: %v", err)
		}
	}
	b.ShotPage(t, "run-open")
}

// scriptedKitchen signs a new User in, speaking the Language named language, on an app
// whose Runs ask the scripted model mise run dev uses.
func scriptedKitchen(t *testing.T, language string) *apptest.Scenario {
	t.Helper()
	s := apptest.NewScenario(t, apptest.WithConfig(func(c *app.Config) {
		c.Runs = runs.Config{LLM: recipes.ScriptedModel{}, Model: "a scripted model"}
	}))
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage(language)
	return s
}

// The scripted model makes a Recipe Draft of any paste without a real model: named after
// its first line, and taking an Ingredient the User knows in the unit they know it in.
func TestTheScriptedModelReadsAnyPasteIntoARecipeDraft(t *testing.T) {
	s := scriptedKitchen(t, "English")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{Name: "Olive oil", Unit: "g", Usual: "pantry", Kcal: "900", Protein: "0", Carbs: "0", Fat: "100"})

	s.PastesRecipe("  Nonna's ragù\nSlow-cooked for hours.")
	s.Sees("Started with a scripted model")
	s.Sees("Calls submit_recipe")
	s.Follows("Recipe Draft")
	s.Sees("Nonna's ragù")
	s.Sees("Olive oil 10 g")
	s.Sees("chickpeas 240 g New 120 kcal protein 7 g carbs 16 g fat 2.5 g fibre – unknown")
}

// The scripted model writes in the Language the system prompt names.
func TestTheScriptedModelWritesInTheUsersLanguage(t *testing.T) {
	s := scriptedKitchen(t, "Italiano")
	s.PastesRecipe("Ragù della nonna")
	s.Follows("Bozza di Ricetta")
	s.Sees("Ragù della nonna")
	s.Sees("pasta secca 160 g Nuovo")
}
