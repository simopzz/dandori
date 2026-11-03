package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// Every page but sign-in needs a signed-in User: the magic link signs them
// in, and signing out sends them back to sign-in.
func TestPagesNeedASignedInUser(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")

	s.Open("/")
	s.IsAt("/sign-in")

	s.SignsIn("ada@example.com")
	s.Open("/")
	s.IsAt("/")
	s.Sees("Signed in as ada@example.com")

	s.SignsOut()
	s.Open("/")
	s.IsAt("/sign-in")
}

// Users share nothing, and row-level security is what enforces it: Today
// reads the signed-in User's email with no WHERE, and still shows only
// theirs.
func TestEachUserSeesOnlyTheirOwnRows(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.Invite("bea@example.com")

	s.SignsIn("ada@example.com")
	s.SignsOut()

	s.SignsIn("bea@example.com")
	s.Sees("Signed in as bea@example.com")
	s.DoesNotSee("ada@example.com")
}

// An email that wasn't invited gets no link, and the page says the same as
// for one that was: it tells no one who has been invited.
func TestAnEmailNotInvitedGetsNoLink(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")

	s.AsksForALink("ada@example.com")
	invited := apptest.Text(s.Page().Body)
	s.AsksForALink("eve@example.com")
	s.Sees(invited)

	if mail := s.Auth.Mail("eve@example.com"); len(mail) > 0 {
		t.Errorf("eve@example.com, not invited, was sent %q", mail)
	}
}

// Signing in lasts until signing out: a day later, past the hour Supabase's
// access token lasts, the User is still signed in.
func TestSigningInLastsUntilSigningOut(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.At("Mon 10:00")
	s.SignsIn("ada@example.com")

	s.NextDay()
	s.Open("/")
	s.IsAt("/")
	s.NextDay()
	s.Open("/")
	s.IsAt("/")
	s.Sees("Signed in as ada@example.com")
}

// A magic link signs in once: opened again, it asks for a new one.
func TestAMagicLinkSignsInOnce(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	h.SignIn(t, "ada@example.com")
	link := h.Auth.LastLink(t, "ada@example.com")

	page := h.OpenLink(t, link)
	if page.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("a used link answered %d, want 422:\n%s", page.StatusCode, page.Body)
	}
	if got := apptest.Text(page.Body); !strings.Contains(got, "expired or was already used") {
		t.Errorf("a used link does not say so: %s", got)
	}
}

// The invitation's own link signs the User in for the first time.
func TestTheInvitationSignsIn(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")

	page := h.OpenLink(t, h.Auth.LastLink(t, "ada@example.com"))
	if page.StatusCode != http.StatusOK || page.Path != "/" {
		t.Fatalf("the invitation landed on %s with %d, want / and 200:\n%s", page.Path, page.StatusCode, page.Body)
	}
	if got := apptest.Text(page.Body); !strings.Contains(got, "Signed in as ada@example.com") {
		t.Errorf("Today after the invitation: %s", got)
	}
}

// A boosted request signed out loads sign-in as a whole page, rather than
// swapping it into the page it came from.
func TestABoostedRequestSignedOutLoadsSignIn(t *testing.T) {
	h := apptest.Start(t)

	res := h.HTMXGet(t, "/")
	if got := res.Header.Get("HX-Redirect"); got != "/sign-in" {
		t.Errorf("HX-Redirect = %q, want /sign-in (status %d)", got, res.StatusCode)
	}
}

// An email that isn't one is rejected with what was typed.
func TestSignInRejectsWhatIsNotAnEmail(t *testing.T) {
	h := apptest.Start(t)

	form := apptest.Form(t, h.Get(t, "/sign-in").Body, "sign-in")
	form.Values.Set("email", "ada")
	page := h.PostForm(t, form.Action, form.Values)
	if page.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", page.StatusCode)
	}
	if got := apptest.Form(t, page.Body, "sign-in").Get("email"); got != "ada" {
		t.Errorf("the form shows %q, want what was typed", got)
	}
}
