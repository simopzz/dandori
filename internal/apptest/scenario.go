package apptest

import (
	"fmt"
	"net/http"
	"slices"
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

// SignsIn signs the invited email in by magic link, landing on Today, or at a first
// sign-in on the Language choice.
func (s *Scenario) SignsIn(email string) {
	s.t.Helper()
	s.page = s.SignIn(s.t, email)
	s.record(fmt.Sprintf("signs in as %s: %d %s", email, s.page.StatusCode, s.page.Path))
	if s.page.StatusCode != http.StatusOK {
		s.t.Fatalf("signing in as %s landed on %s with status %d, want 200:\n%s", email, s.page.Path, s.page.StatusCode, s.page.Body)
	}
}

// ChoosesLanguage chooses the Language named name, as named on the Language choice open
// at a first sign-in.
func (s *Scenario) ChoosesLanguage(name string) {
	s.t.Helper()
	s.page = s.ChooseLanguage(s.t, s.page, name)
	s.record(fmt.Sprintf("chooses %s: %d %s", name, s.page.StatusCode, s.page.Path))
}

// ChangesLanguage changes the Language to the one named name on the Profile open.
func (s *Scenario) ChangesLanguage(name string) {
	s.t.Helper()
	form := Choose(s.t, s.page.Body, "profile-language", name)
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("changes the Language to %s: %d %s", name, s.page.StatusCode, s.page.Path))
}

// SwitchesTo signs out from the Profile and signs the invited email in, as another User
// sharing the browser.
func (s *Scenario) SwitchesTo(email string) {
	s.t.Helper()
	s.Open("/profile")
	s.SignsOut()
	s.SignsIn(email)
}

// HasTimesOfDay sets the Profile of the User signed in as email to these times of day of
// breakfast, lunch, snack and dinner, each HH:MM. It stands in for the Profile's form,
// until one changes them.
func (s *Scenario) HasTimesOfDay(email, breakfast, lunch, snack, dinner string) {
	s.t.Helper()
	s.Exec(s.t, `
		UPDATE profile SET breakfast_at = $2, lunch_at = $3, snack_at = $4, dinner_at = $5
		WHERE user_id = (SELECT id FROM "user" WHERE email = $1)`,
		email, breakfast, lunch, snack, dinner)
	s.record(fmt.Sprintf("has times of day %s, %s, %s, %s", breakfast, lunch, snack, dinner))
}

// Follows follows the link on the page open that shows text.
func (s *Scenario) Follows(text string) {
	s.t.Helper()
	s.page = s.Get(s.t, Link(s.t, s.page.Body, text))
	s.record(fmt.Sprintf("follows %q: %d %s", text, s.page.StatusCode, s.page.Path))
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

// IsConfirmed checks the page last opened confirms want in a floating Notice.
func (s *Scenario) IsConfirmed(want string) {
	s.t.Helper()
	notices := Notices(s.t, s.page.Body)
	if !slices.Contains(notices, want) {
		s.record(fmt.Sprintf("is not confirmed %q", want))
		s.t.Errorf("%s does not confirm %q, only %q", s.page.Path, want, notices)
		return
	}
	s.record(fmt.Sprintf("is confirmed %q", want))
}

// IsNotConfirmed checks the page last opened shows no floating Notice.
func (s *Scenario) IsNotConfirmed() {
	s.t.Helper()
	if notices := Notices(s.t, s.page.Body); len(notices) > 0 {
		s.record(fmt.Sprintf("is confirmed %q", notices))
		s.t.Errorf("%s confirms %q, want nothing", s.page.Path, notices)
		return
	}
	s.record("is not confirmed")
}

// stamp is how the timeline tells the clock's time.
const stamp = "Mon 2 Jan 15:04"

func (s *Scenario) record(step string) {
	s.timeline = append(s.timeline, s.Clock.Now().Format(stamp)+"  "+step)
}

// Ingredient is an Ingredient as the add form takes it, each value as typed: one left ""
// is posted blank.
type Ingredient struct {
	Name, Unit, Usual                       string
	Kcal, Protein, Carbs, Fat, Fibre, Sugar string
	Staple                                  bool
}

// Fill sets f, the add form, to ing.
func (ing Ingredient) Fill(f FormData) {
	for field, v := range map[string]string{
		"name": ing.Name, "unit": ing.Unit, "usual": ing.Usual,
		"kcal": ing.Kcal, "protein": ing.Protein, "carbs": ing.Carbs, "fat": ing.Fat,
		"fibre": ing.Fibre, "sugar": ing.Sugar,
	} {
		f.Values.Set(field, v)
	}
	f.Values.Del("staple")
	if ing.Staple {
		f.Values.Set("staple", "on")
	}
}

// AddsIngredient adds ing with the add form on the page open, posted as a browser
// without htmx.
func (s *Scenario) AddsIngredient(ing Ingredient) {
	s.t.Helper()
	form := Form(s.t, s.page.Body, "ingredient-add")
	ing.Fill(form)
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("adds the Ingredient %q: %d %s", ing.Name, s.page.StatusCode, s.page.Path))
}

// Portions are what the Add Portions form is filled with: a blank field stays blank.
type Portions struct {
	Name, HowMany, Location, Kcal, Protein, Carbs, Fat, Fibre, Sugar, Note string
}

// AddsPortions adds p with the Add Portions page, posted as a browser without htmx.
func (s *Scenario) AddsPortions(p Portions) {
	s.t.Helper()
	s.Open("/kitchen/portions")
	form := Form(s.t, s.page.Body, "portions-form")
	for field, v := range map[string]string{
		"name": p.Name, "how_many": p.HowMany, "location": p.Location, "kcal": p.Kcal, "protein": p.Protein,
		"carbs": p.Carbs, "fat": p.Fat, "fibre": p.Fibre, "sugar": p.Sugar, "note": p.Note,
	} {
		form.Values.Set(field, v)
	}
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("adds %s of the Portions %q: %d %s", p.HowMany, p.Name, s.page.StatusCode, s.page.Path))
}

// EditsIngredient edits the Ingredient named name with its edit form on the page open,
// setting every field to ing's, posted as a browser without htmx.
func (s *Scenario) EditsIngredient(name string, ing Ingredient) {
	s.t.Helper()
	form := EditForm(s.t, s.page.Body, name)
	ing.Fill(form)
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("edits the Ingredient %q to %q: %d %s", name, ing.Name, s.page.StatusCode, s.page.Path))
}

// DeletesIngredient deletes the Ingredient named name with its delete form on the page
// open, posted as a browser without htmx.
func (s *Scenario) DeletesIngredient(name string) {
	s.t.Helper()
	form := DeleteForm(s.t, s.page.Body, name)
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("deletes the Ingredient %q: %d %s", name, s.page.StatusCode, s.page.Path))
}

// Submits submits the form with id on the page open as it is shown, as a browser
// without htmx.
func (s *Scenario) Submits(id string) {
	s.t.Helper()
	form := Form(s.t, s.page.Body, id)
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("submits %s: %d %s", id, s.page.StatusCode, s.page.Path))
}

// SearchesIngredients searches the Ingredients for query with the page's search form,
// as a browser without htmx.
func (s *Scenario) SearchesIngredients(query string) {
	s.t.Helper()
	form := Form(s.t, s.page.Body, "ingredient-search")
	form.Values.Set("q", query)
	s.page = s.Get(s.t, form.Action+"?"+form.Values.Encode())
	s.record(fmt.Sprintf("searches the Ingredients for %q: %d %s", query, s.page.StatusCode, s.page.Path))
}

// IsRefused checks the last form posted was refused, answered with 422.
func (s *Scenario) IsRefused() {
	s.t.Helper()
	if s.page.StatusCode != http.StatusUnprocessableEntity {
		s.record(fmt.Sprintf("is not refused: %d", s.page.StatusCode))
		s.t.Fatalf("the post answered %d, want 422:\n%s", s.page.StatusCode, s.page.Body)
	}
	s.record("is refused")
}

// Moves opens the Kitchen page, follows the row's link (such as "Move rice in the
// pantry") and posts its form: quantity, unless blank leaves its default, to the Storage
// Location to, with note.
func (s *Scenario) Moves(link, quantity, to, note string) {
	s.t.Helper()
	s.Open("/kitchen")
	s.Follows(link)
	form := Form(s.t, s.page.Body, "move-form")
	if quantity != "" {
		form.Values.Set("quantity", quantity)
	}
	form.Values.Set("to", to)
	form.Values.Set("note", note)
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("%s: %s to %q: %d %s", link, quantity, to, s.page.StatusCode, s.page.Path))
}

// CountsAgainASecondAhead stands in for a Count that took hold of the kitchen first with
// a clock a second ahead of the app's: it repeats the ledger's one Count a second after
// it, changing nothing.
func (s *Scenario) CountsAgainASecondAhead() {
	s.t.Helper()
	s.Exec(s.t, `
		INSERT INTO movement (user_id, happened_at, ingredient_id, storage_location, change,
			cause_kind, believed, stated)
		SELECT user_id, happened_at + interval '1 second', ingredient_id, storage_location, 0,
			'counted', stated, stated
		FROM movement`)
	s.record("counts again, a second ahead")
}

// Counts counts name with the Count form, as a browser without htmx: it finds the
// Ingredient, keeps its usual Storage Location unless location is given, and states
// stated, with note.
func (s *Scenario) Counts(name, location, stated, note string) {
	s.t.Helper()
	s.Open("/kitchen/count")
	s.Follows(name)
	form := Form(s.t, s.page.Body, "count-form")
	if location != "" {
		form.Values.Set("location", location)
	}
	form.Values.Set("stated", stated)
	form.Values.Set("note", note)
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("counts %s of %s in %q: %d %s", stated, name, location, s.page.StatusCode, s.page.Path))
}

// CountsPortions opens the Kitchen page, follows the group's link (such as "Count lasagne
// in the freezer") and posts its form as a browser without htmx: stated, with note.
func (s *Scenario) CountsPortions(link, stated, note string) {
	s.t.Helper()
	s.Open("/kitchen")
	s.Follows(link)
	form := Form(s.t, s.page.Body, "count-form")
	form.Values.Set("stated", stated)
	form.Values.Set("note", note)
	s.page = s.PostForm(s.t, form.Action, form.Values)
	s.record(fmt.Sprintf("%s: %s: %d %s", link, stated, s.page.StatusCode, s.page.Path))
}
