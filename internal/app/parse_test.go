package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
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
	s.Sees("Recipe Draft Name Portions")
	draftHolds(t, s, map[string]string{
		"name": "Lasagne", "portions": "4", "active": "30", "total": "70", "slots": "lunch\ndinner", "appliances": "oven\nhob",
		"ing_name_4": "rice", "ing_quantity_4": "100", "ing_kcal_4": "",
		"ing_name_0": "beef mince", "ing_quantity_0": "500", "ing_unit_0": "g", "ing_usual_0": "fridge", "ing_staple_0": "",
		"ing_kcal_0": "250", "ing_protein_0": "26", "ing_carbs_0": "0", "ing_fat_0": "17", "ing_fibre_0": "0", "ing_sugar_0": "0",
		"ing_name_1": "lasagne sheets", "ing_fat_1": "1.5", "ing_fibre_1": "", "ing_sugar_1": "", "ing_usual_1": "pantry",
		"ing_name_3": "olive oil", "ing_quantity_3": "10", "ing_unit_3": "ml", "ing_kcal_3": "820", "ing_staple_3": "on",
	})
	s.Sees("Measured in g")
	s.Sees("Brown the mince. Layer with the sheets and tomatoes. Bake 40 minutes.")
}

// A refusal that stands after the fallback fails the Run, saying why the model refused.
func TestARefusedPasteFailsItsRunSayingWhy(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Model.Script(fakellm.Respond(agent.Response{StopReason: agent.StopRefusal, Refusal: "bio: Not this one."}))

	s.PastesRecipe(pastedLasagne)
	s.Sees("Failed")
	s.Sees("The model declined to go on.")
	s.Sees("bio: Not this one.")
}

// A model that ends its turn without submitting a Recipe fails the Run, its last message
// as the reason, and no Recipe Draft is offered.
func TestAPasteThatIsNotARecipeFailsItsRunWithTheModelsReason(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Model.Script(fakellm.Reply("This is a shopping list, not a recipe."))

	s.PastesRecipe("Milk, eggs, bread.")
	s.Sees("Reading a Recipe Failed")
	s.Sees("The model ended without a Recipe Draft.")
	s.Sees("This is a shopping list, not a recipe.")
	s.Sees("Ended: Failed")
	if strings.Contains(s.Page().Body, `href="/recipe-drafts/`) {
		t.Errorf("a failed Run's page links a Recipe Draft")
	}
}

// "Try again" on a failed parse Run starts a new one with the same text, without pasting
// it again; a Run that succeeded doesn't offer it.
func TestTryAgainOnAFailedParseRunStartsANewOneWithTheSameText(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Model.Script(
		fakellm.Reply("I lost my place."),
		fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Parsed the lasagne."),
	)
	s.PastesRecipe(pastedLasagne)
	failed := s.Page().Path
	s.Sees("Reading a Recipe Failed")

	s.Submits("try-again")
	if s.Page().Path == failed {
		t.Fatalf("trying again stayed on the failed Run %s", failed)
	}
	s.Settle()
	s.Open(s.Page().Path)
	s.Sees("Reading a Recipe Succeeded")
	s.Sees("Pasted recipe " + strings.Join(strings.Fields(pastedLasagne), " "))
	if strings.Contains(s.Page().Body, `id="try-again"`) {
		t.Errorf("a Run that succeeded offers to try again")
	}
	reqs := s.Model.Requests()
	if len(reqs) < 2 {
		t.Fatalf("the model was asked %d times, want a second Run's request", len(reqs))
	}
	if asked := reqs[1].Messages[0].Content[0].Text; !strings.Contains(asked, pastedLasagne) {
		t.Errorf("trying again asked the model:\n%s\nwant the same text:\n%s", asked, pastedLasagne)
	}
}

// A paste over 20,000 characters, counted after trimming, is refused on its field with
// the limit stated, keeping the text, and starts no Run; one of exactly 20,000 is read.
func TestAPasteOver20000CharactersIsRefusedOnItsField(t *testing.T) {
	s := kitchenWith(t, rice)
	long := strings.Repeat("è", 20001)
	s.StartsParse(long)
	s.IsRefused()
	s.Sees("Paste at most 20,000 characters; this is 20,001.")
	if !strings.Contains(s.Page().Body, long) {
		t.Errorf("the refused paste is not kept in its field")
	}
	if n := len(s.Model.Requests()); n != 0 {
		t.Errorf("a refused paste asked the model %d times", n)
	}
	s.Open("/runs")
	s.Sees("No Runs yet.")

	s.Model.Script(fakellm.Reply("Not a recipe."))
	s.StartsParse("  " + strings.Repeat("è", 20000) + "\n")
	if !strings.HasPrefix(s.Page().Path, "/runs/") {
		t.Errorf("a paste of 20,000 characters lands on %s, want its Run", s.Page().Path)
	}
}

// A model failure worth retrying is retried, the retry shown in the Trace, and the Run
// still succeeds.
func TestAModelFailureWorthRetryingShowsInTheTraceAndTheRunSucceeds(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Model.Script(
		fakellm.Overloaded(),
		fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Parsed the lasagne."),
	)
	s.PastesRecipe(pastedLasagne)
	s.Sees("Reading a Recipe Succeeded")
	s.Sees("Retry 1 of 3 in 1 ms overloaded (529)")
	s.Sees("Ended: Succeeded")
}

// A model that keeps calling tools fails its Run once it reaches the turn limit.
func TestARunPastTheTurnLimitFailsSayingSo(t *testing.T) {
	s := kitchenWith(t, rice)
	for range 50 {
		s.Model.Script(fakellm.CallTool("submit_recipe", map[string]any{}))
	}
	s.PastesRecipe(pastedLasagne)
	s.Sees("Reading a Recipe Failed")
	s.Sees("The model did not finish within 50 turns.")
	if n := len(s.Model.Requests()); n != 50 {
		t.Errorf("the model was asked %d times, want 50", n)
	}
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
	draftHolds(t, s, map[string]string{"ing_name_4": "rice", "ing_quantity_4": "100", "ing_name_0": "beef mince", "ing_kcal_0": "250"})
}

// A kind of appliance not on the list goes back to the model, which picks one that is, so
// the Recipe Draft holds only kinds.
func TestAnUnknownKindOfApplianceGoesBackToTheModel(t *testing.T) {
	s := kitchenWith(t, rice)
	wrong := submittedLasagne()
	wrong["appliances"] = []string{"oven", "ice_cream_maker"}
	s.Model.Script(
		fakellm.CallTool("submit_recipe", wrong),
		fakellm.Then(func(req agent.Request) fakellm.Step {
			refused := lastToolResult(req)
			if want := `appliances: "ice_cream_maker" is not a kind of appliance`; !refused.IsError || !strings.Contains(refused.Text, want) {
				t.Errorf("the model was told %q, want it told %q", refused.Text, want)
			}
			return fakellm.CallTool("submit_recipe", submittedLasagne())
		}),
		fakellm.Reply("Fixed the lasagne."),
	)

	s.PastesRecipe(pastedLasagne)
	s.Sees("submit_recipe refuses")
	s.Follows("Recipe Draft")
	draftHolds(t, s, map[string]string{"appliances": "oven\nhob"})
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
		"slot_kinds": []string{"dinner"}, "appliances": []string{"oven"},
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
			if !strings.Contains(req.System, "Write the Recipe name, Ingredient names and steps in Italian.") {
				t.Errorf("the system prompt does not name Italian:\n%s", req.System)
			}
			return fakellm.CallTool("submit_recipe", italianLasagne())
		}),
		fakellm.Reply("Ho letto la lasagna."),
	)

	s.PastesRecipe(pastedLasagne)
	s.Sees("Lettura di una Ricetta")
	s.Follows("Bozza di Ricetta")
	draftHolds(t, s, map[string]string{
		"name": "Lasagne", "portions": "4", "slots": "dinner", "appliances": "oven",
		"ing_name_0": "carne macinata", "ing_quantity_0": "500", "ing_kcal_0": "250", "ing_fibre_0": "", "ing_usual_0": "fridge",
	})
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

// submit_recipe's schema tells the model the rules its checks hold a Recipe to, so a
// typical one is taken the first time rather than refused: the kinds of appliance, a
// quantity whole but for a piece's, and nutritional values to one decimal place, none
// negative or too large.
func TestSubmitRecipesSchemaStatesTheRulesItChecks(t *testing.T) {
	s := kitchenWith(t)
	s.Model.Script(fakellm.Reply("Not a recipe."))
	s.PastesRecipe(pastedLasagne)

	type field struct {
		Description      string   `json:"description"`
		Minimum          *float64 `json:"minimum"`
		ExclusiveMaximum *float64 `json:"exclusiveMaximum"`
	}
	var schema struct {
		Properties struct {
			Appliances struct {
				Items struct {
					Enum []string `json:"enum"`
				} `json:"items"`
			} `json:"appliances"`
			Ingredients struct {
				Items struct {
					Properties struct {
						Quantity field `json:"quantity"`
						New      struct {
							Properties map[string]field `json:"properties"`
						} `json:"new"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"ingredients"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(s.Model.Requests()[0].Tools[0].InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if got, want := schema.Properties.Appliances.Items.Enum, []string{
		"oven", "hob", "microwave", "air_fryer", "pressure_cooker", "slow_cooker",
		"blender", "food_processor", "stand_mixer", "grill", "toaster", "steamer",
	}; !slices.Equal(got, want) {
		t.Errorf("appliances are %q, want %q", got, want)
	}
	ingredient := schema.Properties.Ingredients.Items.Properties
	if q := ingredient.Quantity; q.Description != "A whole number in g or ml; at most one decimal place in piece." ||
		q.ExclusiveMaximum == nil || *q.ExclusiveMaximum != 1000000 {
		t.Errorf("quantity is described as %+v", q)
	}
	for _, name := range []string{"kcal", "protein_g", "carbs_g", "fat_g", "fibre_g", "sugar_g"} {
		f := ingredient.New.Properties[name]
		if f.Description != "At most one decimal place." || f.Minimum == nil || *f.Minimum != 0 ||
			f.ExclusiveMaximum == nil || *f.ExclusiveMaximum != 10000 {
			t.Errorf("new.%s is described as %+v", name, f)
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
	s.Sees("Reading a Recipe Failed")
	s.Sees("Started with " + runs.DefaultModel)
	s.Sees("Asking the model (turn 1)")
	s.Sees("Calls submit_recipe")
	s.Sees("The model says Parsed the lasagne.")
	s.Sees("The model ended without a Recipe Draft.")
	s.Sees("Ended: Failed")
	if !strings.Contains(s.Page().Body, "Parsed <em>the</em> lasagne.") {
		t.Errorf("the model's Markdown is not shown as it wrote it:\n%s", s.Page().Body)
	}

	s.Open("/profile")
	s.ChangesLanguage("Italiano")
	s.Open(run)
	s.Sees("Lettura di una Ricetta Fallita")
	s.Sees("Iniziata con " + runs.DefaultModel)
	s.Sees("Chiedo al modello (turno 1)")
	s.Sees("Chiama submit_recipe")
	s.Sees("Il modello dice Parsed the lasagne.")
	s.Sees("Il modello ha finito senza una Bozza di Ricetta.")
	s.Sees("Finita: Fallita")
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
		draftHolds(t, s, map[string]string{"name": name})
	}
}

// A User sees neither another's Runs, their streams nor their Recipe Drafts, and can't
// cancel their Runs.
func TestTwoUsersNeverSeeEachOthersRunsOrRecipeDrafts(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Invite("bob@example.com")
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Done."))
	s.PastesRecipe(pastedLasagne)
	run := s.Page().Path
	draft := apptest.Link(t, s.Page().Body, "Recipe Draft")

	s.SwitchesTo("bob@example.com")
	s.ChoosesLanguage("English")
	for _, path := range []string{run, run + "/events", draft} {
		if res := s.Get(t, path); res.StatusCode != http.StatusNotFound {
			t.Errorf("another User opening %s got %d, want 404", path, res.StatusCode)
		}
	}
	if res := s.PostForm(t, run+"/cancel", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("another User cancelling %s got %d, want 404", run, res.StatusCode)
	}
	s.Open("/runs")
	if shown := apptest.Text(s.Page().Body); strings.Contains(shown, "Reading a Recipe") {
		t.Errorf("another User's Runs page lists the Run:\n%s", shown)
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
	draftHolds(t, s, map[string]string{"ing_name_0": "beef mince", "ing_quantity_0": "500"})
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

// demoKitchen signs a new User in, speaking the Language named language, on an app
// whose Runs ask the demo model mise run dev uses.
func demoKitchen(t *testing.T, language string) *apptest.Scenario {
	t.Helper()
	s := apptest.NewScenario(t, apptest.WithConfig(func(c *app.Config) {
		c.Runs = runs.Config{LLM: recipes.DemoLLM{}, Model: "a demo model"}
	}))
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage(language)
	return s
}

// The demo model makes a Recipe Draft of any paste without a real model: named after
// its first line, and taking an Ingredient the User knows in the unit they know it in.
func TestTheDemoLLMReadsAnyPasteIntoARecipeDraft(t *testing.T) {
	s := demoKitchen(t, "English")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{Name: "Olive oil", Unit: "g", Usual: "pantry", Kcal: "900", Protein: "0", Carbs: "0", Fat: "100"})

	s.PastesRecipe("  Nonna's ragù\nSlow-cooked for hours.")
	s.Sees("Started with a demo model")
	s.Sees("Calls submit_recipe")
	s.Follows("Recipe Draft")
	draftHolds(t, s, map[string]string{
		"name": "Nonna's ragù", "ing_name_2": "Olive oil", "ing_quantity_2": "10", "ing_kcal_2": "",
		"ing_name_1": "chickpeas", "ing_quantity_1": "240", "ing_kcal_1": "120", "ing_protein_1": "7", "ing_carbs_1": "16",
		"ing_fat_1": "2.5", "ing_fibre_1": "",
	})
}

// The demo model writes in the Language the system prompt names.
func TestTheDemoLLMWritesInTheUsersLanguage(t *testing.T) {
	s := demoKitchen(t, "Italiano")
	s.PastesRecipe("Ragù della nonna")
	s.Follows("Bozza di Ricetta")
	draftHolds(t, s, map[string]string{"name": "Ragù della nonna", "ing_name_0": "pasta secca", "ing_quantity_0": "160"})
}
