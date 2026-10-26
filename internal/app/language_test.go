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
	for _, want := range []string{"DANDORI_CONTENT_LANGUAGE", `"fr"`, "it or en"} {
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
