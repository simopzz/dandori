package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// The dev seed gives the signed-in User a kitchen ready to plan, through the app's own
// forms: Targets, appliances, Ingredients, stock counted and Recipes. A Cycle opened then
// is planned.
func TestTheDevSeedGivesAKitchenReadyToPlan(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	if got := s.PostForm(t, "/_test/seed", nil); got.StatusCode != http.StatusNoContent {
		t.Fatalf("the seed answered %d, want 204:\n%s", got.StatusCode, got.Body)
	}

	s.Open("/kitchen")
	s.Sees("rice 800 g")
	s.Sees("oats 500 g")
	s.Open("/recipes")
	s.Sees("Chicken rice bowl")
	s.Sees("Overnight oats")

	s.Model.Script(fakellm.Hang())
	s.OpensCycle(monday)
	s.StartsPlanning()
	if path := s.Page().Path; !strings.HasPrefix(path, "/runs/") {
		t.Errorf("planning landed on %s, want the planning Run's page", path)
	}
}

// The dev seed needs a signed-in User who has chosen their Language: before, it seeds
// nothing and says so.
func TestTheDevSeedNeedsAUserWithALanguage(t *testing.T) {
	s := apptest.NewScenario(t)
	got := s.PostForm(t, "/_test/seed", nil)
	if got.StatusCode != http.StatusConflict || !strings.Contains(got.Body, "sign in and choose a Language") {
		t.Errorf("the seed answered %d %q, want 409 asking to sign in", got.StatusCode, got.Body)
	}
}
