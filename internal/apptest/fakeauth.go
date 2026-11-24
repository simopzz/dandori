package apptest

import (
	"testing"

	"github.com/simopzz/dandori/internal/devmode"
)

// FakeAuth is the fake Supabase Auth the app signs Users in through.
type FakeAuth struct {
	*devmode.FakeAuth
}

// LastLink is the link in the last email sent to `email`.
func (f *FakeAuth) LastLink(t testing.TB, email string) string {
	t.Helper()
	link, ok := f.FakeAuth.LastLink(email)
	if !ok {
		t.Fatalf("no email was sent to %s", email)
	}
	return link
}
