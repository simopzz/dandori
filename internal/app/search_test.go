package app_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/embedding"
	"github.com/simopzz/dandori/internal/embedding/fakeembed"
)

// startSearch starts the app with a fake LLM to parse Recipes with and the
// fake embedder.
func startSearch(t *testing.T) (*apptest.Harness, *fakellm.LLM, *fakeembed.Embedder) {
	t.Helper()
	llm, embedder := fakellm.New(), fakeembed.New()
	h := apptest.Start(t, apptest.WithPorts(func(p *app.Ports) {
		p.LLM = llm
		p.Embedder = embedder
	}))
	return h, llm, embedder
}

// accept parses a Recipe the model submits as parsed and accepts its
// draft as it stands.
func accept(t *testing.T, h *apptest.Harness, llm *fakellm.LLM, parsed map[string]any) {
	t.Helper()
	llm.Script(fakellm.CallTool("submit_recipe", parsed), fakellm.Reply("Parsed."))
	form := apptest.Form(t, parse(t, h, "a recipe").Body, "recipe")
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("accept %s = %d:\n%s", parsed["name"], res.StatusCode, text(res.Body))
	}
}

func crispyChickpeasParsed() map[string]any {
	return map[string]any{
		"name": "Crispy chickpeas", "portions": 2, "prep_minutes": 20,
		"appliances": []string{"air fryer"}, "steps": []string{"Toss the chickpeas in oil and paprika.", "Air fry until crisp."},
		"ingredients": []map[string]any{
			{"name": "chickpeas", "quantity": 240, "unit": "g",
				"nutrition": map[string]any{"kcal": 120, "protein_g": 7, "carbs_g": 17, "fat_g": 2}},
			{"name": "olive oil", "quantity": 10, "unit": "ml"},
			{"name": "paprika", "quantity": 5, "unit": "g",
				"nutrition": map[string]any{"kcal": 280, "protein_g": 14, "carbs_g": 54, "fat_g": 13}},
		},
	}
}

// library accepts the lemon chicken, the crispy chickpeas and the
// chickpea curry, in that order, so the curry is the newest.
func library(t *testing.T, h *apptest.Harness, llm *fakellm.LLM) {
	t.Helper()
	accept(t, h, llm, parsedRecipe())
	accept(t, h, llm, crispyChickpeasParsed())
	accept(t, h, llm, chickpeaCurryParsed())
}

// search fills the Recipe Library's search form with fields and returns
// the names of the Recipes found, in the order shown.
func search(t *testing.T, h *apptest.Harness, fields map[string]string) (found []string, page string) {
	t.Helper()
	form := apptest.Form(t, h.Get(t, "/recipes").Body, "search")
	for k, v := range fields {
		form.Values.Set(k, v)
	}
	res := h.Get(t, form.Action+"?"+form.Values.Encode())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("search %v = %d:\n%s", fields, res.StatusCode, text(res.Body))
	}
	shown := text(res.Body)
	for _, name := range []string{"Lemon chicken with rice", "Chickpea and chicken curry", "Crispy chickpeas"} {
		if strings.Contains(shown, name) {
			found = append(found, name)
		}
	}
	slices.SortFunc(found, func(a, b string) int { return strings.Index(shown, a) - strings.Index(shown, b) })
	return found, shown
}

func TestRecipeLibraryIsSearchedInNaturalLanguage(t *testing.T) {
	h, llm, _ := startSearch(t)
	library(t, h, llm)

	found, shown := search(t, h, map[string]string{"q": "something warm with chickpeas in the air fryer"})
	if len(found) == 0 || found[0] != "Crispy chickpeas" {
		t.Fatalf("search found %q, want Crispy chickpeas first:\n%s", found, shown)
	}
	if got := apptest.Form(t, h.Get(t, "/recipes?"+url.Values{"q": {"curry"}}.Encode()).Body, "search").Get("q"); got != "curry" {
		t.Errorf("search form shows q = %q, want what was searched", got)
	}
}

func TestSearchIsFilteredByApplianceAndPrepTime(t *testing.T) {
	h, llm, _ := startSearch(t)
	library(t, h, llm)

	tests := []struct {
		name   string
		fields map[string]string
		want   []string
	}{
		{"nothing: every Recipe, newest first", nil,
			[]string{"Chickpea and chicken curry", "Crispy chickpeas", "Lemon chicken with rice"}},
		{"needs the air fryer", map[string]string{"appliance": "air fryer"},
			[]string{"Crispy chickpeas", "Lemon chicken with rice"}},
		{"appliance written another way", map[string]string{"appliance": " Air  Fryer"},
			[]string{"Crispy chickpeas", "Lemon chicken with rice"}},
		{"30 minutes at most", map[string]string{"max_minutes": "30"},
			[]string{"Chickpea and chicken curry", "Crispy chickpeas"}},
		{"both", map[string]string{"appliance": "air fryer", "max_minutes": "30"},
			[]string{"Crispy chickpeas"}},
		{"with what is wanted, closest first", map[string]string{"q": "chicken", "max_minutes": "30"},
			[]string{"Chickpea and chicken curry", "Crispy chickpeas"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if found, shown := search(t, h, tt.fields); !slices.Equal(found, tt.want) {
				t.Errorf("search %v found %q, want %q:\n%s", tt.fields, found, tt.want, shown)
			}
		})
	}
	if _, shown := search(t, h, map[string]string{"max_minutes": "1", "appliance": "oven"}); !strings.Contains(shown, "No Recipes match.") {
		t.Errorf("a search that finds nothing doesn't say so:\n%s", shown)
	}
}

func TestSearchIsFilteredToRecipesStockCovers(t *testing.T) {
	h, llm, _ := startSearch(t)
	library(t, h, llm)
	for _, item := range [][2]string{{"chickpeas", "500"}, {"olive oil", "200"}, {"paprika", "3"}, {"paprika", "3"}, {"chicken breast", "200"}} {
		if res := addStockItem(t, h, item[0], item[1], "pantry", ""); res.StatusCode != http.StatusOK {
			t.Fatalf("add %s = %d:\n%s", item[0], res.StatusCode, text(res.Body))
		}
	}

	found, shown := search(t, h, map[string]string{"stock": "covered"})
	if !slices.Equal(found, []string{"Crispy chickpeas"}) {
		t.Errorf("stock-covered search found %q, want only the crispy chickpeas:\n%s", found, shown)
	}
	if !strings.Contains(shown, "Every Ingredient in stock") {
		t.Errorf("a covered Recipe doesn't say so:\n%s", shown)
	}

	_, shown = search(t, h, map[string]string{"q": "chickpeas"})
	if !strings.Contains(shown, "Not in stock: chicken breast") {
		t.Errorf("the curry doesn't say it needs more chicken breast:\n%s", shown)
	}
	if !strings.Contains(shown, "Not in stock: chicken breast, basmati rice, lemon") {
		t.Errorf("the lemon chicken doesn't say what it lacks, in its order:\n%s", shown)
	}
}

func TestPrepTimeMustBeAWholeNumberOfMinutes(t *testing.T) {
	h, llm, _ := startSearch(t)
	library(t, h, llm)
	res := h.Get(t, "/recipes?"+url.Values{"max_minutes": {"soon"}, "q": {"chickpeas"}}.Encode())
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("search with prep time soon = %d, want 422", res.StatusCode)
	}
	form := apptest.Form(t, res.Body, "search")
	if form.Get("max_minutes") != "soon" || form.Get("q") != "chickpeas" {
		t.Errorf("rejected search isn't shown back as typed: %v", form.Values)
	}
	if !strings.Contains(text(res.Body), "Must be a whole number of minutes") {
		t.Errorf("rejected search doesn't say why:\n%s", text(res.Body))
	}
}

func TestEditedRecipeIsSearchedByWhatItNowSays(t *testing.T) {
	h, llm, embedder := startSearch(t)
	library(t, h, llm)
	lemon := recipeNamed(t, h, "Lemon chicken with rice")
	edit := apptest.Form(t, h.Get(t, lemon+"/edit").Body, "recipe")
	edit.Values["steps"] = []string{"Shape the falafel.", "Fry the falafel."}
	if res := h.PostForm(t, edit.Action, edit.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("edit = %d:\n%s", res.StatusCode, text(res.Body))
	}

	// Accepting and editing embedded every Recipe already: searching
	// embeds nothing more.
	embedder.Fail(embedding.Document, true)
	if found, shown := search(t, h, map[string]string{"q": "falafel"}); len(found) == 0 || found[0] != "Lemon chicken with rice" {
		t.Errorf("search for falafel found %q, want the edited Recipe first:\n%s", found, shown)
	}
	if found, shown := search(t, h, map[string]string{"q": "air fryer chickpeas"}); len(found) == 0 || found[0] != "Crispy chickpeas" {
		t.Errorf("search found %q, want the crispy chickpeas first:\n%s", found, shown)
	}
}

func TestRecipeIsSavedWhileTheEmbeddingProviderIsDownAndEmbeddedLater(t *testing.T) {
	h, llm, embedder := startSearch(t)
	embedder.Fail(embedding.Document, true)
	library(t, h, llm)

	embedder.Fail(embedding.Document, false)
	if found, shown := search(t, h, map[string]string{"q": "something warm with chickpeas in the air fryer"}); len(found) == 0 || found[0] != "Crispy chickpeas" {
		t.Errorf("search found %q, want the crispy chickpeas first:\n%s", found, shown)
	}
}

func TestSearchOnlyFiltersWhenItCantGoByMeaning(t *testing.T) {
	t.Run("the embedding provider is down", func(t *testing.T) {
		h, llm, embedder := startSearch(t)
		library(t, h, llm)
		embedder.Fail(embedding.Query, true)
		found, shown := search(t, h, map[string]string{"q": "chickpeas", "max_minutes": "30"})
		if !slices.Equal(found, []string{"Chickpea and chicken curry", "Crispy chickpeas"}) {
			t.Errorf("search found %q, want the Recipes of 30 minutes at most, newest first:\n%s", found, shown)
		}
		if !strings.Contains(shown, "The embedding provider failed") {
			t.Errorf("search doesn't say it couldn't go by meaning:\n%s", shown)
		}
	})
	t.Run("there is no embedding provider", func(t *testing.T) {
		h, llm := startRecipes(t)
		library(t, h, llm)
		found, shown := search(t, h, map[string]string{"q": "chickpeas", "appliance": "air fryer"})
		if !slices.Equal(found, []string{"Crispy chickpeas", "Lemon chicken with rice"}) {
			t.Errorf("search found %q, want the air fryer Recipes, newest first:\n%s", found, shown)
		}
		if !strings.Contains(shown, "needs an embedding provider") {
			t.Errorf("search doesn't say it couldn't go by meaning:\n%s", shown)
		}
	})
}

// recipeNamed returns the path of the Recipe called name.
func recipeNamed(t *testing.T, h *apptest.Harness, name string) string {
	t.Helper()
	return apptest.Link(t, h.Get(t, "/recipes").Body, name)
}

func TestAgentSearchesTheRecipeLibrary(t *testing.T) {
	llm, embedder := fakellm.New(), fakeembed.New()
	h := apptest.Start(t,
		apptest.WithPorts(func(p *app.Ports) {
			p.LLM = llm
			p.Embedder = embedder
		}),
		apptest.WithAppJob("plan", func(a *app.App) agent.Job {
			return agent.Job{System: "You plan.", Prompt: "Find a quick chickpea Recipe.", Tools: []agent.Tool{a.RecipeSearchTool()}}
		}),
	)
	library(t, h, llm)
	if res := addStockItem(t, h, "chickpeas", "500", "pantry", ""); res.StatusCode != http.StatusOK {
		t.Fatalf("add chickpeas = %d:\n%s", res.StatusCode, text(res.Body))
	}

	llm.Script(
		fakellm.CallTool("search_recipes", map[string]any{"query": "chickpeas in the air fryer", "max_prep_minutes": 30}),
		fakellm.CallTool("search_recipes", map[string]any{"appliance": "oven", "in_stock": true}),
		fakellm.Reply("The crispy chickpeas."),
	)
	run, _ := finish(t, h, "plan")
	trace := text(h.Get(t, run).Body)
	for _, want := range []string{
		"Tool result search_recipes 1. Crispy chickpeas (Recipe 2): 2 Portions, 20 min, needs air fryer; per Portion 192 kcal, 9 g protein, 22 g carbs, 7 g fat. Ingredients: 240 g chickpeas, 10 ml olive oil, 5 g paprika. Not in stock: olive oil, paprika. 2. Chickpea and chicken curry (Recipe 3)",
		"Tool result search_recipes No Recipes match.",
	} {
		if !strings.Contains(trace, want) {
			t.Errorf("trace does not show %q:\n%s", want, trace)
		}
	}
	if strings.Contains(trace, "Lemon chicken with rice (Recipe") {
		t.Errorf("the 40-minute lemon chicken was found:\n%s", trace)
	}
}
