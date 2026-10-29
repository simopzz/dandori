package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
)

// startGroceries starts the app as seededWithIngredients does, with chicken
// breast mapped to the 320 g pack of sliced chicken breast, 500 ml of olive
// oil in the pantry, and a Profile. It is 2026-11-01 until the test sets
// the clock.
func startGroceries(t *testing.T) (*apptest.Harness, *clock) {
	t.Helper()
	h, _ := seededWithIngredients(t)
	today := &clock{}
	today.set(t, "2026-11-01")
	h.Restart(t, apptest.WithConfig(func(c *app.Config) { c.Now = func() time.Time { return today.now } }))
	if res := h.PostForm(t, "/profile", validProfile()); res.StatusCode != http.StatusOK {
		t.Fatalf("save Profile = %d:\n%s", res.StatusCode, text(res.Body))
	}
	form := mappingForm(t, h, "chicken breast")
	form.Values.Set("catalog_product", productID(t, h, pettoDiPollo.Name))
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("map chicken breast = %d:\n%s", res.StatusCode, text(res.Body))
	}
	addStockItem(t, h, "olive oil", "500", "pantry", "")
	return h, today
}

// groceryList follows the Plan page's first link to a Grocery List.
func groceryList(t *testing.T, h *apptest.Harness) string {
	t.Helper()
	return text(h.Get(t, apptest.Link(t, h.Get(t, "/plan").Body, "Grocery List")).Body)
}

func TestApprovedPlansGroceryListIsGroupedByAisleAndBecomesStockOnArrival(t *testing.T) {
	h, today := startGroceries(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))

	list := groceryList(t, h)
	// Lemon chicken with rice cooked for its three Portions needs 600 g of
	// chicken breast: two 320 g packs. The olive oil in stock covers its
	// 30 ml; rice and lemons have no Product Mapping.
	for _, want := range []string{
		"Carne › Pollo",
		"chicken breast 600 g needed · 640 g bought 2 × " + pettoDiPollo.Name,
		"No Product Mapping",
		"basmati rice 240 g needed",
		"lemon 2 piece needed",
	} {
		if !strings.Contains(list, want) {
			t.Errorf("Grocery List does not show %q:\n%s", want, list)
		}
	}
	if strings.Contains(list, "olive oil") {
		t.Errorf("Grocery List lists olive oil, which stock covers:\n%s", list)
	}
	if strings.Index(list, "Carne › Pollo") > strings.Index(list, "No Product Mapping") {
		t.Errorf("unmapped Ingredients come before the aisles:\n%s", list)
	}

	// Ordered, the Grocery List is fixed: a mapping removed since doesn't
	// change it.
	changeDelivery(t, h, map[string]string{"ordered": "yes"})
	form := mappingForm(t, h, "chicken breast")
	form.Values.Set("catalog_product", "")
	h.PostForm(t, form.Action, form.Values)
	if list := groceryList(t, h); !strings.Contains(list, "640 g bought 2 × "+pettoDiPollo.Name) {
		t.Errorf("the ordered Grocery List changed with the Product Mapping:\n%s", list)
	}

	// The Delivery arrives at the end of its window: everything bought
	// becomes Stock Items, the rounding surplus with it.
	today.setAt(t, "2026-11-02 07:30")
	stock := text(h.Get(t, "/stock").Body)
	for _, want := range []string{
		"chicken breast 640 g fridge",
		"basmati rice 240 g pantry",
		"lemon 2 piece pantry",
		"olive oil 500 ml pantry",
	} {
		if !strings.Contains(stock, want) {
			t.Errorf("after the Delivery arrives, Stock does not show %q:\n%s", want, stock)
		}
	}
	// Arrival records them once, however often it is checked.
	if stock := text(h.Get(t, "/stock").Body); strings.Count(stock, "chicken breast 640 g") != 1 {
		t.Errorf("Stock shows the chicken breast bought %d times, want once:\n%s", strings.Count(stock, "chicken breast 640 g"), stock)
	}
}

func TestTentativeDeliverysGroceryListBecomesStockAsItStandsOnArrival(t *testing.T) {
	h, today := startGroceries(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))

	today.setAt(t, "2026-11-02 08:00")
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "chicken breast 640 g fridge") {
		t.Errorf("after a tentative Delivery arrives, Stock does not show its chicken breast:\n%s", stock)
	}
}

func TestCycleWithoutAPlanHasNothingOnItsGroceryList(t *testing.T) {
	h, _ := startGroceries(t)
	openCycle(t, h, "2026-11-02", "")

	if list := groceryList(t, h); !strings.Contains(list, "Nothing to buy") {
		t.Errorf("Grocery List of a Cycle without a Plan does not say there is nothing to buy:\n%s", list)
	}
}

func TestOrderingAMovedDeliveryFixesTheGroceryListOfTheMovedCycle(t *testing.T) {
	h, _ := startGroceries(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))

	// Moved to the 3rd, the Cycle loses the Lemon chicken cooked on the
	// 2nd; the lemon for the snack on the 3rd is all it still needs.
	changeDelivery(t, h, map[string]string{"delivery_on": "2026-11-03", "ordered": "yes"})
	list := groceryList(t, h)
	if strings.Contains(list, "chicken breast") || !strings.Contains(list, "lemon 1 piece needed") {
		t.Errorf("the Grocery List fixed on ordering is not the moved Cycle's:\n%s", list)
	}
}
