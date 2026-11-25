// Package devmode is the dev server's time travel: DANDORI_DEV_NOW runs the
// app on a manual clock, with a fake Supabase Auth and a mailbox under
// /_test/, so that a driver can move the app's time without waiting for it.
package devmode

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/simopzz/dandori/internal/clock"
)

// Dev is a dev server that runs on a manual clock: the app's time stands
// where the driver put it, and what the app assumed is recorded as the
// clock passes it.
type Dev struct {
	// Clock is the app's clock, standing one beat before the dev time.
	Clock *clock.Manual
	// Auth is the fake Supabase Auth the app signs Users in through.
	Auth *FakeAuth

	now   time.Time
	users string
}

// New starts the dev mode: a manual clock standing one beat before `now`,
// and a fake Supabase Auth for the `users` invited once the dev server's
// address is known, each an email or an email and the User's id, as
// "ada@example.com" or "ada@example.com:<uuid>".
func New(now time.Time, beat time.Duration, users string) (*Dev, error) {
	c := clock.NewManual(now.Add(-beat))
	fake, err := NewFakeAuth(c)
	if err != nil {
		return nil, err
	}
	return &Dev{Clock: c, Auth: fake, now: now, users: users}, nil
}

// Listening sets the address the dev server is reached at: the fake Auth's
// URL, which its tokens name as their issuer, and the address its links
// lead to. The invited users are invited then, as an invitation links to
// the app.
func (d *Dev) Listening(base string) {
	d.Auth.URL = base + "/_test/supabase"
	d.Auth.SiteURL = base
	for _, user := range strings.Split(d.users, ",") {
		email, id, _ := strings.Cut(strings.TrimSpace(user), ":")
		switch {
		case email == "":
			// No email, no User to invite.
		case id == "":
			d.Auth.Invite(email)
		default:
			d.Auth.InviteAs(email, id)
		}
	}
}

// Wake wakes the app up at the dev time: the clock moves to it and the
// scheduler's beat runs, so what the app assumed up to then is recorded.
func (d *Dev) Wake() { d.Clock.Set(d.now) }

// Handler serves the dev routes beside the app's: the clock switch, the
// seed, the mailbox and the fake Supabase Auth, all under /_test/.
func (d *Dev) Handler(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /_test/clock", ClockSwitch(d.Clock))
	mux.Handle("POST /_test/seed", Seed(next))
	mux.Handle("GET /_test/mail", Mailbox(d.Auth))
	mux.Handle("/_test/supabase/", http.StripPrefix("/_test/supabase", d.Auth.Handler()))
	mux.Handle("/", next)
	return mux
}

// ClockSwitch sets the clock to the time posted as now, for a driver that
// can't reach the app's clock, such as a browser's. Posted with beat=false,
// it moves the clock without beating, so the next request is the first to
// bring the kitchen up to now.
func ClockSwitch(c *clock.Manual) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now, err := Parse(r.FormValue("now"), c.Now().Location())
		if err != nil {
			http.Error(w, "now: "+err.Error(), http.StatusBadRequest)
			return
		}
		if r.FormValue("beat") == "false" {
			c.SkipTo(now)
		} else {
			c.Set(now)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Parse is the time a driver gives: an RFC 3339 one, as
// 2026-11-25T08:00:00Z, or a local one, as 2026-11-25T08:00, read in `loc`.
func Parse(raw string, loc *time.Location) (time.Time, error) {
	if at, err := time.Parse(time.RFC3339, raw); err == nil {
		return at.In(loc), nil
	}
	if at, err := time.ParseInLocation("2006-01-02T15:04", raw, loc); err == nil {
		return at, nil
	}
	return time.Time{}, fmt.Errorf("%q is not a time: an RFC 3339 one (2026-11-25T08:00:00Z) or a local one (2026-11-25T08:00)", raw)
}

// Mailbox serves the links the fake Auth emailed to `email`, oldest first:
// the driver follows the last one.
func Mailbox(f *FakeAuth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := r.URL.Query().Get("email")
		if email == "" {
			http.Error(w, "email is needed", http.StatusBadRequest)
			return
		}
		links := f.Mail(email)
		if links == nil {
			links = []string{}
		}
		if f.SiteURL != "" {
			// The links lead to the app as this request reached it, so the
			// driver's browser signs in on the same host.
			for i, link := range links {
				links[i] = strings.Replace(link, f.SiteURL, "http://"+r.Host, 1)
			}
		}
		writeJSON(w, links)
	}
}
