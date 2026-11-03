package app_test

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/esselunga/fakestore"
)

// startCartFill starts the app as startGroceries does, against a fake
// Esselunga where the chicken breast is in stock, with the Cycle delivered
// on 2026-11-02 and ending on the 4th planned with shortCyclePlan, Lemon
// chicken with rice cooked for dinner on the 2nd: its Grocery List buys two
// packs of chicken breast, and lists basmati rice and lemons without a
// Product Mapping.
func startCartFill(t *testing.T) (*apptest.Harness, *clock, *fakellm.LLM, *fakestore.Store) {
	t.Helper()
	store := catalogStore()
	store.SetAvailable(pettoDiPollo.Code, true)
	h, llm := seededFrom(t, store)
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
	addStockItem(t, h, "olive oil", "500", "pantry")
	openCycle(t, h, "2026-11-02", "2026-11-04")
	approvePlan(t, h, shortCyclePlan(dinnerOf(map[string]any{"recipe_id": recipeID(t, h)})))
	return h, today, llm, store
}

// fillCart starts Cart Fill from the Grocery List page of the Cycle the
// Plan page shows last, waits for its Run to end and returns the Run's
// page.
func fillCart(t *testing.T, h *apptest.Harness) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, lastCyclePath(t, h, "groceries")).Body, "cart-fill")
	res := h.PostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Path, "/runs/") {
		t.Fatalf("fill the cart = %d at %s, want the Cart Fill Run's page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	h.Stream(t, res.Path+"/events", "").Rest(t)
	return h.Get(t, res.Path)
}

// cartOf is what the fake store's cart holds, as "code×packs" lines.
func cartOf(t *testing.T, store *fakestore.Store) []string {
	t.Helper()
	lines, err := store.Cart(context.Background())
	if err != nil {
		t.Fatalf("read the cart: %v", err)
	}
	var cart []string
	for _, l := range lines {
		cart = append(cart, fmt.Sprintf("%s×%d", l.Code, l.Quantity))
	}
	return cart
}

// fillScript is the model looking up the chicken breast and putting its
// two packs in the cart.
func fillScript() []fakellm.Step {
	return []fakellm.Step{
		fakellm.CallTool("look_up_product", map[string]any{"code": pettoDiPollo.Code}),
		fakellm.CallTool("set_cart_quantity", map[string]any{"code": pettoDiPollo.Code, "quantity": 2}),
		fakellm.Reply("Done."),
	}
}

// outOfStockScript is the model looking up the chicken breast, out of
// stock, and leaving it out of the cart.
func outOfStockScript() []fakellm.Step {
	return []fakellm.Step{
		fakellm.CallTool("look_up_product", map[string]any{"code": pettoDiPollo.Code}),
		fakellm.Reply("Done."),
	}
}

func TestCartFillPutsTheGroceryListInTheCart(t *testing.T) {
	h, _, llm, store := startCartFill(t)

	llm.Script(fillScript()...)
	run := text(fillCart(t, h).Body)
	if !strings.Contains(run, "Succeeded") {
		t.Fatalf("the Cart Fill Run did not succeed:\n%s", run)
	}
	if cart := strings.Join(cartOf(t, store), ", "); cart != pettoDiPollo.Code+"×2" {
		t.Errorf("the cart holds %q, want two packs of chicken breast", cart)
	}
	list := groceryList(t, h)
	for _, want := range []string{
		"Added 2 × " + pettoDiPollo.Name + ", for chicken breast",
		"Skipped, without a Product Mapping basmati rice lemon",
	} {
		if !strings.Contains(list, want) {
			t.Errorf("the Grocery List does not sum up the last Cart Fill with %q:\n%s", want, list)
		}
	}
}

// newestRun is the page of the newest Run of kind the Runs page lists,
// once it has ended.
func newestRun(t *testing.T, h *apptest.Harness, kind string) string {
	t.Helper()
	m := regexp.MustCompile(`href="(/runs/\d+)">#\d+</a></td>\s*<td>` + regexp.QuoteMeta(kind) + `</td>`).FindStringSubmatch(h.Get(t, "/runs").Body)
	if m == nil {
		t.Fatalf("the Runs page lists no %s Run:\n%s", kind, text(h.Get(t, "/runs").Body))
	}
	h.Stream(t, m[1]+"/events", "").Rest(t)
	return m[1]
}

// withoutChicken is shortCyclePlan with lemons set out where the Lemon
// chicken with rice was eaten.
func withoutChicken() map[string]any {
	plan := shortCyclePlan(lemonAt("2026-11-02", "dinner"))
	plan["prep_sessions"] = append(plan["prep_sessions"].([]session), lemonAt("2026-11-04", "lunch"))
	return plan
}

func TestOutOfStockProductLeadsToASubstitutionPlanProposal(t *testing.T) {
	h, _, llm, store := startCartFill(t)
	store.SetAvailable(pettoDiPollo.Code, false)
	replaced := proposePlan(t, h, planOf())

	llm.Script(outOfStockScript()...)
	llm.Script(
		fakellm.CallTool("submit_plan", withoutChicken()),
		fakellm.Reply("Lemons where the Lemon chicken was."),
	)
	if run := text(fillCart(t, h).Body); !strings.Contains(run, "Succeeded") {
		t.Fatalf("the Cart Fill Run did not succeed:\n%s", run)
	}
	planning := newestRun(t, h, "planning")
	if list := apptest.Link(t, h.Get(t, lastCyclePath(t, h, "groceries")).Body, "planning Run "+strings.TrimPrefix(planning, "/runs/")); list != planning {
		t.Errorf("the Cart Fill summary links %s, want the planning Run replanning around the chicken breast, %s", list, planning)
	}
	if cart := cartOf(t, store); len(cart) != 0 {
		t.Errorf("the cart holds %q, want nothing out of stock", cart)
	}
	if catalog := section(t, h.Get(t, "/catalog").Body, "Catalog Products"); !strings.Contains(catalog, "Carne › Pollo €5.60 Out of stock seen") {
		t.Errorf("the Known Catalog does not show the chicken breast out of stock, as Cart Fill saw it:\n%s", catalog)
	}

	if p := text(h.Get(t, replaced).Body); !strings.Contains(p, "Replaced by the Cart Fill substitution for chicken breast") {
		t.Errorf("the pending Plan Proposal is not replaced by the substitution:\n%s", p)
	}

	prompt := plannerRequests(llm)[0].Messages[0].Content[0].Text
	if want := "- chicken breast, bought as " + pettoDiPollo.Name; !strings.Contains(prompt, want) {
		t.Errorf("the planning Run is not told Esselunga is out of the chicken breast:\n%s", prompt)
	}
	page := h.Get(t, proposalPaths(t, h)[0])
	if p := text(page.Body); !strings.Contains(p, "Substitution Plan Proposal") || !strings.Contains(p, "pending") {
		t.Fatalf("the planning Run's Plan Proposal is not a pending Substitution Plan Proposal:\n%s", p)
	}
	approve := apptest.Form(t, page.Body, "approve")
	if res := h.PostForm(t, approve.Action, approve.Values); res.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d:\n%s", res.StatusCode, text(res.Body))
	}
	if list := groceryList(t, h); strings.Contains(list, pettoDiPollo.Aisle) {
		t.Errorf("the Grocery List still buys chicken breast once the substitution is approved:\n%s", list)
	}
}

func TestCartFillAfterAMidRunFailureLeavesNoDuplicatesInTheCart(t *testing.T) {
	h, _, llm, store := startCartFill(t)

	// The session expires once the chicken breast is in the cart.
	llm.Script(
		fakellm.CallTool("look_up_product", map[string]any{"code": pettoDiPollo.Code}),
		fakellm.CallTool("set_cart_quantity", map[string]any{"code": pettoDiPollo.Code, "quantity": 2}),
		fakellm.Then(func(agent.Request) fakellm.Step {
			store.LogOut()
			return fakellm.CallTool("look_up_product", map[string]any{"code": pettoDiPollo.Code})
		}),
	)
	run := text(fillCart(t, h).Body)
	loggedOut := "the Esselunga session is logged out: log in to Esselunga in Dandori's browser, then run this again"
	if !strings.Contains(run, "Run failed") || !strings.Contains(run, loggedOut) {
		t.Fatalf("the Cart Fill Run did not stop on the logged-out session:\n%s", run)
	}
	if list := groceryList(t, h); !strings.Contains(list, "The last Cart Fill failed: "+loggedOut) {
		t.Errorf("the Grocery List does not say why the last Cart Fill failed:\n%s", list)
	}

	store.LogIn()
	llm.Script(fillScript()...)
	if run := text(fillCart(t, h).Body); !strings.Contains(run, "Succeeded") {
		t.Fatalf("Cart Fill run again did not succeed:\n%s", run)
	}
	if cart := strings.Join(cartOf(t, store), ", "); cart != pettoDiPollo.Code+"×2" {
		t.Errorf("after filling it again the cart holds %q, want the same two packs of chicken breast", cart)
	}
}

func TestLoggedOutSessionStopsCartFillBeforeAskingTheModel(t *testing.T) {
	h, _, llm, store := startCartFill(t)
	store.LogOut()
	asked := len(llm.Requests())

	run := text(fillCart(t, h).Body)
	if !strings.Contains(run, "Run failed") || !strings.Contains(run, "the Esselunga session is logged out") {
		t.Errorf("the Cart Fill Run did not stop on the logged-out session:\n%s", run)
	}
	if n := len(llm.Requests()) - asked; n != 0 {
		t.Errorf("the model was asked %d times, want none while logged out", n)
	}
}

func TestProductLeftOutOfTheCartFailsCartFill(t *testing.T) {
	h, _, llm, store := startCartFill(t)

	// The model looks the chicken breast up, finds it in stock, and ends
	// without putting it in the cart.
	llm.Script(
		fakellm.CallTool("look_up_product", map[string]any{"code": pettoDiPollo.Code}),
		fakellm.Reply("Done."),
	)
	run := text(fillCart(t, h).Body)
	failed := "these are not in the cart: " + pettoDiPollo.Name + ". Fill the cart again to retry them"
	if !strings.Contains(run, "Run failed") || !strings.Contains(run, failed) {
		t.Errorf("the Cart Fill Run did not fail saying %q:\n%s", failed, run)
	}
	list := groceryList(t, h)
	for _, want := range []string{
		"The last Cart Fill failed: " + failed,
		"Failed: filling the cart again retries them " + pettoDiPollo.Name + ", for chicken breast",
		"Skipped, without a Product Mapping basmati rice lemon",
	} {
		if !strings.Contains(list, want) {
			t.Errorf("the Grocery List does not show %q for the failed Cart Fill:\n%s", want, list)
		}
	}
	if cart := cartOf(t, store); len(cart) != 0 {
		t.Errorf("the cart holds %q, want nothing", cart)
	}
}

func TestCartFillPutsOnlyWhatTheGroceryListBuysInTheCart(t *testing.T) {
	h, _, llm, store := startCartFill(t)

	// The model tries three packs, then the lentils no line buys, before
	// doing as the Grocery List says.
	llm.Script(
		fakellm.CallTool("set_cart_quantity", map[string]any{"code": pettoDiPollo.Code, "quantity": 2}),
		fakellm.CallTool("look_up_product", map[string]any{"code": pettoDiPollo.Code}),
		fakellm.CallTool("set_cart_quantity", map[string]any{"code": pettoDiPollo.Code, "quantity": 3}),
		fakellm.CallTool("look_up_product", map[string]any{"code": lenticchie.Code}),
	)
	llm.Script(fillScript()[1:]...)
	if run := text(fillCart(t, h).Body); !strings.Contains(run, "Succeeded") {
		t.Fatalf("the Cart Fill Run did not succeed:\n%s", run)
	}
	var refused []string
	for _, req := range llm.Requests() {
		for _, m := range req.Messages {
			for _, b := range m.Content {
				if b.IsError {
					refused = append(refused, b.Text)
				}
			}
		}
	}
	for _, want := range []string{
		"look up product " + pettoDiPollo.Code + " before putting it in the cart",
		"the Grocery List buys 2 of " + pettoDiPollo.Name + ", not 3",
		"product " + lenticchie.Code + " is not on the Grocery List",
	} {
		if !slices.ContainsFunc(refused, func(r string) bool { return strings.Contains(r, want) }) {
			t.Errorf("the model was not told %q; it was told %q", want, refused)
		}
	}
	if cart := strings.Join(cartOf(t, store), ", "); cart != pettoDiPollo.Code+"×2" {
		t.Errorf("the cart holds %q, want two packs of chicken breast", cart)
	}
}

func TestSubstitutionWaitingForTheCurrentCycleFlagsTheUpcomingPlan(t *testing.T) {
	h, today, llm, store := startCartFill(t)
	// The first Cycle is current from the 2nd; the next one, delivered on
	// the 8th, buys chicken breast for dinner on the 9th, while a Plan
	// Proposal for the current Cycle is pending.
	today.set(t, "2026-11-03")
	openCycleIn(t, h, "2026-11-08", "18:00", "20:00", "")
	approvePlan(t, h, planOf(cooks(recipeID(t, h), "2026-11-09", "dinner", eat("fridge", "2026-11-09", "dinner"))))
	pending := proposePlanFor(t, h, "2026-11-02", planFrom("2026-11-04", "breakfast"))
	store.SetAvailable(pettoDiPollo.Code, false)

	llm.Script(outOfStockScript()...)
	if run := text(fillCart(t, h).Body); !strings.Contains(run, "Succeeded") {
		t.Fatalf("the Cart Fill Run did not succeed:\n%s", run)
	}
	list := text(h.Get(t, lastCyclePath(t, h, "groceries")).Body)
	if want := "Substituted: the planner can't replan now, since a Plan Proposal for the current Cycle is pending: approve or reject it: the Plan needs a new Plan Proposal " + pettoDiPollo.Name + ", for chicken breast: out of stock"; !strings.Contains(list, want) {
		t.Errorf("the Cart Fill summary does not say the planner can't replan yet:\n%s", list)
	}
	if n := planningRunsGoing(t, h); n != 0 || len(plannerRequests(llm)) != 0 {
		t.Errorf("a planning Run started while the current Cycle's Plan Proposal is pending")
	}
	if page := text(h.Get(t, "/plan").Body); !strings.Contains(page, "Esselunga is out of chicken breast: this Plan needs a new Plan Proposal.") {
		t.Errorf("the upcoming Plan is not flagged for what Esselunga is out of:\n%s", page)
	}
	if p := text(h.Get(t, pending).Body); !strings.Contains(p, "pending") {
		t.Errorf("the current Cycle's Plan Proposal did not stay pending:\n%s", p)
	}
}

func TestCartFillIsOfferedOnlyWhileTheDeliveryIsTentative(t *testing.T) {
	h, today, _, _ := startCartFill(t)
	offered := func() bool {
		return strings.Contains(h.Get(t, lastCyclePath(t, h, "groceries")).Body, `id="cart-fill"`)
	}
	if !offered() {
		t.Fatalf("Cart Fill is not offered on a tentative Delivery's Grocery List:\n%s", groceryList(t, h))
	}
	action := strings.TrimSuffix(lastCyclePath(t, h, "groceries"), "/groceries") + "/cart-fill"

	changeDelivery(t, h, map[string]string{"ordered": "yes"})
	if offered() {
		t.Errorf("Cart Fill is offered once the Delivery is ordered")
	}
	res := h.PostForm(t, action, nil)
	if want := "The cart can't be filled: its Delivery is ordered, so its Grocery List is fixed."; res.StatusCode != http.StatusConflict || !strings.Contains(text(res.Body), want) {
		t.Errorf("fill the cart of an ordered Delivery = %d, want 409 saying %q:\n%s", res.StatusCode, want, text(res.Body))
	}

	today.setAt(t, "2026-11-02 08:00")
	if offered() {
		t.Errorf("Cart Fill is offered once the Delivery has arrived")
	}
}

func TestCartFillStartedTwiceFollowsTheRunGoing(t *testing.T) {
	h, _, llm, _ := startCartFill(t)
	llm.Script(fakellm.Hang())
	form := apptest.Form(t, h.Get(t, lastCyclePath(t, h, "groceries")).Body, "cart-fill")
	first := h.PostForm(t, form.Action, form.Values)
	if !strings.HasPrefix(first.Path, "/runs/") {
		t.Fatalf("fill the cart landed at %s, want its Run's page", first.Path)
	}
	if again := h.PostForm(t, form.Action, form.Values); again.Path != first.Path {
		t.Errorf("filling the cart again while it fills landed at %s, want the Run going, %s", again.Path, first.Path)
	}
	if link := apptest.Link(t, h.Get(t, lastCyclePath(t, h, "groceries")).Body, "follow Cart Fill"); link != first.Path {
		t.Errorf("the Grocery List follows %s, want the Cart Fill going, %s", link, first.Path)
	}
}

func TestCartFillRunAgainKeepsThePendingSubstitution(t *testing.T) {
	h, _, llm, store := startCartFill(t)
	store.SetAvailable(pettoDiPollo.Code, false)
	llm.Script(outOfStockScript()...)
	llm.Script(
		fakellm.CallTool("submit_plan", withoutChicken()),
		fakellm.Reply("Lemons where the Lemon chicken was."),
	)
	fillCart(t, h)
	planning := newestRun(t, h, "planning")
	substitution := proposalPaths(t, h)[0]

	llm.Script(outOfStockScript()...)
	if run := text(fillCart(t, h).Body); !strings.Contains(run, "Succeeded") {
		t.Fatalf("Cart Fill run again did not succeed:\n%s", run)
	}
	if p := text(h.Get(t, substitution).Body); !strings.Contains(p, "Substitution Plan Proposal") || !strings.Contains(p, "pending") {
		t.Errorf("filling the cart again ended the pending Substitution Plan Proposal:\n%s", p)
	}
	if n := len(plannerRequests(llm)); n != 2 {
		t.Errorf("the planner was asked %d times, want only the first substitution's two turns", n)
	}
	if list, want := groceryList(t, h), "their substitution Plan Proposal, from planning Run "+strings.TrimPrefix(planning, "/runs/")+" , is pending"; !strings.Contains(list, want) {
		t.Errorf("the Cart Fill summary does not point at the pending substitution:\n%s", list)
	}
}

func TestDeliveryOrderedDuringCartFillKeepsItsPlan(t *testing.T) {
	h, _, llm, store := startCartFill(t)
	store.SetAvailable(pettoDiPollo.Code, false)
	pending := proposePlan(t, h, planOf())

	// The user marks the Delivery ordered while Cart Fill looks the chicken
	// breast up.
	llm.Script(
		fakellm.CallTool("look_up_product", map[string]any{"code": pettoDiPollo.Code}),
		fakellm.Then(func(agent.Request) fakellm.Step {
			changeDelivery(t, h, map[string]string{"ordered": "yes"})
			return fakellm.Reply("Done.")
		}),
	)
	if run := text(fillCart(t, h).Body); !strings.Contains(run, "Succeeded") {
		t.Fatalf("the Cart Fill Run did not succeed:\n%s", run)
	}
	if n := planningRunsGoing(t, h); n != 0 || len(plannerRequests(llm)) != 0 {
		t.Errorf("a planning Run replans the Cycle whose Delivery is ordered")
	}
	if p := text(h.Get(t, pending).Body); !strings.Contains(p, "pending") {
		t.Errorf("the Plan Proposal was replaced once the Delivery is ordered:\n%s", p)
	}
	if list := groceryList(t, h); !strings.Contains(list, "Substituted: its Delivery is ordered, so its Plan stays as it is") {
		t.Errorf("the Cart Fill summary does not say the ordered Delivery keeps its Plan:\n%s", list)
	}
}
