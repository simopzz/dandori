package app_test

import (
	"fmt"
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
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("remove chicken breast's mapping = %d:\n%s", res.StatusCode, text(res.Body))
	}
	if list := groceryList(t, h); !strings.Contains(list, "640 g bought 2 × "+pettoDiPollo.Name) {
		t.Errorf("the ordered Grocery List changed with the Product Mapping:\n%s", list)
	}

	// The Delivery arrives at the end of its window: everything bought
	// becomes Stock Items, the rounding surplus with it. The Lemon chicken
	// cooked that day uses up its share: 600 g of the 640 g of chicken
	// breast, all the rice, 30 ml of olive oil and a lemon.
	today.setAt(t, "2026-11-02 07:30")
	stock := text(h.Get(t, "/stock").Body)
	for _, want := range []string{
		"chicken breast 40 g fridge",
		"lemon 1 piece pantry",
		"olive oil 470 ml pantry",
	} {
		if !strings.Contains(stock, want) {
			t.Errorf("after the Delivery arrives, Stock does not show %q:\n%s", want, stock)
		}
	}
	// Arrival records them once, however often it is checked.
	if stock := text(h.Get(t, "/stock").Body); strings.Count(stock, "chicken breast 40 g") != 1 {
		t.Errorf("Stock shows the chicken breast bought %d times, want once:\n%s", strings.Count(stock, "chicken breast 40 g"), stock)
	}
}

func TestTentativeDeliverysGroceryListBecomesStockAsItStandsOnArrival(t *testing.T) {
	h, today := startGroceries(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))

	// What the Lemon chicken cooked that day leaves of it.
	today.setAt(t, "2026-11-02 08:00")
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "chicken breast 40 g fridge") {
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

func TestDeliveryIsNotMarkedOrderedBeforeItsCycleHasAPlan(t *testing.T) {
	h, _ := startGroceries(t)
	openCycle(t, h, "2026-11-02", "")

	res := changeDelivery(t, h, map[string]string{"ordered": "yes"})
	if want := "Approve a Plan first: ordering fixes its Grocery List."; res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), want) {
		t.Fatalf("mark ordered without a Plan = %d, want 422 saying %q:\n%s", res.StatusCode, want, text(res.Body))
	}
	if page := text(h.Get(t, "/plan").Body); !strings.Contains(page, "tentative") {
		t.Errorf("the Delivery was marked ordered without a Plan:\n%s", page)
	}

	approvePlan(t, h, lemonPlan(t, h))
	if res := changeDelivery(t, h, map[string]string{"ordered": "yes"}); res.StatusCode != http.StatusOK {
		t.Errorf("mark ordered with a Plan = %d:\n%s", res.StatusCode, text(res.Body))
	}
}

func TestCycleArrivingWithoutAPlanSaysSoOnItsGroceryList(t *testing.T) {
	h, today := startGroceries(t)
	openCycle(t, h, "2026-11-02", "")

	today.setAt(t, "2026-11-02 08:00")
	if list := groceryList(t, h); !strings.Contains(list, "Its Delivery arrived before a Plan was approved.") {
		t.Errorf("Grocery List of a Cycle that arrived without a Plan does not say so:\n%s", list)
	}
}

func TestPrepSessionUsesUpItsIngredientsStockOnceItsDayComes(t *testing.T) {
	h, today := startGroceries(t)
	openCycle(t, h, "2026-11-02", "")
	approvePlan(t, h, lemonPlan(t, h))

	// The Delivery arrives the day the Lemon chicken with rice is cooked
	// for its three Portions: 600 g of the 640 g of chicken breast, all the
	// rice, 30 ml of the olive oil and one lemon; the other lemon is for
	// the snack on the 3rd.
	today.setAt(t, "2026-11-02 07:30")
	for range 2 {
		stock := text(h.Get(t, "/stock").Body)
		for _, want := range []string{"chicken breast 40 g fridge", "lemon 1 piece pantry", "olive oil 470 ml pantry"} {
			if !strings.Contains(stock, want) {
				t.Errorf("once the Lemon chicken's day comes, Stock does not show %q:\n%s", want, stock)
			}
		}
		if strings.Contains(stock, "basmati rice 240 g pantry") {
			t.Errorf("Stock still lists the basmati rice the Lemon chicken used up:\n%s", stock)
		}
	}

	// The user finds more chicken breast left than that; the correction
	// stands as later Prep Sessions use up theirs.
	form := apptest.Form(t, h.Get(t, "/stock").Body, "stock-item-"+stockItemID(t, h))
	form.Values.Set("quantity", "300")
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("correct chicken breast = %d:\n%s", res.StatusCode, text(res.Body))
	}
	today.set(t, "2026-11-03")
	stock := text(h.Get(t, "/stock").Body)
	if !strings.Contains(stock, "chicken breast 300 g fridge") {
		t.Errorf("Stock does not keep the corrected chicken breast:\n%s", stock)
	}
	if strings.Contains(stock, "lemon 1 piece pantry") {
		t.Errorf("Stock still lists the lemon set out for the snack on the 3rd:\n%s", stock)
	}
}

func TestUpcomingGroceryListBuysWhatTheCurrentCyclesPrepSessionsWillUseUp(t *testing.T) {
	h, today := startGroceries(t)
	recipe := strings.TrimPrefix(libraryRecipe(t, h), "/recipes/")
	// lemonChicken cooks Lemon chicken with rice for dinner on day, eaten
	// then, at lunch on lunch and at dinner on dinner.
	lemonChicken := func(day, lunch, dinner string) string {
		return fmt.Sprintf(`{"prep_sessions": [
			{"on": %[1]q, "kind": "dinner", "recipe_id": %[2]s, "portions": [
				{"location": "fridge", "eat_on": %[1]q, "eat_kind": "dinner"},
				{"location": "fridge", "eat_on": %[3]q, "eat_kind": "lunch"},
				{"location": "freezer", "eat_on": %[4]q, "eat_kind": "dinner"}]}
		]}`, day, recipe, lunch, dinner)
	}
	openCycle(t, h, "2026-11-02", "2026-11-10")
	approvePlan(t, h, lemonChicken("2026-11-05", "2026-11-07", "2026-11-09"))
	// 640 g of chicken breast arrives, and the Lemon chicken on the 5th
	// will use 600 g of it: the next Cycle's needs 560 g more.
	today.setAt(t, "2026-11-02 08:00")
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "chicken breast 640 g fridge") {
		t.Fatalf("Stock does not show the chicken breast delivered:\n%s", stock)
	}

	today.set(t, "2026-11-03")
	openCycleIn(t, h, "2026-11-10", "18:00", "20:00", "2026-11-20")
	approvePlan(t, h, lemonChicken("2026-11-11", "2026-11-13", "2026-11-16"))
	// The upcoming Cycle's links come after the current one's.
	page := h.Get(t, "/plan").Body
	page = page[strings.LastIndex(page, "Exceptions</a>"):]
	if list := text(h.Get(t, apptest.Link(t, page, "Grocery List")).Body); !strings.Contains(list, "chicken breast 560 g needed · 640 g bought 2 × "+pettoDiPollo.Name) {
		t.Errorf("the upcoming Grocery List does not buy the chicken breast the current Cycle will use up:\n%s", list)
	}
}

func TestPrepSessionTheNextDeliveryDropsUsesUpNothing(t *testing.T) {
	h, today := startGroceries(t)
	lemonFor := func(day, kind string) string {
		return fmt.Sprintf(`{"prep_sessions": [
			{"on": %[1]q, "kind": %[2]q, "ingredients": [{"name": "lemon", "quantity": 1}], "portions": [
				{"location": "fridge", "eat_on": %[1]q, "eat_kind": %[2]q}]}
		]}`, day, kind)
	}
	openCycle(t, h, "2026-11-02", "2026-11-10")
	approvePlan(t, h, lemonFor("2026-11-05", "dinner"))
	today.setAt(t, "2026-11-02 08:00")
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "lemon 1 piece pantry") {
		t.Fatalf("Stock does not show the lemon delivered:\n%s", stock)
	}

	// The next Delivery, in the evening of the 5th, closes the Cycle early:
	// the next Cycle starts at dinner that day, and the lemon set out for
	// it is dropped. The next Cycle's Grocery List counts on that lemon.
	today.set(t, "2026-11-03")
	openCycleIn(t, h, "2026-11-05", "18:00", "20:00", "2026-11-15")
	approvePlan(t, h, lemonFor("2026-11-06", "snack"))
	page := h.Get(t, "/plan").Body
	page = page[strings.LastIndex(page, "Exceptions</a>"):]
	if list := text(h.Get(t, apptest.Link(t, page, "Grocery List")).Body); strings.Contains(list, "lemon") {
		t.Errorf("the upcoming Grocery List buys a lemon for a Prep Session its Delivery drops:\n%s", list)
	}

	for _, moment := range []string{"2026-11-05 12:00", "2026-11-05 20:30"} {
		today.setAt(t, moment)
		if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "lemon 1 piece pantry") {
			t.Errorf("at %s, Stock does not keep the lemon of the dropped Prep Session:\n%s", moment, stock)
		}
	}
}
