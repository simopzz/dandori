package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

var (
	peas    = apptest.Ingredient{Name: "peas", Unit: "g", Usual: "freezer", Kcal: "81", Protein: "5.4", Carbs: "14", Fat: "0.4"}
	lasagne = apptest.Portions{Name: "lasagne", HowMany: "3", Location: "freezer", Kcal: "620", Protein: "32", Carbs: "58", Fat: "28", Fibre: "4", Sugar: "8"}
)

// Portions no Prep Session produced are added from the Kitchen page and show as one group
// under their Storage Location, above its Stock Items.
func TestAddedPortionsShowGroupedAboveTheStockItems(t *testing.T) {
	s := kitchenWith(t, peas)
	s.Counts("peas", "", "400", "")

	s.AddsPortions(lasagne)
	s.IsAt("/kitchen")
	s.IsConfirmed("Added lasagne × 3 to the freezer.")
	s.Sees("Freezer lasagne × 3 per Portion: 620 kcal protein 32 g carbs 58 g fat 28 g fibre 4 g sugar 8 g Count lasagne in the freezer Move lasagne in the freezer peas 400 g")
}

// Calories, protein, carbs and fat are required, each refused on its field with what was
// typed kept. Nothing is added.
func TestAddingPortionsRefusesAMissingValueOnItsField(t *testing.T) {
	s := kitchenWith(t)
	p := lasagne
	p.Kcal, p.Protein, p.Carbs, p.Fat = "", "", "", ""

	s.AddsPortions(p)
	s.IsRefused()
	s.Sees("Calories Required. Protein Required. Carbs Required. Fat Required.")
	if got := apptest.Form(t, s.Page().Body, "portions-form").Get("name"); got != "lasagne" {
		t.Errorf("the refused form holds the name %q", got)
	}
	s.Open("/kitchen")
	s.Sees("Nothing in your kitchen yet.")
}

// A name and a Storage Location are refused on their fields too.
func TestAddingPortionsRefusesNoNameAndNoStorageLocation(t *testing.T) {
	s := kitchenWith(t)
	p := lasagne
	p.Name, p.Location = "  ", ""

	s.AddsPortions(p)
	s.IsRefused()
	s.Sees("Give it a name.")
	s.Sees("Choose where it is kept.")
}

// Fibre and sugar may be unknown, and show as unknown: never 0.
func TestAddedPortionsShowUnknownFibreAndSugarAsUnknown(t *testing.T) {
	s := kitchenWith(t)
	p := lasagne
	p.Fibre, p.Sugar = "", ""

	s.AddsPortions(p)
	s.Sees("fat 28 g fibre – unknown sugar – unknown")
	s.DoesNotSee("fibre 0")
}

// How many is a whole number from 1 to 99.
func TestAddingPortionsTakesAWholeNumberOfAtLeastOne(t *testing.T) {
	for _, c := range []struct{ typed, refused string }{
		{"", "Say how many."},
		{"0", "Must be a whole number of at least 1."},
		{"-1", "Must be a whole number of at least 1."},
		{"2.5", "Must be a whole number of at least 1."},
		{"three", "Must be a whole number of at least 1."},
		{"100", "Must be at most 99."},
	} {
		t.Run(c.typed, func(t *testing.T) {
			s := kitchenWith(t)
			p := lasagne
			p.HowMany = c.typed
			s.AddsPortions(p)
			s.IsRefused()
			s.Sees(c.refused)
		})
	}

	s := kitchenWith(t)
	p := lasagne
	p.HowMany = "99"
	s.AddsPortions(p)
	s.Sees("lasagne × 99")
	p.Name, p.HowMany = "soup", "1"
	s.AddsPortions(p)
	s.Sees("soup × 1")
}

// Alike means the same name, ignoring case and surrounding space, the same values and the
// same Storage Location: anything else is another group.
func TestPortionsGroupOnlyWhenAlike(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	fridge := lasagne
	fridge.Location, fridge.HowMany = "fridge", "1"
	s.AddsPortions(fridge)
	s.Sees("Fridge lasagne × 1")
	s.Sees("Freezer lasagne × 3")

	lighter := lasagne
	lighter.Kcal, lighter.HowMany = "480", "2"
	s.AddsPortions(lighter)
	s.Sees("lasagne × 3 per Portion: 620 kcal")
	s.Sees("lasagne × 2 per Portion: 480 kcal")

	spelled := lasagne
	spelled.Name, spelled.HowMany = "  LASAGNE ", "1"
	s.AddsPortions(spelled)
	s.Sees("lasagne × 4 per Portion: 620 kcal")
}

// An added Portion's name is a Food Term, so it fixes the User's Language.
func TestAnAddedPortionFixesTheLanguage(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	s.Open("/profile")
	s.ChangesLanguage("Italiano")
	s.IsRefused()
	s.Sees("Your kitchen already holds Ingredients, Recipes or Portions in this Language, so it stays.")
}

// Portions are told in the User's Language.
func TestAddingPortionsSpeaksItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")

	s.AddsPortions(lasagne)
	s.IsConfirmed("Aggiunto: lasagne × 3 nel congelatore.")
	s.Sees("Congelatore lasagne × 3 per Porzione: 620 kcal")
}

// Two Users never see each other's Portions.
func TestUsersSeeOnlyTheirOwnPortions(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	s.Invite("bob@example.com")
	s.SwitchesTo("bob@example.com")
	s.ChoosesLanguage("English")
	s.Open("/kitchen")
	s.DoesNotSee("lasagne")
	s.Sees("Nothing in your kitchen yet.")

	s.SwitchesTo("ada@example.com")
	s.Open("/kitchen")
	s.Sees("lasagne × 3")
}

// Posted from the sheet by htmx, an add answers with a fragment: the form as typed when
// refused, and when added a Notice that closes the sheet and lists the kitchen again.
func TestAddingPortionsAnswersHTMXWithAFragment(t *testing.T) {
	s := kitchenWith(t)
	s.Open("/kitchen/portions")
	form := apptest.Form(t, s.Page().Body, "portions-form")
	for field, v := range map[string]string{
		"name": "lasagne", "location": "freezer", "kcal": "620", "protein": "32", "carbs": "58", "fat": "28",
	} {
		form.Values.Set(field, v)
	}

	form.Values.Set("how_many", "100")
	refused := s.HTMXPostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
		t.Fatalf("the refused add answered %d, want 422 and a fragment:\n%s", refused.StatusCode, refused.Body)
	}
	if got := apptest.Form(t, refused.Body, "portions-form").Get("how_many"); got != "100" {
		t.Errorf("the refused add holds %q, want what was typed", got)
	}

	form.Values.Set("how_many", "3")
	added := s.HTMXPostForm(t, form.Action, form.Values)
	if added.StatusCode != http.StatusOK || strings.Contains(added.Body, "<html") {
		t.Fatalf("the add answered %d, want 200 and a fragment:\n%s", added.StatusCode, added.Body)
	}
	if notices := apptest.Notices(t, added.Body); !slices.Equal(notices, []string{"Added lasagne × 3 to the freezer."}) {
		t.Errorf("the add confirms %q", notices)
	}
	if got := added.Header.Get("HX-Trigger"); got != "movement-saved, close-sheet" {
		t.Errorf("the add triggers %q, want the kitchen listed again and the sheet closed", got)
	}
}

// The chevron only chooses what the + adds, and the + opens that sheet. A refused add
// comes back in it with focus on the refused field; one accepted closes it, shows the
// group and leaves the choice, which a new page forgets.
func TestBrowserAddsPortionsInTheSheet(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	expect := playwright.NewPlaywrightAssertions()

	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Choose what to add"}).Click(); err != nil {
		t.Fatalf("open what to add: %v", err)
	}
	choice := b.GetByRole("radio", playwright.PageGetByRoleOptions{Name: "Add Portions"})
	if err := choice.Click(); err != nil {
		t.Fatalf("choose Portions: %v", err)
	}
	if err := expect.Locator(choice).ToBeHidden(); err != nil {
		t.Errorf("choosing doesn't close the menu: %v", err)
	}
	count := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count", Exact: playwright.Bool(true)})
	if err := expect.Locator(count).ToBeHidden(); err != nil {
		t.Errorf("the + still counts: %v", err)
	}
	plus := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Add Portions"})
	if err := expect.Locator(plus).ToBeFocused(); err != nil {
		t.Errorf("choosing doesn't leave focus on the +: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Add Portions"})
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("choosing opens the sheet by itself: %v", err)
	}
	b.Shot(t, "add-choice")
	if err := plus.Click(); err != nil {
		t.Fatalf("open the Add Portions sheet: %v", err)
	}
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Fatalf("the Add Portions sheet doesn't open: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("Name")).ToBeFocused(); err != nil {
		t.Errorf("opening the sheet doesn't focus its first field: %v", err)
	}
	b.Shot(t, "add-portions-sheet")

	fill := func(label, value string) {
		t.Helper()
		if err := sheet.GetByLabel(label).Fill(value); err != nil {
			t.Fatalf("fill %s: %v", label, err)
		}
	}
	fill("Name", "lasagne")
	fill("How many", "100")
	if err := sheet.GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: "freezer"}).Click(); err != nil {
		t.Fatalf("choose the freezer: %v", err)
	}
	fill("Calories", "620")
	fill("Protein", "32")
	fill("Carbs", "58")
	fill("Fat", "28")
	add := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add"})
	if err := add.Click(); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("Must be at most 99."); err != nil {
		t.Fatalf("the sheet doesn't refuse 100: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("How many")).ToBeFocused(); err != nil {
		t.Errorf("the refused field isn't focused: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("Name")).ToHaveValue("lasagne"); err != nil {
		t.Errorf("the refused form loses what was typed: %v", err)
	}

	fill("How many", "3")
	if err := add.Click(); err != nil {
		t.Fatalf("add again: %v", err)
	}
	b.IsConfirmed(t, "Added lasagne × 3 to the freezer.")
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("the sheet doesn't close: %v", err)
	}
	if err := expect.Locator(b.Locator("#stock")).ToContainText("lasagne"); err != nil {
		t.Errorf("the group isn't listed: %v", err)
	}
	if err := expect.Locator(b.Locator("#stock")).ToContainText("× 3"); err != nil {
		t.Errorf("the group isn't counted: %v", err)
	}
	b.IsAt(t, "/kitchen")
	b.Shot(t, "portions-listed")
	if err := expect.Locator(plus).ToBeVisible(); err != nil {
		t.Errorf("adding forgets what the + adds: %v", err)
	}

	b.Open(t, "/kitchen")
	if err := expect.Locator(count).ToBeVisible(); err != nil {
		t.Errorf("a new page doesn't start on Count: %v", err)
	}
	if err := expect.Locator(plus).ToBeHidden(); err != nil {
		t.Errorf("a new page remembers the choice: %v", err)
	}
}

// The links on a group of lasagne in the freezer.
const (
	countLasagne = "Count lasagne in the freezer"
	moveLasagne  = "Move lasagne in the freezer"
)

// A Count up adds Portions alike, which show with the group.
func TestCountingPortionsUpAddsMoreAlike(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	s.CountsPortions(countLasagne, "5", "")
	s.IsAt("/kitchen")
	s.IsConfirmed("Counted lasagne: 5 in the freezer.")
	s.Sees("Freezer lasagne × 5 per Portion: 620 kcal protein 32 g carbs 58 g fat 28 g fibre 4 g sugar 8 g")
	s.DoesNotSee("× 3")
}

// A Count down removes Portions, and a Count to nothing removes the group from the page.
func TestCountingPortionsDownRemovesThem(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	s.CountsPortions(countLasagne, "1", "ate two")
	s.IsConfirmed("Counted lasagne: 1 in the freezer.")
	s.Sees("lasagne × 1")

	s.CountsPortions(countLasagne, "0", "")
	s.DoesNotSee("lasagne")
	s.Sees("Nothing in your kitchen yet.")
}

// A Count below 0, not a whole number, or above what one Count takes is refused on its
// field, with what was typed kept and nothing changed.
func TestCountingPortionsRefusesWhatIsNotACount(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	for typed, why := range map[string]string{
		"-1":  "Can't be below 0.",
		"1.5": "Must be a whole number.",
		"100": "Must be at most 99.",
		"":    "Say how many.",
	} {
		s.CountsPortions(countLasagne, typed, "")
		s.IsRefused()
		s.Sees(why)
		if got := apptest.Form(t, s.Page().Body, "count-form").Get("stated"); got != typed {
			t.Errorf("the refused form holds %q, want %q", got, typed)
		}
	}
	s.Open("/kitchen")
	s.Sees("lasagne × 3")
}

// Moving some leaves two groups, and moving all the rest joins it to them, leaving one.
func TestMovingPortions(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	s.Moves(moveLasagne, "2", "fridge", "to thaw")
	s.IsAt("/kitchen")
	s.IsConfirmed("Moved lasagne × 2 to the fridge.")
	s.Sees("Fridge lasagne × 2")
	s.Sees("Freezer lasagne × 1")

	s.Moves(moveLasagne, "", "fridge", "")
	s.IsConfirmed("Moved lasagne × 1 to the fridge.")
	s.Sees("Fridge lasagne × 3")
	s.DoesNotSee("Freezer")
}

// A move above what the group holds is refused on its field, and so is a move of none,
// part of one, or to nowhere.
func TestMovingPortionsRefusesWhatTheGroupCannotGive(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	for _, c := range []struct{ typed, to, why string }{
		{"4", "fridge", "The group holds 3. If there are more, count them first."},
		{"0", "fridge", "Say how many to move."},
		{"1.5", "fridge", "Must be a whole number."},
		{"1", "", "Choose where it goes."},
	} {
		s.Moves(moveLasagne, c.typed, c.to, "")
		s.IsRefused()
		s.Sees(c.why)
	}
	s.Open("/kitchen")
	s.Sees("lasagne × 3")
}

// A group's Movements show newest first, with cause, before and after, and the note; one
// act is one Movement, and a move tells where it went on the side it left.
func TestAGroupsMovementsTellWhyItHoldsWhatItDoes(t *testing.T) {
	s := kitchenWith(t)
	s.At("Mon 10:00")
	s.AddsPortions(lasagne)
	s.At("Mon 11:00")
	s.CountsPortions(countLasagne, "2", "ate one")
	s.At("Mon 12:00")
	s.Moves(moveLasagne, "1", "fridge", "to thaw")

	s.Open(groupMovements(t, s, countLasagne))
	s.Sees("lasagne · Freezer")
	s.Sees("Moved to the fridge 2 → 1 Mon 2 Nov 12:00 to thaw Counted 3 → 2 Mon 2 Nov 11:00 ate one Added 0 → 3 Mon 2 Nov 10:00")
	if got := strings.Count(apptest.Text(s.Page().Body), "Counted"); got != 1 {
		t.Errorf("the Count shows as %d Movements, want one", got)
	}

	s.Open(groupMovements(t, s, "Move lasagne in the fridge"))
	s.Sees("lasagne · Fridge")
	s.Sees("Moved from the freezer 0 → 1")
	s.DoesNotSee("Added")
}

// kitchenPage is the Kitchen page's markup.
func kitchenPage(s *apptest.Scenario) string {
	s.Open("/kitchen")
	return s.Page().Body
}

// groupMovements is the path of the Movements of the group the link on the Kitchen page
// acts on.
func groupMovements(t *testing.T, s *apptest.Scenario, link string) string {
	t.Helper()
	_, query, ok := strings.Cut(apptest.Link(t, kitchenPage(s), link), "?")
	if !ok {
		t.Fatalf("the link %q asks for no group", link)
	}
	return "/kitchen/portions/movements?" + query
}

// A Count that changes nothing is one Movement like any other, and is the group's.
func TestAPortionCountThatChangesNothingIsAMovement(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)
	s.CountsPortions(countLasagne, "3", "")

	s.Open("/kitchen")
	s.Sees("lasagne × 3")
	s.Follows("lasagne")
	s.Sees("Counted 3 → 3")
}

// A group emptied and filled again starts its Movements over.
func TestAGroupsMovementsBeginWithWhatItHoldsNow(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)
	s.CountsPortions(countLasagne, "0", "")
	s.AddsPortions(lasagne)

	s.Open("/kitchen")
	s.Follows("lasagne")
	s.Sees("Added 0 → 3")
	s.DoesNotSee("Counted")
}

func TestAGroupsMovementsInItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	p := lasagne
	s.At("Mon 14:05")
	s.AddsPortions(p)
	s.CountsPortions("Conta lasagne nel congelatore", "2", "mangiata una")
	s.IsConfirmed("Contato lasagne: 2 nel congelatore.")

	s.Open("/kitchen")
	s.Follows("lasagne")
	s.Sees("Movimenti")
	s.Sees("lasagne · Congelatore")
	s.Sees("Contato 3 → 2 Lun 2 Nov 14:05 mangiata una Aggiunte 0 → 3")
}

// A Portion that isn't the User's, or a place that is no Storage Location, is no group.
func TestNoGroupIsNotFound(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)
	_, query, ok := strings.Cut(apptest.Link(t, kitchenPage(s), countLasagne), "?")
	if !ok {
		t.Fatalf("the link %q asks for no group", countLasagne)
	}

	for _, path := range []string{
		"/kitchen/portions/movements?portion=999&location=freezer",
		"/kitchen/portions/count?portion=999&location=freezer",
		"/kitchen/portions/move?portion=999&location=freezer",
		"/kitchen/portions/count?" + strings.ReplaceAll(query, "freezer", "garage"),
	} {
		if got := s.Get(t, path); got.StatusCode != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", path, got.StatusCode)
		}
	}
}

// A Count and a move of Portions, posted by htmx, answer with no page: refused, the form
// as a fragment with what was typed; saved, a Notice for the sheet to close under.
func TestPortionFormsAnswerHTMXWithFragments(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	for link, form := range map[string]struct {
		id, field, refused, saved, notice string
	}{
		countLasagne: {"count-form", "stated", "-1", "2", "Counted lasagne: 2 in the freezer."},
		moveLasagne:  {"move-form", "quantity", "9", "1", "Moved lasagne × 1 to the fridge."},
	} {
		s.Open("/kitchen")
		s.Follows(link)
		f := apptest.Form(t, s.Page().Body, form.id)
		f.Values.Set("to", "fridge")

		f.Values.Set(form.field, form.refused)
		refused := s.HTMXPostForm(t, f.Action, f.Values)
		if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
			t.Fatalf("%s refused answered %d, want 422 and a fragment:\n%s", link, refused.StatusCode, refused.Body)
		}
		if got := apptest.Form(t, refused.Body, form.id).Get(form.field); got != form.refused {
			t.Errorf("%s: the refused fragment holds %q", link, got)
		}

		f.Values.Set(form.field, form.saved)
		saved := s.HTMXPostForm(t, f.Action, f.Values)
		if saved.StatusCode != http.StatusOK || strings.Contains(saved.Body, "<html") || !strings.Contains(apptest.Text(saved.Body), form.notice) {
			t.Errorf("%s saved answered %d, want 200 and its Notice:\n%s", link, saved.StatusCode, saved.Body)
		}
	}
}

// A group's Movements open in the side sheet, leaving the User on the Kitchen page, which
// a plain URL stands in for without htmx.
func TestBrowserOpensAGroupsMovementsInPlace(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen/portions")
	expect := playwright.NewPlaywrightAssertions()
	for label, value := range map[string]string{"Name": "lasagne", "How many": "3", "Calories": "620", "Protein": "32", "Carbs": "58", "Fat": "28"} {
		if err := b.GetByLabel(label).Fill(value); err != nil {
			t.Fatalf("fill %s: %v", label, err)
		}
	}
	if err := b.GetByRole("radio", playwright.PageGetByRoleOptions{Name: "freezer"}).Click(); err != nil {
		t.Fatalf("choose the freezer: %v", err)
	}
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Add", Exact: playwright.Bool(true)}).Click(); err != nil {
		t.Fatalf("add: %v", err)
	}
	b.IsConfirmed(t, "Added lasagne × 3 to the freezer.")
	b.Open(t, "/kitchen")

	link := b.Locator("#stock").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "lasagne", Exact: playwright.Bool(true)})
	href, err := link.GetAttribute("href")
	if err != nil || !strings.HasPrefix(href, "/kitchen/portions/movements?") {
		t.Fatalf("the group's plain URL is %q: %v", href, err)
	}
	if err := link.Click(); err != nil {
		t.Fatalf("open lasagne: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Movements"})
	if err := expect.Locator(sheet).ToContainText("Added"); err != nil {
		t.Fatalf("the Movements don't open: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("0 → 3"); err != nil {
		t.Errorf("the Movements show the wrong before and after: %v", err)
	}
	b.IsAt(t, "/kitchen")
}
