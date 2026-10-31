package app_test

import (
	"encoding/json"
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

// session is a Prep Session as the planner submits it.
type session = map[string]any

// planOf is a Plan of sessions, as the planner submits it.
func planOf(sessions ...session) map[string]any {
	return map[string]any{"prep_sessions": append([]session{}, sessions...)}
}

// planFrom is planOf for a Plan from the Slot kind on the day on.
func planFrom(on, kind string, sessions ...session) map[string]any {
	plan := planOf(sessions...)
	plan["from"] = map[string]any{"on": on, "kind": kind}
	return plan
}

// keeping is plan putting the Portion with id portion, cooked before it,
// in the Slot kind on the day on.
func keeping(plan map[string]any, portion, on, kind string) map[string]any {
	kept, _ := plan["kept"].([]map[string]any)
	plan["kept"] = append(kept, map[string]any{"portion_id": json.Number(portion), "on": on, "kind": kind})
	return plan
}

// eat is a Portion kept in location and eaten at kind on the day on.
func eat(location, on, kind string) map[string]any {
	return map[string]any{"location": location, "eat_on": on, "eat_kind": kind}
}

// cooks is a Prep Session of the Recipe with id recipe at kind on the day
// on, yielding portions. Each Portion fills its own Slot: two in one Slot,
// or the same Recipe in Slots back to back, break the Plan's rules.
func cooks(recipe int64, on, kind string, portions ...map[string]any) session {
	return session{"on": on, "kind": kind, "recipe_id": recipe, "portions": portions}
}

// setsOut is a Prep Session setting out quantity of the Ingredient named
// ingredient, as it is, at kind on the day on, yielding portions: with
// none given, one eaten straight away from the fridge.
func setsOut(on, kind, ingredient string, quantity float64, portions ...map[string]any) session {
	if len(portions) == 0 {
		portions = []map[string]any{eat("fridge", on, kind)}
	}
	return session{"on": on, "kind": kind,
		"ingredients": []map[string]any{{"name": ingredient, "quantity": quantity}},
		"portions":    portions}
}

// lemonAt is a lemon set out at kind on the day on, and eaten straight
// away.
func lemonAt(on, kind string) session { return setsOut(on, kind, "lemon", 1) }
