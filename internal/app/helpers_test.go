package app_test

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/simopzz/dandori/internal/apptest"
)

// section is the text of the page's Section titled title, as text gives it,
// so what a check doesn't want to see isn't found in another Section or a
// form's options.
func section(t *testing.T, page, title string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "section" {
			continue
		}
		for c := range n.Descendants() {
			if c.Type == html.ElementNode && c.Data == "h2" && strings.HasSuffix(nodeText(c), title) {
				var b strings.Builder
				if err := html.Render(&b, n); err != nil {
					t.Fatal(err)
				}
				return text(b.String())
			}
		}
	}
	t.Fatalf("page has no Section titled %q:\n%s", title, text(page))
	return ""
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	for c := range n.Descendants() {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// lastCyclePath is the path of the page the Plan page links as page, such
// as "exceptions" or "groceries", for the Cycle it shows last: the upcoming
// one when there is one.
func lastCyclePath(t *testing.T, h *apptest.Harness, page string) string {
	t.Helper()
	plan := h.Get(t, "/plan").Body
	end := `/` + page + `"`
	i := strings.LastIndex(plan, end)
	if i < 0 {
		t.Fatalf("Plan page links no %s page:\n%s", page, text(plan))
	}
	return plan[strings.LastIndex(plan[:i], `"`)+1 : i+len(end)-1]
}

// planOf is a Plan, as the planner submits it, of sessions, each one as
// cooks or setsOut gives it.
func planOf(sessions ...string) string {
	return `{"prep_sessions": [` + strings.Join(sessions, ", ") + `]}`
}

// eatAt is a Portion kept in location and eaten at kind on day, as a Plan
// gives it.
type eatAt struct{ location, day, kind string }

func (e eatAt) json() string {
	return fmt.Sprintf(`{"location": %q, "eat_on": %q, "eat_kind": %q}`, e.location, e.day, e.kind)
}

func portionsJSON(portions []eatAt) string {
	var list []string
	for _, p := range portions {
		list = append(list, p.json())
	}
	return strings.Join(list, ", ")
}

// cooks is a Prep Session of the Recipe with id recipe, at kind on day,
// yielding portions. Each Portion fills its own Slot: two in one Slot, or
// the same Recipe in Slots back to back, break the Plan's rules.
func cooks(recipe, day, kind string, portions ...eatAt) string {
	return fmt.Sprintf(`{"on": %q, "kind": %q, "recipe_id": %s, "portions": [%s]}`, day, kind, recipe, portionsJSON(portions))
}

// setsOut is a Prep Session setting out quantity of the Ingredient named
// ingredient, as it is, at kind on day, eaten then from the fridge.
func setsOut(day, kind, ingredient string, quantity float64) string {
	return fmt.Sprintf(`{"on": %q, "kind": %q, "ingredients": [{"name": %q, "quantity": %v}], "portions": [%s]}`,
		day, kind, ingredient, quantity, eatAt{"fridge", day, kind}.json())
}
