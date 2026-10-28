package app_test

import (
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/esselunga"
	"github.com/simopzz/dandori/internal/esselunga/fakestore"
)

// Products as Esselunga shows them: lentils with a nutrition label, bananas
// sold by weight without one, and chicken breast.
var (
	lenticchie = esselunga.Product{
		Code: "412106", Name: "Esselunga Lenticchie 400 g", Brand: "Esselunga",
		Quantity: "400g · Peso sgocciolato: 240g", Aisle: "Dispensa › Legumi e cereali",
		Price: 0.73, Available: true, NutritionPer: "Per 100g",
		Nutrition: []esselunga.NutritionRow{
			{Label: "Energia", Value: "373 kJ"}, {Label: "", Value: "89 kcal"},
			{Label: "Grassi", Value: "0,5 g"}, {Label: "Carboidrati", Value: "13,9 g"}, {Label: "Proteine", Value: "5,0 g"},
		},
	}
	banane = esselunga.Product{
		Code: "751030", Name: "Esselunga Bio banane", Brand: "Esselunga",
		SoldByWeight: true, Aisle: "Frutta › Frutta fresca › Frutta esotica", Price: 2.29, Available: true,
	}
	pettoDiPollo = esselunga.Product{
		Code: "708114", Name: "Fileni Petto di Pollo a Fette 320 g", Brand: "Fileni",
		Quantity: "320 g", Aisle: "Carne › Pollo", Price: 5.60, Available: false, NutritionPer: "Per 100g",
		Nutrition: []esselunga.NutritionRow{{Label: "Energia", Value: "100 kcal"}, {Label: "Proteine", Value: "23 g"}},
	}
)

// orderedStore is a logged-in fake Esselunga whose order history holds two
// orders of those products.
func orderedStore() *fakestore.Store {
	store := fakestore.New(lenticchie, banane, pettoDiPollo)
	store.Ordered(esselunga.Order{ID: "127737508", DeliveredOn: time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)},
		esselunga.OrderLine{Code: "412106", Name: "Esselunga Lenticchie 400 g", Quantity: 2, Price: 0.73},
		esselunga.OrderLine{Code: "708114", Name: "Fileni Petto di Pollo a Fette", Quantity: 2, Price: 4.67},
	)
	store.Ordered(esselunga.Order{ID: "128019105", DeliveredOn: time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)},
		esselunga.OrderLine{Code: "751030", Name: "Esselunga Bio banane", Quantity: 1, Price: 1.95, Weight: "850 g"},
		esselunga.OrderLine{Code: "412106", Name: "Esselunga Lenticchie 400 g", Quantity: 2, Price: 0.73},
	)
	return store
}

// startCatalog starts the app with a fake LLM and store, and the throttle
// at throttle.
func startCatalog(t *testing.T, store *fakestore.Store, throttle time.Duration) (*apptest.Harness, *fakellm.LLM) {
	t.Helper()
	llm := fakellm.New()
	h := apptest.Start(t,
		apptest.WithPorts(func(p *app.Ports) { p.LLM, p.Store = llm, store }),
		apptest.WithConfig(func(c *app.Config) { c.StoreThrottle = throttle }),
	)
	return h, llm
}

// seedScript is the model reading the order history, looking up its three
// products and saving them as Catalog Products.
func seedScript() []fakellm.Step {
	return []fakellm.Step{
		fakellm.CallTool("read_order_history", map[string]any{}),
		fakellm.CallTools("Looking up each product.",
			fakellm.Call{Tool: "look_up_product", Input: map[string]any{"code": "412106"}},
			fakellm.Call{Tool: "look_up_product", Input: map[string]any{"code": "751030"}},
			fakellm.Call{Tool: "look_up_product", Input: map[string]any{"code": "708114"}},
		),
		fakellm.CallTools("",
			fakellm.Call{Tool: "save_catalog_product", Input: map[string]any{
				"code": "412106", "pack_quantity": 400, "unit": "g",
				"nutrition": map[string]any{"kcal": 89, "protein_g": 5, "carbs_g": 13.9, "fat_g": 0.5}}},
			fakellm.Call{Tool: "save_catalog_product", Input: map[string]any{
				"code": "751030", "pack_quantity": 850, "unit": "g"}},
			fakellm.Call{Tool: "save_catalog_product", Input: map[string]any{
				"code": "708114", "pack_quantity": 320, "unit": "g",
				"nutrition": map[string]any{"kcal": 100, "protein_g": 23, "carbs_g": 0, "fat_g": 1.2}}},
		),
		fakellm.Reply("Seeded three Catalog Products."),
	}
}

// startRun posts the form with id on the Known Catalog page, which starts
// a Run, waits for the Run to end and returns its page.
func startRun(t *testing.T, h *apptest.Harness, id string) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, "/catalog").Body, id)
	res := h.PostForm(t, form.Action, form.Values)
	runID, ok := strings.CutPrefix(res.Path, "/catalog/runs/")
	if res.StatusCode != http.StatusOK || !ok {
		t.Fatalf("%s = %d at %s, want the Run's page:\n%s", id, res.StatusCode, res.Path, text(res.Body))
	}
	h.Stream(t, "/runs/"+runID+"/events", "").Rest(t)
	return h.Get(t, res.Path)
}

func TestRunNotesShowOnTheKnownCatalogRendered(t *testing.T) {
	h, llm := startCatalog(t, orderedStore(), time.Millisecond)
	script := seedScript()
	script[len(script)-1] = fakellm.Reply("Seeded three Catalog Products.\n\n- **Not saved:** cotton buds, not food.\n- <script>alert(1)</script>")
	llm.Script(script...)

	res := startRun(t, h, "seed")
	if !strings.Contains(res.Body, "<strong>Not saved:</strong>") {
		t.Errorf("seed Run's page does not show its notes rendered:\n%s", res.Body)
	}
	if strings.Contains(res.Body, "**Not saved:**") || strings.Contains(res.Body, "<script>alert") {
		t.Errorf("seed Run's page shows its notes' Markdown raw, or the HTML in them:\n%s", res.Body)
	}
	back := apptest.Link(t, res.Body, "Back to the Known Catalog")

	catalog := h.Get(t, back)
	for _, want := range []string{"Notes from the last Run", "Not saved: cotton buds, not food."} {
		if !strings.Contains(text(catalog.Body), want) {
			t.Errorf("Known Catalog does not show %q:\n%s", want, text(catalog.Body))
		}
	}
	if !strings.Contains(catalog.Body, "<strong>Not saved:</strong>") {
		t.Errorf("Known Catalog does not show the last Run's notes rendered:\n%s", catalog.Body)
	}
	if run := h.Get(t, apptest.Link(t, catalog.Body, "Run #")); !strings.Contains(text(run.Body), "Seeded three Catalog Products.") {
		t.Errorf("the last Run's link does not lead to its page:\n%s", text(run.Body))
	}
}

func TestSeedRunFillsTheKnownCatalogFromOrderHistory(t *testing.T) {
	store := orderedStore()
	h, llm := startCatalog(t, store, time.Millisecond)
	if page := text(h.Get(t, "/catalog").Body); !strings.Contains(page, "No Catalog Products yet.") {
		t.Fatalf("Known Catalog is not empty before the seed:\n%s", page)
	}
	llm.Script(seedScript()...)

	run := text(startRun(t, h, "seed").Body)
	for _, want := range []string{
		"Run succeeded",
		"Store Esselunga: checking the session",
		"Store Esselunga: reading the last 10 orders",
		"Store Esselunga: looking up product 412106",
		// What the model was told the order history holds, each product once.
		`{"code":"751030","name":"Esselunga Bio banane","times_bought":1,"last_price":1.95,"weight":"850 g"}`,
		`{"code":"412106","name":"Esselunga Lenticchie 400 g","times_bought":2,"last_price":0.73}`,
	} {
		if !strings.Contains(run, want) {
			t.Errorf("seed Run's trace does not show %q:\n%s", want, run)
		}
	}

	catalog := text(h.Get(t, "/catalog").Body)
	for _, want := range []string{
		"Esselunga Lenticchie 400 g", "400 g", "89 kcal · 5 g protein per 100 g", "Dispensa › Legumi e cereali", "€0.73",
		// Sold by weight: a pack is one bunch, at €2.29/kg.
		"Esselunga Bio banane", "850 g", "No nutrition label", "€1.95",
		"Fileni Petto di Pollo a Fette 320 g", "Out of stock",
	} {
		if !strings.Contains(catalog, want) {
			t.Errorf("Known Catalog does not show %q:\n%s", want, catalog)
		}
	}
}

func TestStoreActionsAreThrottledAndEveryWaitIsTraced(t *testing.T) {
	h, llm := startCatalog(t, orderedStore(), 300*time.Millisecond)
	llm.Script(seedScript()...)

	start := time.Now()
	run := text(startRun(t, h, "seed").Body)
	// Seven store actions: the session, the order history, its two orders
	// and three products, each at least 300ms after the one before.
	if took := time.Since(start); took < 6*300*time.Millisecond {
		t.Errorf("seven store actions took %s, want at least 1.8s at 300ms apart", took)
	}
	for _, want := range []string{
		"before reading the last 10 orders",
		"before reading order 128019105",
		"before reading order 127737508",
		"before looking up product 412106",
		"before looking up product 751030",
		"before looking up product 708114",
	} {
		if !strings.Contains(run, "Wait Waiting") || !strings.Contains(run, want) {
			t.Errorf("seed Run's trace does not show a throttle wait %q:\n%s", want, run)
		}
	}
	if n := strings.Count(run, "Wait Waiting"); n != 6 {
		t.Errorf("seed Run's trace shows %d throttle waits, want one before each action after the first, 6:\n%s", n, run)
	}
}

// The gap between store actions varies, by up to half the throttle, so
// Dandori doesn't tick like a clock.
func TestStoreActionsAreSpacedWithJitter(t *testing.T) {
	h, llm := startCatalog(t, orderedStore(), 300*time.Millisecond)
	llm.Script(seedScript()...)

	run := text(startRun(t, h, "seed").Body)
	var waits []time.Duration
	for _, m := range regexp.MustCompile(`Waiting (\S+) before`).FindAllStringSubmatch(run, -1) {
		d, err := time.ParseDuration(m[1])
		if err != nil {
			t.Fatalf("throttle wait %q: %v", m[1], err)
		}
		if d > 450*time.Millisecond {
			t.Errorf("throttle waited %s, want at most 450ms at 300ms with jitter", d)
		}
		waits = append(waits, d)
	}
	if len(waits) != 6 {
		t.Fatalf("seed Run's trace shows %d throttle waits, want 6:\n%s", len(waits), run)
	}
	if spread := slices.Max(waits) - slices.Min(waits); spread < 20*time.Millisecond {
		t.Errorf("throttle waits %v are all about the same, want them to vary", waits)
	}
}

func TestLoggedOutSessionStopsTheSeedBeforeAskingTheModel(t *testing.T) {
	store := orderedStore()
	store.LogOut()
	h, llm := startCatalog(t, store, time.Millisecond)

	run := text(startRun(t, h, "seed").Body)
	for _, want := range []string{"Run failed", "the Esselunga session is logged out: log in to Esselunga in Dandori's browser, then run this again"} {
		if !strings.Contains(run, want) {
			t.Errorf("seed Run's trace does not show %q:\n%s", want, run)
		}
	}
	if n := len(llm.Requests()); n != 0 {
		t.Errorf("the model was asked %d times, want none while logged out", n)
	}
	if catalog := text(h.Get(t, "/catalog").Body); !strings.Contains(catalog, "No Catalog Products yet.") {
		t.Errorf("Known Catalog is not empty after a logged-out seed:\n%s", catalog)
	}
}

func TestSessionLoggingOutMidRunStopsTheSeed(t *testing.T) {
	store := orderedStore()
	h, llm := startCatalog(t, store, time.Millisecond)
	loggedOut := make(chan struct{})
	llm.Script(
		fakellm.CallTool("read_order_history", map[string]any{}),
		fakellm.CallTool("look_up_product", map[string]any{"code": "412106"}).After(loggedOut),
		// Never asked: the Run stops at the failed lookup.
		fakellm.Reply("Seeded."),
	)
	go func() {
		// Once the order history is read, down to its oldest order's
		// lines, the session expires.
		for !slices.Contains(store.Actions(), "OrderLines 127737508") {
			time.Sleep(time.Millisecond)
		}
		store.LogOut()
		close(loggedOut)
	}()

	run := text(startRun(t, h, "seed").Body)
	if !strings.Contains(run, "Run failed") || !strings.Contains(run, "the Esselunga session is logged out") {
		t.Errorf("seed Run did not stop on the logged-out session:\n%s", run)
	}
	if n := len(llm.Requests()); n != 2 {
		t.Errorf("the model was asked %d times, want 2: the Run stops instead of telling it", n)
	}
}

// mappingForm reads the Product Mapping form of the Ingredient named
// ingredient from the Known Catalog page.
func mappingForm(t *testing.T, h *apptest.Harness, ingredient string) apptest.FormData {
	t.Helper()
	page := h.Get(t, "/catalog").Body
	_, row, ok := strings.Cut(page, `<span class="row-title">`+ingredient+`</span>`)
	if !ok {
		t.Fatalf("Known Catalog has no row for %s:\n%s", ingredient, text(page))
	}
	m := regexp.MustCompile(`id="(mapping-\d+)"`).FindStringSubmatch(row)
	if m == nil {
		t.Fatalf("%s has no Product Mapping form:\n%s", ingredient, text(page))
	}
	return apptest.Form(t, page, m[1])
}

// mappingShown is what the Known Catalog page shows for an Ingredient's
// Product Mapping, up to the next Ingredient's.
func mappingShown(t *testing.T, h *apptest.Harness, ingredient string) string {
	t.Helper()
	page := h.Get(t, "/catalog").Body
	_, row, ok := strings.Cut(page, `<span class="row-title">`+ingredient+`</span>`)
	if !ok {
		t.Fatalf("Known Catalog has no row for %s:\n%s", ingredient, text(page))
	}
	row, _, _ = strings.Cut(row, `<span class="row-title">`)
	return text(row)
}

// requestOf is the first request a Run whose instructions contain system
// sent the model.
func requestOf(t *testing.T, llm *fakellm.LLM, system string) agent.Request {
	t.Helper()
	for _, req := range llm.Requests() {
		if strings.Contains(req.System, system) {
			return req
		}
	}
	t.Fatalf("no request to the model had instructions containing %q", system)
	return agent.Request{}
}

func TestSeedProposesAProductMappingTheUserConfirms(t *testing.T) {
	h, llm := startCatalog(t, orderedStore(), time.Millisecond)
	acceptFirstRecipe(t, h, llm)
	script := seedScript()
	last := len(script) - 1
	llm.Script(append(script[:last:last],
		fakellm.CallTool("propose_product_mapping", map[string]any{"ingredient": "chicken breast", "code": "708114"}),
		script[last])...)

	startRun(t, h, "seed")
	prompt := requestOf(t, llm, "You seed the Known Catalog").Messages[0].Content[0].Text
	for _, want := range []string{"Ingredients without a Product Mapping:", "- chicken breast (g)", "- lemon (piece)"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("seed Run was not told %q:\n%s", want, prompt)
		}
	}
	if got := mappingShown(t, h, "chicken breast"); !strings.HasPrefix(got, "Fileni Petto di Pollo a Fette 320 g Proposed") {
		t.Errorf("chicken breast shows %q, want its proposed mapping", got)
	}
	if got := mappingShown(t, h, "basmati rice"); !strings.HasPrefix(got, "No Product Mapping") {
		t.Errorf("basmati rice shows %q, want no Product Mapping", got)
	}

	form := mappingForm(t, h, "chicken breast")
	res := h.PostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusOK || res.Path != "/catalog" {
		t.Fatalf("confirm mapping = %d at %s, want the Known Catalog:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	if got := mappingShown(t, h, "chicken breast"); !strings.HasPrefix(got, "Fileni Petto di Pollo a Fette 320 g Change") {
		t.Errorf("chicken breast shows %q after it was confirmed, want the mapping no longer proposed", got)
	}
}

// latte is a product sold in millilitres.
var latte = esselunga.Product{
	Code: "302714", Name: "Esselunga Latte parzialmente scremato UHT 1000 ml", Brand: "Esselunga",
	Quantity: "1000 ml", Aisle: "Latte, yogurt e uova › Latte", Price: 1.09, Available: true,
}

// seededWithIngredients starts the app with the Ingredients of
// pastedRecipe known and the Known Catalog seeded with lentils, chicken
// breast and milk, without Product Mappings.
func seededWithIngredients(t *testing.T) (*apptest.Harness, *fakellm.LLM) {
	t.Helper()
	store := fakestore.New(lenticchie, pettoDiPollo, latte)
	store.Ordered(esselunga.Order{ID: "1"},
		esselunga.OrderLine{Code: lenticchie.Code, Name: lenticchie.Name, Quantity: 1},
		esselunga.OrderLine{Code: pettoDiPollo.Code, Name: pettoDiPollo.Name, Quantity: 1},
		esselunga.OrderLine{Code: latte.Code, Name: latte.Name, Quantity: 1},
	)
	h, llm := startCatalog(t, store, time.Millisecond)
	acceptFirstRecipe(t, h, llm)
	llm.Script(
		fakellm.CallTools("",
			fakellm.Call{Tool: "look_up_product", Input: map[string]any{"code": lenticchie.Code}},
			fakellm.Call{Tool: "look_up_product", Input: map[string]any{"code": pettoDiPollo.Code}},
			fakellm.Call{Tool: "look_up_product", Input: map[string]any{"code": latte.Code}},
		),
		fakellm.CallTools("",
			fakellm.Call{Tool: "save_catalog_product", Input: map[string]any{"code": lenticchie.Code, "pack_quantity": 400, "unit": "g"}},
			fakellm.Call{Tool: "save_catalog_product", Input: map[string]any{"code": pettoDiPollo.Code, "pack_quantity": 320, "unit": "g"}},
			fakellm.Call{Tool: "save_catalog_product", Input: map[string]any{"code": latte.Code, "pack_quantity": 1000, "unit": "ml"}},
		),
		fakellm.Reply("Seeded."),
	)
	if run := text(startRun(t, h, "seed").Body); !strings.Contains(run, "Run succeeded") {
		t.Fatalf("seed failed:\n%s", run)
	}
	return h, llm
}

// productID is the id the Known Catalog page's forms give the Catalog
// Product named name.
func productID(t *testing.T, h *apptest.Harness, name string) string {
	t.Helper()
	page := h.Get(t, "/catalog").Body
	m := regexp.MustCompile(`<option value="(\d+)"[^>]*>` + regexp.QuoteMeta(name) + `<`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no form offers %s:\n%s", name, page)
	}
	return m[1]
}

func TestUserSetsChangesAndRemovesAProductMapping(t *testing.T) {
	h, _ := seededWithIngredients(t)

	form := mappingForm(t, h, "basmati rice")
	form.Values.Set("catalog_product", productID(t, h, lenticchie.Name))
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusOK || res.Path != "/catalog" {
		t.Fatalf("set mapping = %d at %s:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	if got := mappingShown(t, h, "basmati rice"); !strings.HasPrefix(got, lenticchie.Name+" Change") {
		t.Errorf("basmati rice shows %q, want it mapped to lentils by the user", got)
	}

	form = mappingForm(t, h, "basmati rice")
	form.Values.Set("catalog_product", productID(t, h, pettoDiPollo.Name))
	h.PostForm(t, form.Action, form.Values)
	if got := mappingShown(t, h, "basmati rice"); !strings.HasPrefix(got, pettoDiPollo.Name+" Change") {
		t.Errorf("basmati rice shows %q, want its mapping changed", got)
	}

	form = mappingForm(t, h, "basmati rice")
	form.Values.Set("catalog_product", "")
	h.PostForm(t, form.Action, form.Values)
	if got := mappingShown(t, h, "basmati rice"); !strings.HasPrefix(got, "No Product Mapping") {
		t.Errorf("basmati rice shows %q, want its mapping removed", got)
	}
}

func TestProductMappingToAProductSoldInAnotherUnitIsRejected(t *testing.T) {
	h, _ := seededWithIngredients(t)
	// Only olive oil's form offers milk; a tampered or stale form can
	// still send it for chicken breast.
	form := mappingForm(t, h, "chicken breast")
	form.Values.Set("catalog_product", productID(t, h, latte.Name))

	res := h.PostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("mapping to a product in ml = %d, want 422:\n%s", res.StatusCode, text(res.Body))
	}
	if want := "chicken breast is measured in g, but " + latte.Name + " is sold in ml"; !strings.Contains(text(res.Body), want) {
		t.Errorf("rejection does not say %q:\n%s", want, text(res.Body))
	}
	if got := mappingShown(t, h, "chicken breast"); !strings.HasPrefix(got, "No Product Mapping") {
		t.Errorf("chicken breast shows %q, want no mapping after the rejection", got)
	}
}

func TestProposalRunProposesMappingsOnlyForIngredientsWithoutOne(t *testing.T) {
	h, llm := seededWithIngredients(t)
	form := mappingForm(t, h, "basmati rice")
	form.Values.Set("catalog_product", productID(t, h, lenticchie.Name))
	h.PostForm(t, form.Action, form.Values)

	llm.Script(
		fakellm.CallTools("",
			fakellm.Call{Tool: "propose_product_mapping", Input: map[string]any{"ingredient": "chicken breast", "code": pettoDiPollo.Code}},
			// The user's own mapping is never replaced.
			fakellm.Call{Tool: "propose_product_mapping", Input: map[string]any{"ingredient": "basmati rice", "code": pettoDiPollo.Code}},
			// Nor is a product in another unit proposed.
			fakellm.Call{Tool: "propose_product_mapping", Input: map[string]any{"ingredient": "olive oil", "code": lenticchie.Code}},
		),
		fakellm.Reply("Proposed one Product Mapping."),
	)
	run := text(startRun(t, h, "propose").Body)
	for _, want := range []string{"Run succeeded", "basmati rice already has a Product Mapping", "olive oil is measured in ml, but"} {
		if !strings.Contains(run, want) {
			t.Errorf("proposal Run's trace does not show %q:\n%s", want, run)
		}
	}
	prompt := requestOf(t, llm, "You propose Product Mappings").Messages[0].Content[0].Text
	if !strings.Contains(prompt, "- chicken breast (g)") || strings.Contains(prompt, "- basmati rice (g)") {
		t.Errorf("proposal Run was not told just the Ingredients without a mapping:\n%s", prompt)
	}

	if got := mappingShown(t, h, "chicken breast"); !strings.HasPrefix(got, pettoDiPollo.Name+" Proposed") {
		t.Errorf("chicken breast shows %q, want the proposed mapping", got)
	}
	if got := mappingShown(t, h, "basmati rice"); !strings.HasPrefix(got, lenticchie.Name+" Change") {
		t.Errorf("basmati rice shows %q, want the user's mapping kept", got)
	}
	if got := mappingShown(t, h, "olive oil"); !strings.HasPrefix(got, "No Product Mapping") {
		t.Errorf("olive oil shows %q, want no mapping", got)
	}
}

func TestProposalIsOfferedOnlyWhileAnIngredientHasNoMapping(t *testing.T) {
	h, _ := startCatalog(t, orderedStore(), time.Millisecond)
	if page := h.Get(t, "/catalog").Body; strings.Contains(page, `id="propose"`) {
		t.Errorf("a proposal is offered with nothing to map:\n%s", text(page))
	}

	h, _ = seededWithIngredients(t)
	if page := h.Get(t, "/catalog").Body; !strings.Contains(page, `id="propose"`) {
		t.Errorf("no proposal is offered while Ingredients have no mapping:\n%s", text(page))
	}
}

func TestStorePortHasNoCheckoutOrPayment(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)checkout|check_out|pay|purchase|place_?order|confirm_?order`)
	for m := range reflect.TypeFor[esselunga.Store]().Methods() {
		if forbidden.MatchString(m.Name) {
			t.Errorf("the store port offers %s; Cart Fill never checks out (ADR-0005)", m.Name)
		}
	}

	h, llm := startCatalog(t, orderedStore(), time.Millisecond)
	llm.Script(seedScript()...)
	startRun(t, h, "seed")
	for _, tool := range requestOf(t, llm, "You seed the Known Catalog").Tools {
		if forbidden.MatchString(tool.Name) || forbidden.MatchString(tool.Description) {
			t.Errorf("the seed Run offers the agent %s: %s", tool.Name, tool.Description)
		}
	}
}

func TestSeedingAgainRefreshesProductsAndKeepsTheirMappings(t *testing.T) {
	store := fakestore.New(lenticchie, pettoDiPollo, latte)
	store.Ordered(esselunga.Order{ID: "1"}, esselunga.OrderLine{Code: pettoDiPollo.Code, Name: pettoDiPollo.Name, Quantity: 1})
	h, llm := startCatalog(t, store, time.Millisecond)
	acceptFirstRecipe(t, h, llm)
	seedChicken := func() {
		llm.Script(
			fakellm.CallTool("look_up_product", map[string]any{"code": pettoDiPollo.Code}),
			fakellm.CallTool("save_catalog_product", map[string]any{"code": pettoDiPollo.Code, "pack_quantity": 320, "unit": "g"}),
			fakellm.Reply("Seeded."),
		)
		startRun(t, h, "seed")
	}
	seedChicken()
	form := mappingForm(t, h, "chicken breast")
	form.Values.Set("catalog_product", productID(t, h, pettoDiPollo.Name))
	h.PostForm(t, form.Action, form.Values)
	if catalog := text(h.Get(t, "/catalog").Body); !strings.Contains(catalog, "Out of stock") {
		t.Fatalf("chicken breast is not out of stock after the first seed:\n%s", catalog)
	}

	store.SetAvailable(pettoDiPollo.Code, true)
	seedChicken()

	catalog := text(h.Get(t, "/catalog").Body)
	if strings.Contains(catalog, "Out of stock") || strings.Count(catalog, "Fileni Petto di Pollo a Fette 320 g 320 g") != 1 {
		t.Errorf("seeding again did not refresh the one Catalog Product:\n%s", catalog)
	}
	if got := mappingShown(t, h, "chicken breast"); !strings.HasPrefix(got, pettoDiPollo.Name+" Change") {
		t.Errorf("chicken breast shows %q after seeding again, want its mapping kept", got)
	}
}

func TestProductSoldByWeightIsSavedByWeight(t *testing.T) {
	h, llm := startCatalog(t, orderedStore(), time.Millisecond)
	llm.Script(
		fakellm.CallTool("look_up_product", map[string]any{"code": banane.Code}),
		fakellm.CallTool("save_catalog_product", map[string]any{"code": banane.Code, "pack_quantity": 1, "unit": "piece"}),
		fakellm.Reply("Could not save the bananas."),
	)
	run := text(startRun(t, h, "seed").Body)
	if !strings.Contains(run, "Esselunga Bio banane is sold by weight: save it in g") {
		t.Errorf("a product sold by weight was not refused in pieces:\n%s", run)
	}
	if catalog := text(h.Get(t, "/catalog").Body); !strings.Contains(catalog, "No Catalog Products yet.") {
		t.Errorf("bananas were saved by the piece, at their price per kg:\n%s", catalog)
	}
}

// A Product Mapping is changed in place, so the user keeps their place in
// a long list of Ingredients. See ADR 0009.
func TestProductMappingChangesInPlaceUnderHTMX(t *testing.T) {
	h, _ := seededWithIngredients(t)
	form := mappingForm(t, h, "basmati rice")

	form.Values.Set("catalog_product", productID(t, h, lenticchie.Name))
	res := h.HTMXPostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusOK || res.Method != http.MethodPost || !isRow(res.Body) {
		t.Fatalf("HTMX set mapping = %d %s, want 200 and its row alone:\n%s", res.StatusCode, res.Method, res.Body)
	}
	if row := text(res.Body); !strings.HasPrefix(row, "basmati rice "+lenticchie.Name+" Change") {
		t.Errorf("row does not show the new mapping: %s", row)
	}

	form.Values.Set("catalog_product", productID(t, h, latte.Name))
	res = h.HTMXPostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusUnprocessableEntity || !isRow(res.Body) {
		t.Fatalf("HTMX mapping to a product in ml = %d, want 422 and its row alone:\n%s", res.StatusCode, res.Body)
	}
	if row := text(res.Body); !strings.HasPrefix(row, "basmati rice "+lenticchie.Name) || !strings.Contains(row, "is sold in ml") {
		t.Errorf("rejected row does not show the mapping kept and why: %s", row)
	}
}

func TestProductMappingFormsSwapOnlyTheirRow(t *testing.T) {
	h, _ := seededWithIngredients(t)
	page := h.Get(t, "/catalog").Body
	form := mappingForm(t, h, "chicken breast")
	id := strings.TrimPrefix(form.Action, "/product-mappings/")
	i := strings.Index(page, `id="mapping-`+id+`"`)
	tag := page[i : i+strings.Index(page[i:], ">")]
	for _, want := range []string{`method="post"`, `hx-post="` + form.Action + `"`, `hx-target="closest li"`, `hx-swap="outerHTML"`} {
		if !strings.Contains(tag, want) {
			t.Errorf("mapping form lacks %s: %s", want, tag)
		}
	}
}
