package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
)

const pastedRecipe = `Lemon chicken with rice — serves 3, about 40 minutes.
600g chicken breast, 240g basmati rice, 2 tbsp olive oil, 1 lemon.
Cook the rice. Brown the chicken in the oil in the air fryer. Squeeze the lemon over.`

// parsedRecipe is what the model submits for pastedRecipe.
func parsedRecipe() map[string]any {
	return map[string]any{
		"name":         "Lemon chicken with rice",
		"portions":     3,
		"prep_minutes": 40,
		"appliances":   []string{"air fryer", "hob"},
		"steps":        []string{"Cook the rice.", "Brown the chicken in the oil in the air fryer.", "Squeeze the lemon over."},
		"ingredients": []map[string]any{
			{"name": "Chicken Breast", "quantity": 600, "unit": "g",
				"nutrition": map[string]any{"kcal": 110, "protein_g": 23, "carbs_g": 0, "fat_g": 1.5}},
			{"name": "basmati rice", "quantity": 240, "unit": "g",
				"nutrition": map[string]any{"kcal": 350, "protein_g": 7.5, "carbs_g": 78, "fat_g": 0.9}},
			{"name": "olive oil", "quantity": 30, "unit": "ml",
				"nutrition": map[string]any{"kcal": 820, "protein_g": 0, "carbs_g": 0, "fat_g": 91}},
			{"name": "lemon", "quantity": 1, "unit": "piece",
				"nutrition": map[string]any{"kcal": 20, "protein_g": 0.6, "carbs_g": 6, "fat_g": 0.2}},
		},
	}
}

// startRecipes starts the app with a fake LLM that answers from steps.
func startRecipes(t *testing.T, steps ...fakellm.Step) (*apptest.Harness, *fakellm.LLM) {
	t.Helper()
	llm := fakellm.New(steps...)
	return apptest.Start(t, apptest.WithPorts(func(p *app.Ports) { p.LLM = llm })), llm
}

// parse pastes text into the Recipe Library, waits for the parse Run to end
// and returns its draft page.
func parse(t *testing.T, h *apptest.Harness, text string) apptest.Response {
	t.Helper()
	res := h.PostForm(t, "/recipes/parse", url.Values{"text": {text}})
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Path, "/recipe-drafts/") {
		t.Fatalf("POST /recipes/parse = %d at %s, want the draft page:\n%s", res.StatusCode, res.Path, res.Body)
	}
	run := "/runs/" + strings.TrimPrefix(res.Path, "/recipe-drafts/")
	h.Stream(t, run+"/events", "").Rest(t)
	return h.Get(t, res.Path)
}

func TestPastedRecipeIsParsedIntoADraftAndSavedOnlyWhenAccepted(t *testing.T) {
	h, _ := startRecipes(t,
		fakellm.CallTool("submit_recipe", parsedRecipe()),
		fakellm.Reply("Parsed the Recipe."),
	)

	draft := parse(t, h, pastedRecipe)
	shown := text(draft.Body)
	for _, want := range []string{
		"air fryer",
		"Brown the chicken in the oil in the air fryer.",
		// 660 + 840 + 246 + 20 kcal over three Portions.
		"Per Portion 589 kcal · 52 g protein",
	} {
		if !strings.Contains(shown, want) {
			t.Errorf("draft does not show %q:\n%s", want, shown)
		}
	}
	form := apptest.Form(t, draft.Body, "recipe")
	for field, want := range map[string]string{
		"name":            "Lemon chicken with rice",
		"prep_minutes":    "40",
		"portions":        "3",
		"ing_name_0":      "chicken breast",
		"ing_quantity_0":  "600",
		"ing_unit_0":      "g",
		"ing_unit_2":      "ml",
		"ing_quantity_3":  "1",
		"ing_unit_3":      "piece",
		"ing_fat_g_2":     "91",
		"ing_protein_g_3": "0.6",
	} {
		if got := form.Get(field); got != want {
			t.Errorf("draft field %s = %q, want %q", field, got, want)
		}
	}
	if lib := text(h.Get(t, "/recipes").Body); strings.Contains(lib, "Lemon chicken") {
		t.Fatalf("an unaccepted draft is already in the Recipe Library:\n%s", lib)
	}

	saved := h.PostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || !strings.HasPrefix(saved.Path, "/recipes/") {
		t.Fatalf("accept = %d at %s, want the Recipe:\n%s", saved.StatusCode, saved.Path, saved.Body)
	}
	recipe := text(saved.Body)
	for _, want := range []string{"Lemon chicken with rice", "Per Portion 589 kcal", "600 g chicken breast", "1 piece lemon"} {
		if !strings.Contains(recipe, want) {
			t.Errorf("saved Recipe does not show %q:\n%s", want, recipe)
		}
	}
	if lib := text(h.Get(t, "/recipes").Body); !strings.Contains(lib, "Lemon chicken with rice") {
		t.Errorf("accepted Recipe is not in the Recipe Library:\n%s", lib)
	}
}

func TestDiscardedDraftSavesNothing(t *testing.T) {
	h, llm := startRecipes(t,
		fakellm.CallTool("submit_recipe", parsedRecipe()),
		fakellm.Reply("Parsed the Recipe."),
	)
	draft := parse(t, h, pastedRecipe)
	discard := apptest.Form(t, draft.Body, "discard")

	res := h.PostForm(t, discard.Action, discard.Values)
	if res.StatusCode != http.StatusOK || res.Path != "/recipes" {
		t.Fatalf("discard = %d at %s, want the Recipe Library", res.StatusCode, res.Path)
	}
	if lib := text(res.Body); strings.Contains(lib, "Lemon chicken") || !strings.Contains(lib, "No Recipes yet.") {
		t.Errorf("a discarded draft shows in the Recipe Library:\n%s", lib)
	}
	if again := h.Get(t, draft.Path); strings.Contains(again.Body, `id="recipe"`) {
		t.Errorf("a discarded draft can still be accepted:\n%s", again.Body)
	}

	// Its Ingredients were never confirmed, so they are still new.
	llm.Script(fakellm.CallTool("submit_recipe", parsedRecipe()), fakellm.Reply("Parsed the Recipe."))
	if again := parse(t, h, pastedRecipe); apptest.Form(t, again.Body, "recipe").Get("ing_new_0") != "true" {
		t.Errorf("a discarded draft's Ingredients were saved:\n%s", again.Body)
	}
}

func TestDraftIsCorrectedBeforeItIsAccepted(t *testing.T) {
	h, _ := startRecipes(t,
		fakellm.CallTool("submit_recipe", parsedRecipe()),
		fakellm.Reply("Parsed the Recipe."),
	)
	form := apptest.Form(t, parse(t, h, pastedRecipe).Body, "recipe")

	form.Values.Set("name", "Lemon chicken")
	form.Values.Set("portions", "2")
	form.Values.Set("ing_quantity_0", "450")
	// Clearing a row removes the lemon; an added row, new until it is named, adds garlic.
	form.Values.Set("ing_new_4", "true")
	form.Values.Set("ing_name_3", "")
	form.Values.Set("ing_quantity_3", "")
	form.Values.Set("ing_name_4", "Garlic")
	form.Values.Set("ing_quantity_4", "2")
	form.Values.Set("ing_unit_4", "piece")
	for field, v := range map[string]string{"kcal": "4", "protein_g": "0,2", "carbs_g": "1", "fat_g": "0"} {
		form.Values.Set("ing_"+field+"_4", v)
	}
	// Cleared steps are dropped; a blank one adds a step.
	form.Values["steps"] = []string{"Cook the rice.", "", "", "Roast the chicken."}

	saved := h.PostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || !strings.HasPrefix(saved.Path, "/recipes/") {
		t.Fatalf("accept = %d at %s, want the Recipe:\n%s", saved.StatusCode, saved.Path, text(saved.Body))
	}
	recipe := text(saved.Body)
	// (450×1.1 + 2.4×350 + 0.3×820 + 2×4) / 2 = (495 + 840 + 246 + 8) / 2 = 794.5
	for _, want := range []string{"Lemon chicken", "Portions 2", "Per Portion 795 kcal", "450 g chicken breast", "2 piece garlic", "Roast the chicken."} {
		if !strings.Contains(recipe, want) {
			t.Errorf("accepted Recipe does not show %q:\n%s", want, recipe)
		}
	}
	for _, unwanted := range []string{"lemon", "Lemon chicken with rice", "Brown the chicken"} {
		if strings.Contains(strings.ReplaceAll(recipe, "Lemon chicken", ""), unwanted) {
			t.Errorf("accepted Recipe still shows %q after it was corrected:\n%s", unwanted, recipe)
		}
	}
}

func TestInvalidDraftIsShownBackAndNothingIsSaved(t *testing.T) {
	tests := []struct {
		name, field, value, message string
	}{
		{"quantity not a number", "ing_quantity_1", "lots", "Must be a number"},
		{"zero quantity", "ing_quantity_1", "0", "Must be more than 0"},
		{"no Portions", "portions", "0", "Must be at least 1"},
		{"missing name", "name", "", "Required"},
		{"new Ingredient's nutrition missing", "ing_kcal_2", "", "Required"},
		{"negative nutrition", "ing_fat_g_2", "-3", "Can't be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := startRecipes(t,
				fakellm.CallTool("submit_recipe", parsedRecipe()),
				fakellm.Reply("Parsed the Recipe."),
			)
			form := apptest.Form(t, parse(t, h, pastedRecipe).Body, "recipe")
			form.Values.Set("appliances", "oven")
			form.Values.Set(tt.field, tt.value)

			res := h.PostForm(t, form.Action, form.Values)
			if res.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("accept status = %d, want 422:\n%s", res.StatusCode, text(res.Body))
			}
			if !strings.Contains(text(res.Body), tt.message) {
				t.Errorf("rejection does not say %q:\n%s", tt.message, text(res.Body))
			}
			shown := apptest.Form(t, res.Body, "recipe")
			if shown.Get(tt.field) != tt.value || shown.Get("appliances") != "oven" {
				t.Errorf("rejected draft does not keep what was typed: %s = %q, appliances = %q", tt.field, shown.Get(tt.field), shown.Get("appliances"))
			}
			if lib := text(h.Get(t, "/recipes").Body); !strings.Contains(lib, "No Recipes yet.") {
				t.Errorf("a rejected draft was saved:\n%s", lib)
			}
		})
	}
}

// acceptFirstRecipe parses and accepts pastedRecipe, so its Ingredients
// are known.
func acceptFirstRecipe(t *testing.T, h *apptest.Harness, llm *fakellm.LLM) {
	t.Helper()
	llm.Script(fakellm.CallTool("submit_recipe", parsedRecipe()), fakellm.Reply("Parsed the Recipe."))
	form := apptest.Form(t, parse(t, h, pastedRecipe).Body, "recipe")
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("accept first Recipe = %d:\n%s", res.StatusCode, text(res.Body))
	}
}

const chickpeaCurry = "Chickpea and chicken curry for 2: 300 g chicken breast, 1 can chickpeas (240 g drained)."

func chickpeaCurryParsed() map[string]any {
	return map[string]any{
		"name": "Chickpea and chicken curry", "portions": 2, "prep_minutes": 30,
		"appliances": []string{"hob"}, "steps": []string{"Simmer everything."},
		"ingredients": []map[string]any{
			// The model's estimate for a known Ingredient is not used.
			{"name": "chicken breast", "quantity": 300, "unit": "g",
				"nutrition": map[string]any{"kcal": 999, "protein_g": 1, "carbs_g": 1, "fat_g": 1}},
			{"name": "chickpeas", "quantity": 240, "unit": "g",
				"nutrition": map[string]any{"kcal": 120, "protein_g": 7, "carbs_g": 17, "fat_g": 2}},
		},
	}
}

func TestParsedIngredientsAreMatchedToKnownIngredients(t *testing.T) {
	h, llm := startRecipes(t)
	acceptFirstRecipe(t, h, llm)

	llm.Script(fakellm.CallTool("submit_recipe", chickpeaCurryParsed()), fakellm.Reply("Parsed."))
	draft := parse(t, h, chickpeaCurry)

	reqs := llm.Requests()
	prompt := reqs[len(reqs)-2].Messages[0].Content[0].Text
	for _, want := range []string{"chicken breast (g)", "olive oil (ml)", "lemon (piece)", chickpeaCurry} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the model is not told %q:\n%s", want, prompt)
		}
	}

	form := apptest.Form(t, draft.Body, "recipe")
	if form.Get("ing_new_0") != "false" || form.Values.Has("ing_kcal_0") {
		t.Errorf("chicken breast is not shown as the known Ingredient: new = %q, kcal = %q", form.Get("ing_new_0"), form.Get("ing_kcal_0"))
	}
	if form.Get("ing_new_1") != "true" || form.Get("ing_kcal_1") != "120" {
		t.Errorf("chickpeas are not shown as a new Ingredient with estimated values: new = %q, kcal = %q", form.Get("ing_new_1"), form.Get("ing_kcal_1"))
	}
	if n := strings.Count(draft.Body, `title="New Ingredient: confirm its nutritional values"`); n != 1 {
		t.Errorf("%d Ingredients are flagged as new, want only chickpeas:\n%s", n, draft.Body)
	}

	saved := h.PostForm(t, form.Action, form.Values)
	// (300×1.1 + 2.4×120) / 2 = (330 + 288) / 2 = 309 kcal: the known 110 kcal per 100 g.
	if recipe := text(saved.Body); !strings.Contains(recipe, "Per Portion 309 kcal") {
		t.Errorf("accepted Recipe does not use the known chicken breast:\n%s", recipe)
	}
}

func TestIngredientThatTurnsKnownOrNewMustBeCheckedAgain(t *testing.T) {
	h, llm := startRecipes(t)
	acceptFirstRecipe(t, h, llm)
	llm.Script(fakellm.CallTool("submit_recipe", chickpeaCurryParsed()), fakellm.Reply("Parsed."))
	form := apptest.Form(t, parse(t, h, chickpeaCurry).Body, "recipe")

	// A new Ingredient renamed to a known one: its typed values would be
	// silently replaced by the known ones.
	form.Values.Set("ing_name_1", "Basmati Rice")
	res := h.PostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "changed between known and new") {
		t.Fatalf("accept = %d, want 422 asking to check the Ingredients again:\n%s", res.StatusCode, text(res.Body))
	}
	shown := apptest.Form(t, res.Body, "recipe")
	if shown.Get("ing_new_1") != "false" || shown.Values.Has("ing_kcal_1") {
		t.Errorf("basmati rice is not shown back as the known Ingredient: new = %q", shown.Get("ing_new_1"))
	}

	// A known Ingredient renamed to a new one: it has no values yet.
	shown.Values.Set("ing_name_0", "turkey breast")
	res = h.PostForm(t, shown.Action, shown.Values)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "changed between known and new") {
		t.Fatalf("accept = %d, want 422 asking to check the Ingredients again:\n%s", res.StatusCode, text(res.Body))
	}
	shown = apptest.Form(t, res.Body, "recipe")
	if shown.Get("ing_new_0") != "true" {
		t.Errorf("turkey breast is not shown back as a new Ingredient: new = %q", shown.Get("ing_new_0"))
	}

	for field, v := range map[string]string{"kcal": "105", "protein_g": "24", "carbs_g": "0", "fat_g": "1"} {
		shown.Values.Set("ing_"+field+"_0", v)
	}
	saved := h.PostForm(t, shown.Action, shown.Values)
	if saved.StatusCode != http.StatusOK || !strings.Contains(text(saved.Body), "300 g turkey breast") {
		t.Errorf("checked Recipe was not saved:\n%s", text(saved.Body))
	}
}

func TestModelIsToldWhatIsWrongWithItsRecipeAndTriesAgain(t *testing.T) {
	h, llm := startRecipes(t)
	acceptFirstRecipe(t, h, llm)

	wrong := chickpeaCurryParsed()
	wrong["ingredients"] = []map[string]any{
		{"name": "chicken breast", "quantity": 1, "unit": "piece"},
		{"name": "chickpeas", "quantity": 240, "unit": "g"},
	}
	llm.Script(
		fakellm.CallTool("submit_recipe", wrong),
		fakellm.CallTool("submit_recipe", chickpeaCurryParsed()),
		fakellm.Reply("Parsed."),
	)
	draft := parse(t, h, chickpeaCurry)

	if form := apptest.Form(t, draft.Body, "recipe"); form.Get("ing_quantity_0") != "300" {
		t.Errorf("draft is not the corrected submission: chicken quantity = %q", form.Get("ing_quantity_0"))
	}
	reqs := llm.Requests()
	last := reqs[len(reqs)-2].Messages
	feedback := last[len(last)-1].Content[0]
	for _, want := range []string{"chicken breast is a known Ingredient measured in g", "chickpeas is a new Ingredient; estimate its nutrition"} {
		if !feedback.IsError || !strings.Contains(feedback.Text, want) {
			t.Errorf("the model is not told %q:\n%+v", want, feedback)
		}
	}
}

func TestModelMustGiveEveryNutritionalValueOfANewIngredient(t *testing.T) {
	partial := chickpeaCurryParsed()
	partial["ingredients"] = []map[string]any{
		{"name": "chickpeas", "quantity": 240, "unit": "g", "nutrition": map[string]any{"kcal": 120}},
	}
	h, llm := startRecipes(t,
		fakellm.CallTool("submit_recipe", partial),
		fakellm.CallTool("submit_recipe", chickpeaCurryParsed()),
		fakellm.Reply("Parsed."),
	)
	draft := parse(t, h, chickpeaCurry)

	reqs := llm.Requests()
	last := reqs[len(reqs)-2].Messages
	if feedback := last[len(last)-1].Content[0]; !feedback.IsError || !strings.Contains(feedback.Text, "protein_g") {
		t.Errorf("the model is not told its nutrition lacks protein_g:\n%+v", feedback)
	}
	// The draft is the full resubmission, not chickpeas with 0 g protein.
	if form := apptest.Form(t, draft.Body, "recipe"); form.Get("ing_protein_g_1") == "0" {
		t.Errorf("draft keeps a missing value as 0: %v", form.Values)
	}
}

// libraryRecipe is the path of the only Recipe in the Recipe Library.
func libraryRecipe(t *testing.T, h *apptest.Harness) string {
	t.Helper()
	page := h.Get(t, "/recipes").Body
	i := strings.Index(page, `href="/recipes/`)
	if i < 0 {
		t.Fatalf("the Recipe Library lists no Recipe:\n%s", page)
	}
	rest := page[i+len(`href="`):]
	return rest[:strings.Index(rest, `"`)]
}

func TestRecipeIsEdited(t *testing.T) {
	h, llm := startRecipes(t)
	acceptFirstRecipe(t, h, llm)
	recipe := libraryRecipe(t, h)

	form := apptest.Form(t, h.Get(t, recipe+"/edit").Body, "recipe")
	if form.Get("name") != "Lemon chicken with rice" || form.Get("ing_new_0") != "false" {
		t.Fatalf("edit form does not show the Recipe with known Ingredients: %v", form.Values)
	}
	form.Values.Set("portions", "4")
	form.Values.Set("ing_name_1", "")
	form.Values.Set("ing_quantity_1", "")

	saved := h.PostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || saved.Path != recipe {
		t.Fatalf("save = %d at %s, want the Recipe at %s:\n%s", saved.StatusCode, saved.Path, recipe, text(saved.Body))
	}
	// (660 + 246 + 20) / 4 = 231.5 kcal, without the rice.
	shown := text(saved.Body)
	for _, want := range []string{"Portions 4", "Per Portion 232 kcal"} {
		if !strings.Contains(shown, want) {
			t.Errorf("edited Recipe does not show %q:\n%s", want, shown)
		}
	}
	if strings.Contains(shown, "basmati rice") {
		t.Errorf("edited Recipe still lists the removed rice:\n%s", shown)
	}

	form.Values.Set("portions", "")
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("invalid edit status = %d, want 422", res.StatusCode)
	}
	if shown := text(h.Get(t, recipe).Body); !strings.Contains(shown, "Portions 4") {
		t.Errorf("an invalid edit changed the Recipe:\n%s", shown)
	}
}

func TestRecipeIsDeleted(t *testing.T) {
	h, llm := startRecipes(t)
	acceptFirstRecipe(t, h, llm)
	recipe := libraryRecipe(t, h)

	res := h.PostForm(t, recipe+"/delete", nil)
	if res.StatusCode != http.StatusOK || res.Path != "/recipes" || !strings.Contains(text(res.Body), "No Recipes yet.") {
		t.Fatalf("delete = %d at %s, want an empty Recipe Library:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	if got := h.Get(t, recipe).StatusCode; got != http.StatusNotFound {
		t.Errorf("GET deleted Recipe = %d, want 404", got)
	}

	// Its Ingredients stay known for other Recipes.
	llm.Script(fakellm.CallTool("submit_recipe", chickpeaCurryParsed()), fakellm.Reply("Parsed."))
	if form := apptest.Form(t, parse(t, h, chickpeaCurry).Body, "recipe"); form.Get("ing_new_0") != "false" {
		t.Errorf("deleting a Recipe forgot its Ingredients")
	}
}

func TestRecipeLibraryIsLinkedFromTheAppShell(t *testing.T) {
	h := apptest.Start(t)
	if page := h.Get(t, "/").Body; !strings.Contains(page, `href="/recipes"`) {
		t.Errorf("app shell does not link to the Recipe Library:\n%s", page)
	}
}

func TestEmptyPasteIsRejected(t *testing.T) {
	h, llm := startRecipes(t)
	res := h.PostForm(t, "/recipes/parse", url.Values{"text": {"  \n "}})
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Paste a recipe to parse") {
		t.Errorf("empty paste = %d, want 422 asking for a recipe:\n%s", res.StatusCode, text(res.Body))
	}
	if n := len(llm.Requests()); n != 0 {
		t.Errorf("an empty paste asked the model %d times", n)
	}
}

func TestParseThatProducesNoDraftSaysSo(t *testing.T) {
	h, _ := startRecipes(t, fakellm.Reply("That is not a recipe."))
	page := parse(t, h, "Remember to call the dentist.")
	shown := text(page.Body)
	if !strings.Contains(shown, "There is no draft to check") || !strings.Contains(shown, "That is not a recipe.") {
		t.Errorf("draft page does not say there is no draft, with the trace:\n%s", shown)
	}
	if strings.Contains(page.Body, `id="recipe"`) {
		t.Errorf("there is a Recipe to accept although none was parsed")
	}
}

func TestDraftPageFollowsTheParseWhileItRuns(t *testing.T) {
	thinking := make(chan struct{})
	h, _ := startRecipes(t,
		fakellm.CallTool("submit_recipe", parsedRecipe()).After(thinking),
		fakellm.Reply("Parsed the Recipe."),
	)
	res := h.PostForm(t, "/recipes/parse", url.Values{"text": {pastedRecipe}})
	defer close(thinking)

	if !strings.Contains(text(res.Body), "being parsed") || !strings.Contains(res.Body, "data-trace-stream") {
		t.Errorf("draft page of a running parse does not follow its trace:\n%s", res.Body)
	}
	if strings.Contains(res.Body, `id="recipe"`) {
		t.Errorf("draft page offers a Recipe before the parse produced one")
	}
}

func TestKnownIngredientInAnotherUnitMustBeRequantified(t *testing.T) {
	h, llm := startRecipes(t)
	// Drafted while olive oil is still new, measured in pieces (a bottle).
	bottle := chickpeaCurryParsed()
	bottle["ingredients"] = []map[string]any{{"name": "olive oil", "quantity": 1, "unit": "piece",
		"nutrition": map[string]any{"kcal": 4000, "protein_g": 0, "carbs_g": 0, "fat_g": 450}}}
	llm.Script(fakellm.CallTool("submit_recipe", bottle), fakellm.Reply("Parsed."))
	draft := parse(t, h, "A bottle of olive oil.")

	// Olive oil becomes known, in ml, before the draft is accepted.
	acceptFirstRecipe(t, h, llm)

	page := h.Get(t, draft.Path)
	form := apptest.Form(t, page.Body, "recipe")
	if !strings.Contains(text(page.Body), "olive oil is measured in ml") || form.Get("ing_quantity_0") != "" {
		t.Errorf("draft does not ask for the olive oil in ml: quantity = %q\n%s", form.Get("ing_quantity_0"), text(page.Body))
	}
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("accept without a quantity in ml = %d, want 422", res.StatusCode)
	}
	form.Values.Set("ing_quantity_0", "15")
	if res := h.PostForm(t, form.Action, form.Values); !strings.Contains(text(res.Body), "15 ml olive oil") {
		t.Errorf("requantified draft was not saved:\n%s", text(res.Body))
	}
}

func TestRowRenamedToAKnownIngredientInAnotherUnitMustBeRequantified(t *testing.T) {
	h, llm := startRecipes(t)
	acceptFirstRecipe(t, h, llm)
	recipe := libraryRecipe(t, h)

	form := apptest.Form(t, h.Get(t, recipe+"/edit").Body, "recipe")
	// The lemon (1 piece) becomes rice, which is measured in grams.
	form.Values.Set("ing_name_3", "basmati rice")
	form.Values.Set("ing_name_1", "")
	form.Values.Set("ing_quantity_1", "")
	res := h.PostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "basmati rice is measured in g") {
		t.Errorf("save = %d, want 422 asking for the quantity in g:\n%s", res.StatusCode, text(res.Body))
	}
}

func TestAmountsMustBeFiniteNumbers(t *testing.T) {
	for _, v := range []string{"NaN", "Inf", "-inf"} {
		t.Run(v, func(t *testing.T) {
			h, _ := startRecipes(t, fakellm.CallTool("submit_recipe", parsedRecipe()), fakellm.Reply("Parsed."))
			form := apptest.Form(t, parse(t, h, pastedRecipe).Body, "recipe")
			form.Values.Set("ing_quantity_0", v)
			res := h.PostForm(t, form.Action, form.Values)
			if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Must be a number") {
				t.Errorf("quantity %s: accept = %d, want 422", v, res.StatusCode)
			}
		})
	}
}

func TestDraftIsAcceptedOnlyOnce(t *testing.T) {
	h, _ := startRecipes(t, fakellm.CallTool("submit_recipe", parsedRecipe()), fakellm.Reply("Parsed."))
	form := apptest.Form(t, parse(t, h, pastedRecipe).Body, "recipe")

	h.PostForm(t, form.Action, form.Values)
	if again := h.PostForm(t, form.Action, form.Values); again.StatusCode != http.StatusNotFound {
		t.Errorf("accepting the same draft again = %d, want 404", again.StatusCode)
	}
	if n := strings.Count(h.Get(t, "/recipes").Body, `href="/recipes/`); n != 1 {
		t.Errorf("the Recipe Library has %d Recipes, want 1", n)
	}
}

func TestDraftIsNotOfferedWhileTheParseIsStillRunning(t *testing.T) {
	finishing := make(chan struct{})
	h, _ := startRecipes(t,
		fakellm.CallTool("submit_recipe", parsedRecipe()),
		fakellm.Reply("Parsed.").After(finishing),
	)
	defer close(finishing)
	res := h.PostForm(t, "/recipes/parse", url.Values{"text": {pastedRecipe}})
	run := "/runs/" + strings.TrimPrefix(res.Path, "/recipe-drafts/")
	readUntil(t, h.Stream(t, run+"/events", ""), "The draft is saved")

	if page := h.Get(t, res.Path); strings.Contains(page.Body, `id="recipe"`) {
		t.Errorf("draft offered for acceptance while the model may still change it")
	}
}

// porridge is a parsed Recipe of oats, named oats.
func porridge(oats string, nutrition map[string]any) map[string]any {
	oatsRow := map[string]any{"name": oats, "quantity": 80, "unit": "g"}
	if nutrition != nil {
		oatsRow["nutrition"] = nutrition
	}
	return map[string]any{
		"name": "Porridge", "portions": 1, "prep_minutes": 5,
		"appliances": []string{"pentolino"}, "steps": []string{"Cuoci i fiocchi d'avena nel latte."},
		"ingredients": []map[string]any{oatsRow},
	}
}

func TestTypographicApostropheMatchesTheKnownIngredient(t *testing.T) {
	h, llm := startRecipes(t)
	llm.Script(
		fakellm.CallTool("submit_recipe", porridge("fiocchi d'avena", map[string]any{"kcal": 370, "protein_g": 13, "carbs_g": 60, "fat_g": 7})),
		fakellm.Reply("Parsed."),
	)
	first := apptest.Form(t, parse(t, h, "Porridge").Body, "recipe")
	if res := h.PostForm(t, first.Action, first.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("accept = %d:\n%s", res.StatusCode, text(res.Body))
	}

	// Typed on a phone: no nutrition, since it is the known Ingredient.
	llm.Script(fakellm.CallTool("submit_recipe", porridge("Fiocchi d’avena", nil)), fakellm.Reply("Parsed."))
	form := apptest.Form(t, parse(t, h, "Porridge di nuovo").Body, "recipe")
	if form.Get("ing_name_0") != "fiocchi d'avena" || form.Get("ing_new_0") != "false" {
		t.Fatalf("the parser does not match fiocchi d’avena to the known Ingredient: name = %q, new = %q", form.Get("ing_name_0"), form.Get("ing_new_0"))
	}

	form.Values.Set("ing_name_0", "fiocchi d‘avena")
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusOK || !strings.Contains(text(res.Body), "80 g fiocchi d'avena") {
		t.Errorf("the Recipe form does not match fiocchi d‘avena to the known Ingredient: %d\n%s", res.StatusCode, text(res.Body))
	}
}
