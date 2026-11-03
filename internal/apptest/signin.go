package apptest

import (
	"net/http"
	"strings"
	"testing"
)

// Invite invites email to Dandori, as from Supabase's dashboard.
func (h *Harness) Invite(email string) { h.Auth.Invite(email) }

// SignIn signs the invited email in as a User does: asks for a magic link
// on the sign-in page, opens it from their mailbox and confirms. It
// returns the page that lands on: the Language choice at a first sign-in.
func (h *Harness) SignIn(t testing.TB, email string) Response {
	t.Helper()
	if sent := h.AskForLink(t, email); sent.StatusCode != http.StatusOK {
		t.Fatalf("ask for a magic link for %s: status %d:\n%s", email, sent.StatusCode, sent.Body)
	}
	return h.OpenLink(t, h.Auth.LastLink(t, email))
}

// AskForLink asks for a magic link for email on the sign-in page,
// returning the page that answers.
func (h *Harness) AskForLink(t testing.TB, email string) Response {
	t.Helper()
	form := Form(t, h.Get(t, "/sign-in").Body, "sign-in")
	form.Values.Set("email", email)
	return h.PostForm(t, form.Action, form.Values)
}

// OpenLink opens a link from an email and confirms signing in with it,
// returning the page that lands on.
func (h *Harness) OpenLink(t testing.TB, link string) Response {
	t.Helper()
	path, ok := strings.CutPrefix(link, h.URL)
	if !ok {
		t.Fatalf("the emailed link %s does not lead to the app at %s", link, h.URL)
	}
	page := h.Get(t, path)
	if page.StatusCode != http.StatusOK {
		t.Fatalf("open the emailed link %s: status %d:\n%s", path, page.StatusCode, page.Body)
	}
	confirm := Form(t, page.Body, "confirm")
	return h.PostForm(t, confirm.Action, confirm.Values)
}

// ChooseLanguage chooses the Language named name on page, the Language
// choice, returning the page that lands on.
func (h *Harness) ChooseLanguage(t testing.TB, page Response, name string) Response {
	t.Helper()
	form := Choose(t, page.Body, "language", name)
	return h.PostForm(t, form.Action, form.Values)
}

// SignUp signs the invited email in for the first time and chooses
// English, returning Today.
func (h *Harness) SignUp(t testing.TB, email string) Response {
	t.Helper()
	page := h.ChooseLanguage(t, h.SignIn(t, email), "English")
	if page.StatusCode != http.StatusOK || page.Path != "/" {
		t.Fatalf("choosing English as %s landed on %s with status %d, want / and 200:\n%s", email, page.Path, page.StatusCode, page.Body)
	}
	return page
}
