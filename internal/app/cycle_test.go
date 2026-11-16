package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// thursday is a Cycle whose Delivery comes on Thursday 5 November between 6 and 8 pm,
// ending when left to.
var thursday = apptest.Cycle{Delivery: "11/05/2026", From: "6pm", To: "8pm"}

// plannerOf signs up email, in English, with their Targets set.
func plannerOf(t *testing.T, email string) *apptest.Scenario {
	s := profileOf(t, email)
	s.SavesTargets(targets)
	return s
}

// Opening a Cycle lands on the Cycle page, which shows it upcoming with its Delivery
// tentative, ending ten days after the Delivery's date when no end date is typed.
func TestOpeningACycleShowsItUpcoming(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.Open("/plan")
	s.Sees("No Cycle is upcoming.")
	s.OpensCycle(thursday)
	s.IsAt("/plan")
	s.IsConfirmed("Cycle opened.")
	s.Sees("Upcoming Cycle")
	s.Sees("Delivery Thursday, November 5, 6:00 pm to 8:00 pm Tentative")
	s.Sees("End date Sunday, November 15")
	s.DoesNotSee("No Cycle is upcoming.")

	s.Open("/plan")
	s.Sees("Upcoming Cycle")
}

// An end date typed is the Cycle's.
func TestACycleEndsOnTheDateTyped(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	c := thursday
	c.Ends = "11/12/2026"
	s.OpensCycle(c)
	s.Sees("End date Thursday, November 12")
}

// Each refusal is on its own field, opening nothing, with what was typed kept.
func TestOpeningACycleRefusesEachFieldOnItsOwn(t *testing.T) {
	cases := []struct {
		name   string
		change func(*apptest.Cycle)
		shown  string
	}{
		{"a window ending before it starts", func(c *apptest.Cycle) { c.To = "5pm" }, "Window to Must be after the window's start."},
		{"a window already ended", func(c *apptest.Cycle) { c.Delivery, c.From, c.To = "11/02/2026", "6am", "8am" }, "Window to This window has already ended."},
		{"a Delivery already past", func(c *apptest.Cycle) { c.Delivery = "11/01/2026" }, "Delivery date This window has already ended."},
		{"an end date on the Delivery's", func(c *apptest.Cycle) { c.Ends = "11/05/2026" }, "End date (optional) Must be after the Delivery's date."},
		{"an end date more than 14 days after", func(c *apptest.Cycle) { c.Ends = "11/20/2026" }, "End date (optional) At most 14 days after the Delivery's date."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := plannerOf(t, "ada@example.com")
			s.At("Mon 10:00")
			typed := thursday
			c.change(&typed)
			s.OpensCycle(typed)
			s.IsRefused()
			s.Sees(c.shown)
			if got := apptest.Form(t, s.Page().Body, "open-cycle").Get("from"); got != typed.From {
				t.Errorf("the refused form keeps %q as the window's start, want %q", got, typed.From)
			}
			s.Open("/plan")
			s.Sees("No Cycle is upcoming.")
		})
	}
}

// Another Cycle can't be opened while one is upcoming: the form says so, and links to it.
func TestOpeningACycleIsRefusedWhileOneIsUpcoming(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.OpensCycle(thursday)
	s.OpensCycle(apptest.Cycle{Delivery: "11/07/2026", From: "10am", To: "12pm"})
	s.IsRefused()
	s.Sees("A Cycle is already upcoming. Drop it to open another.")
	s.Follows("See the upcoming Cycle")
	s.Sees("Thursday, November 5")
	s.DoesNotSee("Saturday, November 7")
}

// Opening a Cycle needs the Targets, and the form says so with a link to set them.
func TestOpeningACycleIsRefusedWithoutTargets(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.Open("/plan/open")
	s.Sees("Planning needs your Targets.")
	s.OpensCycle(thursday)
	s.IsRefused()
	s.Sees("Planning needs your Targets.")
	if got := apptest.Link(t, s.Page().Body, "Set your Targets"); got != "/profile#targets" {
		t.Errorf("the refusal links to %q, want the Profile's Targets", got)
	}
	s.Open("/plan")
	s.Sees("No Cycle is upcoming.")
}

// Once a Cycle's Delivery has arrived, the next one's can come before its end date or
// after it.
func TestADeliveryBeforeOrAfterAnotherCyclesEndIsAccepted(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.OpensCycle(thursday)
	s.At("Thu 20:00")
	s.Open("/plan")
	s.Sees("No Cycle is upcoming.")

	s.OpensCycle(apptest.Cycle{Delivery: "11/10/2026", From: "6pm", To: "8pm"})
	s.IsAt("/plan")
	s.Sees("Delivery Tuesday, November 10")
	s.DropsCycle()
	s.OpensCycle(apptest.Cycle{Delivery: "11/20/2026", From: "6pm", To: "8pm"})
	s.IsAt("/plan")
	s.Sees("Delivery Friday, November 20")
}

// Dropping the upcoming Cycle asks first, naming it, then the Cycle page no longer shows
// it, and another can be opened.
func TestDroppingTheUpcomingCycle(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.OpensCycle(thursday)
	s.Follows("Drop the Cycle")
	s.Sees("Drop the Cycle whose Delivery comes on Thursday, November 5?")
	s.Submits("remove-step")
	s.IsAt("/plan")
	s.IsConfirmed("Cycle dropped.")
	s.Sees("No Cycle is upcoming.")
	s.OpensCycle(thursday)
	s.IsAt("/plan")
}

// A Cycle whose Delivery has arrived can't be dropped.
func TestACycleArrivedCantBeDropped(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.OpensCycle(thursday)
	form := apptest.Form(t, s.Get(t, apptest.Link(t, s.Page().Body, "Drop the Cycle")).Body, "remove-step")
	s.At("Thu 20:00")
	if got := s.PostForm(t, form.Action, form.Values); got.StatusCode != http.StatusNotFound {
		t.Errorf("dropping an arrived Cycle answers %d, want 404", got.StatusCode)
	}
}

// Each form answers htmx as it does a plain post: opening lands on the Cycle page, a
// refusal is the form again with 422, and dropping, posted from its sheet, lands on the
// Cycle page.
func TestCycleFormsPostedAsHTMX(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	open := apptest.Form(t, s.Get(t, "/plan/open").Body, "open-cycle")
	refused := thursday
	refused.To = "5pm"
	refused.Fill(open)
	if got := s.HTMXPostForm(t, open.Action, open.Values); got.StatusCode != http.StatusUnprocessableEntity || apptest.Form(t, got.Body, "open-cycle").Get("to") != "5pm" {
		t.Errorf("a refused opening as htmx answers %d, want 422 with the form as typed", got.StatusCode)
	}
	thursday.Fill(open)
	if got := s.HTMXPostForm(t, open.Action, open.Values); got.Path != "/plan" || !strings.Contains(apptest.Text(got.Body), "Upcoming Cycle") {
		t.Errorf("opening as htmx lands on %s, want the Cycle page showing it", got.Path)
	}
	src := apptest.SheetSource(t, s.Get(t, "/plan").Body, "Drop the Cycle")
	drop := apptest.Form(t, s.HTMXGet(t, src).Body, "remove-step")
	got := s.HTMXPostForm(t, drop.Action, drop.Values)
	if got.Path != "/plan" || !strings.Contains(apptest.Text(got.Body), "No Cycle is upcoming.") {
		t.Errorf("dropping as htmx lands on %s, want the Cycle page without it", got.Path)
	}
}

// Today offers to open a Cycle while there's none, current or upcoming.
func TestTodayOffersToOpenACycle(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.Open("/")
	if got := apptest.Link(t, s.Page().Body, "Open a Cycle"); got != "/plan/open" {
		t.Errorf("Today's offer leads to %q, want /plan/open", got)
	}
	s.OpensCycle(thursday)
	s.Open("/")
	s.DoesNotSee("Open a Cycle")
	s.At("Thu 20:00")
	s.Open("/")
	s.DoesNotSee("Open a Cycle")
}

// Two Users never see each other's Cycles, and one can't drop the other's.
func TestTwoUsersNeverSeeEachOthersCycles(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.OpensCycle(thursday)
	drop := apptest.Link(t, s.Page().Body, "Drop the Cycle")
	s.Open("/profile")
	s.SignsOut()
	s.Invite("bob@example.com")
	s.SignsIn("bob@example.com")
	s.ChoosesLanguage("English")
	s.Open("/plan")
	s.Sees("No Cycle is upcoming.")
	if got := s.Get(t, drop); got.StatusCode != http.StatusNotFound {
		t.Errorf("another User's drop step answers %d, want 404", got.StatusCode)
	}
}

// A Cycle across a change of the clocks, to summer time in March and back to standard
// time in October, ends ten days after its Delivery's date, and arrives at the end of
// its window on the wall clock, not an hour early or late.
func TestACycleAcrossAChangeOfTheClocks(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	for _, c := range []struct {
		before, delivery, shown, ends, lastMinute, arrives string
	}{
		{"2027-03-26 10:00", "03/27/2027", "Delivery Saturday, March 27, 6:00 pm to 8:00 pm", "End date Tuesday, April 6", "2027-03-27 19:59", "2027-03-27 20:00"},
		{"2027-10-29 10:00", "10/30/2027", "Delivery Saturday, October 30, 6:00 pm to 8:00 pm", "End date Tuesday, November 9", "2027-10-30 19:59", "2027-10-30 20:00"},
	} {
		s.At(c.before)
		s.OpensCycle(apptest.Cycle{Delivery: c.delivery, From: "6pm", To: "8pm"})
		s.Sees(c.shown)
		s.Sees(c.ends)
		s.At(c.lastMinute)
		s.Open("/plan")
		s.Sees("Upcoming Cycle")
		s.At(c.arrives)
		s.Open("/plan")
		s.Sees("No Cycle is upcoming.")
	}
}

// Through a browser: the Cycle page opens a Cycle on a page of its own, refusing a field
// in place, back to the Cycle page showing it, and Drop asks in its sheet, over the Cycle
// page, before dropping it.
func TestBrowserOpensAndDropsACycle(t *testing.T) {
	b := signedUpBrowser(t)
	expect := playwright.NewPlaywrightAssertions()
	fill := func(selector, value string) {
		t.Helper()
		if err := b.Locator(selector).Fill(value); err != nil {
			t.Fatalf("fill %s: %v", selector, err)
		}
	}
	follow := func(name string) {
		t.Helper()
		if err := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: name}).Click(); err != nil {
			t.Fatalf("follow %s: %v", name, err)
		}
	}
	b.Open(t, "/profile")
	for field, v := range map[string]string{"kcal": "2200", "protein": "130", "fibre": "30", "sugar": "50"} {
		fill("#profile-targets [name="+field+"]", v)
	}
	if err := b.Locator("#profile-targets button[type=submit]").Click(); err != nil {
		t.Fatalf("save the Targets: %v", err)
	}
	b.IsConfirmed(t, "Profile saved.")

	b.Open(t, "/plan")
	follow("Open a Cycle")
	b.IsAt(t, "/plan/open")
	fill("[name=delivery]", "11/05/2026")
	fill("[name=from]", "6pm")
	fill("[name=to]", "5pm")
	b.Click(t, "Open the Cycle")
	b.Sees(t, "Must be after the window's start.")
	b.Shot(t, "cycle-open-refused")
	fill("[name=to]", "8pm")
	b.Click(t, "Open the Cycle")
	b.IsAt(t, "/plan")
	b.IsConfirmed(t, "Cycle opened.")
	b.Sees(t, "Thursday, November 5, 6:00 pm to 8:00 pm")
	b.Sees(t, "Sunday, November 15")
	b.Shot(t, "cycle-upcoming")
	if err := b.Notices().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close notice"}).Click(); err != nil {
		t.Fatalf("close the Notice: %v", err)
	}

	follow("Drop the Cycle")
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Drop the Cycle"})
	if err := expect.Locator(sheet).ToContainText("Drop the Cycle whose Delivery comes on Thursday, November 5?"); err != nil {
		t.Fatalf("Drop doesn't ask in its sheet: %v", err)
	}
	b.Shot(t, "cycle-drop")
	b.IsAt(t, "/plan")
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Drop", Exact: playwright.Bool(true)}).Click(); err != nil {
		t.Fatalf("drop: %v", err)
	}
	b.IsConfirmed(t, "Cycle dropped.")
	b.Sees(t, "No Cycle is upcoming.")
}
