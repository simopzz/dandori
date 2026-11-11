package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// asksAgain is the model asked a second time, closing asked, then answered by next: by
// then the Run's Trace holds what submit_recipe answered.
func asksAgain(asked chan struct{}, next fakellm.Step) fakellm.Step {
	return fakellm.Then(func(agent.Request) fakellm.Step {
		close(asked)
		return next
	})
}

// Without JavaScript, a Run's page shows its Trace so far and a link to see it further
// along; once the Run has succeeded, the link to the Recipe Draft it left.
func TestARunsPageWithoutJavaScriptShowsItsTraceSoFarAndALink(t *testing.T) {
	s := kitchenWith(t, rice)
	asked, release := make(chan struct{}), make(chan struct{})
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), asksAgain(asked, fakellm.Reply("Parsed the lasagne.").After(release)))
	s.StartsParse(pastedLasagne)
	run := s.Page().Path

	<-asked
	s.Open(run)
	s.Sees("Reading a Recipe Going")
	s.Sees("Calls submit_recipe")
	s.Follows("See how it's going")
	if s.Page().Path != run {
		t.Errorf("the link to see the Run further along leads to %s, want %s", s.Page().Path, run)
	}

	close(release)
	s.Settle()
	s.Follows("See how it's going")
	s.Sees("Reading a Recipe Succeeded")
	s.Sees("Parsed the lasagne.")
	s.Follows("Recipe Draft")
	draftHolds(t, s, map[string]string{"name": "Lasagne"})
}

// Cancelling a Run going ends it as cancelled, and the Recipe Draft its model had already
// submitted is never shown: a Recipe Draft is only a succeeded Run's. Cancelling it again
// lands on its page as before.
func TestCancellingARunEndsItCancelledWithNoRecipeDraft(t *testing.T) {
	s := kitchenWith(t, rice)
	asked := make(chan struct{})
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), asksAgain(asked, fakellm.Hang()))
	s.StartsParse(pastedLasagne)
	run := s.Page().Path
	<-asked
	s.Open(run)

	cancel := apptest.Form(t, s.Page().Body, "cancel-run")
	for range 2 {
		s.CancelsRun(cancel)
		if res := s.Page(); res.StatusCode != http.StatusOK || res.Path != run {
			t.Fatalf("cancelling the Run = %d at %s, want its page %s", res.StatusCode, res.Path, run)
		}
		// The page cancelling lands on already tells the end: Cancel waits for the Run.
		s.Sees("Reading a Recipe Cancelled")
		s.Sees("Ended: Cancelled")
		if body := s.Page().Body; strings.Contains(body, `href="/recipe-drafts/`) || strings.Contains(body, "Dandori stopped") {
			t.Errorf("a cancelled Run's page links a Recipe Draft or shows a failure:\n%s", apptest.Text(body))
		}
	}
	if strings.Contains(s.Page().Body, `id="cancel-run"`) {
		t.Errorf("a cancelled Run is still offered for cancelling")
	}

	s.Open("/recipes")
	if shown := apptest.Text(s.Page().Body); strings.Contains(shown, "Lasagne") {
		t.Errorf("the Recipe Library shows a cancelled Run's Recipe Draft:\n%s", shown)
	}
	var id int64
	s.Scan(t, &id, `SELECT id FROM recipe_draft`)
	if res := s.Get(t, fmt.Sprintf("/recipe-drafts/%d", id)); res.StatusCode != http.StatusNotFound {
		t.Errorf("a cancelled Run's Recipe Draft opened with %d, want 404", res.StatusCode)
	}
}

// The Runs page, which the navigation reaches, lists the User's Runs newest first, each
// named by the first line of what it was given, with its status, kind and when it
// started, and opening its Trace while it goes and after it ends.
func TestTheRunsPageListsTheUsersRunsNewestFirst(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Model.Script(fakellm.Reply("Not a recipe."), fakellm.Hang())
	s.At("Tue 09:15")
	s.PastesRecipe("Two eggs, boiled.")
	ended := s.Page().Path
	s.At("Tue 10:30")
	s.StartsParse(pastedLasagne)
	going := s.Page().Path

	s.Open("/")
	s.Follows("Runs")
	s.Sees("Runs Lasagne for 4 Going Reading a Recipe Tue 3 Nov 10:30 Two eggs, boiled. Succeeded Reading a Recipe Tue 3 Nov 09:15")
	s.Follows("Lasagne for 4")
	if s.Page().Path != going {
		t.Errorf("the newest Run opens %s, want %s", s.Page().Path, going)
	}
	s.Sees("Started with")

	s.Open("/runs")
	s.Follows("Two eggs, boiled.")
	if s.Page().Path != ended {
		t.Errorf("the ended Run opens %s, want %s", s.Page().Path, ended)
	}
	s.Sees("Succeeded")
	s.Sees("The model says Not a recipe.")
}

// The Runs page names a Run by the first line of what it was given that isn't blank, cut
// with an ellipsis when it is long.
func TestTheRunsPageNamesARunByItsFirstLineCutWhenLong(t *testing.T) {
	s := kitchenWith(t)
	s.Model.Script(fakellm.Reply("Not a recipe."), fakellm.Reply("Not a recipe."))
	s.PastesRecipe(strings.Repeat("Pasta e fagioli ", 6))
	s.PastesRecipe("\n\n  Ragù\nSlow-cooked for hours.")

	s.Open("/runs")
	s.Sees("Ragù Succeeded")
	s.Sees("Pasta e fagioli Pasta e fagioli Pasta e fagioli Pasta e fagi… Succeeded")
}

// A Browser follows a parse Run's Trace live, as it goes, and once the Run succeeds goes
// to the Recipe Draft it left.
func TestBrowserARunsTraceStreamsLiveAndThePageGoesToItsRecipeDraft(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	submit, reply := make(chan struct{}), make(chan struct{})
	r := submittedLasagne()
	r["ingredients"] = r["ingredients"].([]map[string]any)[:4] // Ada has no rice
	h.Model.Script(fakellm.CallTool("submit_recipe", r).After(submit), fakellm.Reply("Parsed the lasagne.").After(reply))

	b.Open(t, "/recipes")
	if err := b.GetByLabel("Paste a recipe").Fill(pastedLasagne); err != nil {
		t.Fatal(err)
	}
	b.Click(t, "Read it")
	if err := b.WaitForURL("**/runs/*"); err != nil {
		t.Fatalf("pasting does not land on the Run's page: %v", err)
	}
	b.Sees(t, "Asking the model (turn 1)")
	if err := playwright.NewPlaywrightAssertions().Locator(b.GetByText("Calls submit_recipe")).ToHaveCount(0); err != nil {
		t.Fatal(err)
	}

	close(submit)
	b.Sees(t, "Calls submit_recipe")
	b.Sees(t, "submit_recipe answers")
	b.Shot(t, "run-live")

	close(reply)
	if err := b.WaitForURL("**/recipe-drafts/*"); err != nil {
		t.Fatalf("the Run's page does not go to its Recipe Draft: %v", err)
	}
	b.Sees(t, "Recipe Draft")
}
