package apptest

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Scenario is a test told as Given/When/Then over the app's clock: it
// moves the clock (At, NextDay), acts as the User would (Open), and
// checks what the User sees (Sees). Each step goes on a timeline, which a
// failed test prints.
type Scenario struct {
	*Harness
	t        testing.TB
	page     Response
	timeline []string
}

// NewScenario starts the app for a scenario. The clock stands at the
// start of Monday 2 November 2026 until a step moves it.
func NewScenario(t testing.TB, opts ...Option) *Scenario {
	t.Helper()
	s := &Scenario{Harness: Start(t, opts...), t: t}
	s.record("starts")
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("timeline:\n%s", strings.Join(s.timeline, "\n"))
		}
	})
	return s
}

// At moves the clock forward to when: a weekday and a time, "Tue 10:00",
// the next one from now (or now itself), or a date and a time,
// "2026-11-03 10:00". The clock never moves back.
func (s *Scenario) At(when string) {
	s.t.Helper()
	now := s.Clock.Now()
	var next time.Time
	if day, timeOfDay, ok := strings.Cut(when, " "); ok && len(day) == 3 {
		wd, ok := weekdays[day]
		hm, err := time.Parse("15:04", timeOfDay)
		if !ok || err != nil {
			s.t.Fatalf("At(%q): want a weekday and a time, such as \"Tue 10:00\"", when)
		}
		next = time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, now.Location())
		next = next.AddDate(0, 0, (int(wd)-int(next.Weekday())+7)%7)
		if next.Before(now) {
			next = next.AddDate(0, 0, 7)
		}
	} else {
		at, err := time.ParseInLocation("2006-01-02 15:04", when, now.Location())
		if err != nil {
			s.t.Fatalf("At(%q): want a weekday and a time, \"Tue 10:00\", or a date and a time, \"2026-11-03 10:00\"", when)
		}
		if at.Before(now) {
			s.t.Fatalf("At(%q) is before now, %s: the clock never moves back", when, now.Format(stamp))
		}
		next = at
	}
	s.Clock.Set(next)
	s.record("at " + when)
}

var weekdays = map[string]time.Weekday{
	"Sun": time.Sunday, "Mon": time.Monday, "Tue": time.Tuesday, "Wed": time.Wednesday,
	"Thu": time.Thursday, "Fri": time.Friday, "Sat": time.Saturday,
}

// NextDay moves the clock to the same time tomorrow.
func (s *Scenario) NextDay() {
	s.Clock.Set(s.Clock.Now().AddDate(0, 0, 1))
	s.record("next day")
}

// Open requests path as the User's browser does, and keeps the page for
// the checks that follow.
func (s *Scenario) Open(path string) {
	s.t.Helper()
	s.page = s.Get(s.t, path)
	s.record(fmt.Sprintf("opens %s: %d", path, s.page.StatusCode))
	if s.page.StatusCode != http.StatusOK {
		s.t.Fatalf("GET %s status = %d, want 200:\n%s", path, s.page.StatusCode, s.page.Body)
	}
}

// IsAt checks the page last opened is at path, where any redirect led.
func (s *Scenario) IsAt(path string) {
	s.t.Helper()
	if s.page.Path != path {
		s.record(fmt.Sprintf("is not at %s but %s", path, s.page.Path))
		s.t.Fatalf("the page is at %s, want %s", s.page.Path, path)
	}
	s.record("is at " + path)
}

// Invite invites email, as from Supabase's dashboard.
func (s *Scenario) Invite(email string) {
	s.Harness.Invite(email)
	s.record("invites " + email)
}

// AsksForALink asks for a magic link for email on the sign-in page.
func (s *Scenario) AsksForALink(email string) {
	s.t.Helper()
	s.page = s.AskForLink(s.t, email)
	s.record(fmt.Sprintf("asks for a link for %s: %d %s", email, s.page.StatusCode, s.page.Path))
}

// Page is the page last opened.
func (s *Scenario) Page() Response { return s.page }

// SignsIn signs the invited email in by magic link, landing on Today.
func (s *Scenario) SignsIn(email string) {
	s.t.Helper()
	s.page = s.SignIn(s.t, email)
	s.record(fmt.Sprintf("signs in as %s: %d %s", email, s.page.StatusCode, s.page.Path))
	if s.page.StatusCode != http.StatusOK || s.page.Path != "/" {
		s.t.Fatalf("signing in as %s landed on %s with status %d, want / and 200:\n%s", email, s.page.Path, s.page.StatusCode, s.page.Body)
	}
}

// SignsOut signs out from the page last opened.
func (s *Scenario) SignsOut() {
	s.t.Helper()
	form := Form(s.t, s.page.Body, "sign-out")
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("signs out: %d %s", s.page.StatusCode, s.page.Path))
}

// Sees checks the page last opened shows want.
func (s *Scenario) Sees(want string) {
	s.t.Helper()
	if shown := Text(s.page.Body); !strings.Contains(shown, want) {
		s.record(fmt.Sprintf("does not see %q", want))
		s.t.Errorf("%s does not show %q: %s", s.page.Path, want, shown)
		return
	}
	s.record(fmt.Sprintf("sees %q", want))
}

// DoesNotSee checks the page last opened doesn't show unwanted.
func (s *Scenario) DoesNotSee(unwanted string) {
	s.t.Helper()
	if shown := Text(s.page.Body); strings.Contains(shown, unwanted) {
		s.record(fmt.Sprintf("sees %q", unwanted))
		s.t.Errorf("%s shows %q: %s", s.page.Path, unwanted, shown)
		return
	}
	s.record(fmt.Sprintf("does not see %q", unwanted))
}

// stamp is how the timeline tells the clock's time.
const stamp = "Mon 2 Jan 15:04"

func (s *Scenario) record(step string) {
	s.timeline = append(s.timeline, s.Clock.Now().Format(stamp)+"  "+step)
}
