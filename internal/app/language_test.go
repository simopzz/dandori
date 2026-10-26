package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/domain"
)

func TestUnsupportedContentLanguageStopsStartup(t *testing.T) {
	cfg := app.Config{DatabaseURL: "postgres://localhost/dandori", ContentLanguage: "fr"}
	a, err := app.New(context.Background(), cfg, app.Ports{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		a.Close()
		t.Fatal("app.New accepted the Content Language fr")
	}
	for _, want := range []string{"DANDORI_CONTENT_LANGUAGE", `"fr"`, "it", "en"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestParseRunWritesFoodTermsInTheContentLanguage(t *testing.T) {
	tests := []struct {
		name     string
		language domain.ContentLanguage
		want     string
	}{
		{name: "unset means English", language: "", want: "English"},
		{name: "Italian", language: domain.Italian, want: "Italian"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := fakellm.New(fakellm.CallTool("submit_recipe", parsedRecipe()), fakellm.Reply("Parsed the Recipe."))
			h := apptest.Start(t,
				apptest.WithPorts(func(p *app.Ports) { p.LLM = llm }),
				apptest.WithConfig(func(c *app.Config) { c.ContentLanguage = tt.language }),
			)
			parse(t, h, pastedRecipe)

			system := llm.Requests()[0].System
			for _, want := range []string{
				"Write the Recipe name, Ingredient names, steps and appliances in " + tt.want,
				"translate it into " + tt.want,
			} {
				if !strings.Contains(system, want) {
					t.Errorf("the parse instructions do not say %q:\n%s", want, system)
				}
			}
		})
	}
}

func TestContentLanguageIsRecordedAndCannotChange(t *testing.T) {
	h := apptest.Start(t, apptest.WithConfig(func(c *app.Config) { c.ContentLanguage = domain.Italian }))
	if res := h.Get(t, "/"); res.StatusCode != http.StatusOK {
		t.Fatalf("GET / on first start = %d, want 200:\n%s", res.StatusCode, res.Body)
	}

	h.Restart(t, apptest.WithConfig(func(c *app.Config) { c.ContentLanguage = domain.English }))
	res := h.Get(t, "/")
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("GET / after switching to English = %d, want 500", res.StatusCode)
	}
	if want := "the database's Content Language is it, but DANDORI_CONTENT_LANGUAGE is en: switching language means a new database"; !strings.Contains(res.Body, want) {
		t.Errorf("error does not say %q:\n%s", want, res.Body)
	}

	h.Restart(t, apptest.WithConfig(func(c *app.Config) { c.ContentLanguage = domain.Italian }))
	if res := h.Get(t, "/"); res.StatusCode != http.StatusOK {
		t.Errorf("GET / back in Italian = %d, want 200:\n%s", res.StatusCode, res.Body)
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
