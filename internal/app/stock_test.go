package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// startStock starts the app with the Ingredients of pastedRecipe known:
// chicken breast and basmati rice in g, olive oil in ml, lemon in pieces.
func startStock(t *testing.T) *apptest.Harness {
	t.Helper()
	h, llm := startRecipes(t)
	acceptFirstRecipe(t, h, llm)
	return h
}

// addStockItem records a Stock Item through the Stock page's form.
func addStockItem(t *testing.T, h *apptest.Harness, ingredient, quantity, location, expires string) apptest.Response {
	t.Helper()
	form := apptest.Form(t, h.Get(t, "/stock").Body, "add-stock-item")
	form.Values.Set("ingredient", ingredient)
	form.Values.Set("quantity", quantity)
	form.Values.Set("location", location)
	form.Values.Set("expires_on", expires)
	return h.PostForm(t, form.Action, form.Values)
}

func TestStockItemsAreRecordedWithAnOptionalExpiryDate(t *testing.T) {
	h := startStock(t)

	res := addStockItem(t, h, "chicken breast", "450", "fridge", "2026-11-02")
	if res.StatusCode != http.StatusOK || res.Path != "/stock" || res.Method != http.MethodGet {
		t.Fatalf("add Stock Item = %d %s %s, want the Stock page after a redirect:\n%s", res.StatusCode, res.Method, res.Path, text(res.Body))
	}
	addStockItem(t, h, "basmati rice", "1000", "pantry", "")

	stock := text(h.Get(t, "/stock").Body)
	for _, want := range []string{
		"chicken breast 450 g fridge expires 2026-11-02",
		"basmati rice 1000 g pantry",
	} {
		if !strings.Contains(stock, want) {
			t.Errorf("Stock page does not show %q:\n%s", want, stock)
		}
	}
	if strings.Contains(stock, "basmati rice 1000 g pantry expires") {
		t.Errorf("a Stock Item without an expiry date shows one:\n%s", stock)
	}
}

func TestInvalidStockItemIsRejectedAndNothingIsRecorded(t *testing.T) {
	tests := []struct {
		name, field, value, message string
	}{
		{"missing quantity", "quantity", "", "Required"},
		{"quantity not a number", "quantity", "lots", "Must be a number"},
		{"zero quantity", "quantity", "0", "Must be more than 0"},
		{"infinite quantity", "quantity", "Inf", "Must be a number"},
		{"unknown Storage Location", "location", "garage", "Must be pantry, fridge or freezer"},
		{"expiry not a date", "expires_on", "next week", "Must be a date"},
		{"Ingredient the Recipe Library doesn't know", "ingredient", "saffron", "Not a known Ingredient"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := startStock(t)
			form := apptest.Form(t, h.Get(t, "/stock").Body, "add-stock-item")
			form.Values.Set("ingredient", "lemon")
			form.Values.Set("quantity", "3")
			form.Values.Set("location", "fridge")
			form.Values.Set(tt.field, tt.value)

			res := h.PostForm(t, form.Action, form.Values)
			if res.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("add Stock Item = %d, want 422:\n%s", res.StatusCode, text(res.Body))
			}
			if !strings.Contains(text(res.Body), tt.message) {
				t.Errorf("rejection does not say %q:\n%s", tt.message, text(res.Body))
			}
			if tt.field != "ingredient" && tt.field != "location" && !hasField(res.Body, tt.field, tt.value) {
				t.Errorf("rejected form does not keep what was typed:\n%s", res.Body)
			}
			if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "No Stock Items yet.") {
				t.Errorf("a rejected Stock Item was recorded:\n%s", stock)
			}
		})
	}
}

// stockItemID is the id of the only Stock Item on the Stock page.
func stockItemID(t *testing.T, h *apptest.Harness) string {
	t.Helper()
	page := h.Get(t, "/stock").Body
	i := strings.Index(page, `id="stock-item-`)
	if i < 0 {
		t.Fatalf("the Stock page lists no Stock Item:\n%s", page)
	}
	rest := page[i+len(`id="stock-item-`):]
	return rest[:strings.Index(rest, `"`)]
}

func TestStockItemIsAdjustedAndMoved(t *testing.T) {
	h := startStock(t)
	addStockItem(t, h, "chicken breast", "600", "fridge", "2026-11-02")
	id := stockItemID(t, h)

	form := apptest.Form(t, h.Get(t, "/stock").Body, "stock-item-"+id)
	if form.Get("quantity") != "600" || form.Get("location") != "fridge" || form.Get("expires_on") != "2026-11-02" {
		t.Fatalf("change form does not show the Stock Item: %v", form.Values)
	}
	// Half was cooked; the rest goes into the freezer, where it keeps longer.
	form.Values.Set("quantity", "300")
	form.Values.Set("location", "freezer")
	form.Values.Set("expires_on", "2027-01-15")
	res := h.PostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusOK || res.Path != "/stock" || res.Method != http.MethodGet {
		t.Fatalf("change Stock Item = %d %s %s, want the Stock page after a redirect:\n%s", res.StatusCode, res.Method, res.Path, text(res.Body))
	}
	if stock := text(res.Body); !strings.Contains(stock, "chicken breast 300 g freezer expires 2027-01-15") {
		t.Errorf("Stock page does not show the adjusted and moved Stock Item:\n%s", stock)
	}

	// Clearing the expiry date removes it.
	form.Values.Set("expires_on", "")
	if stock := text(h.PostForm(t, form.Action, form.Values).Body); !strings.Contains(stock, "chicken breast 300 g freezer") || strings.Contains(stock, "expires") {
		t.Errorf("Stock Item keeps a cleared expiry date:\n%s", stock)
	}

	form.Values.Set("quantity", "-5")
	res = h.PostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Must be more than 0") {
		t.Fatalf("invalid change = %d, want 422 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	if !hasField(res.Body, "quantity", "-5") {
		t.Errorf("rejected change form does not keep what was typed:\n%s", res.Body)
	}
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "chicken breast 300 g freezer") {
		t.Errorf("an invalid change altered the Stock Item:\n%s", stock)
	}
}

func TestStockItemIsRemoved(t *testing.T) {
	h := startStock(t)
	addStockItem(t, h, "lemon", "2", "fridge", "")
	id := stockItemID(t, h)

	remove := apptest.Form(t, h.Get(t, "/stock").Body, "remove-stock-item-"+id)
	res := h.PostForm(t, remove.Action, remove.Values)
	if res.StatusCode != http.StatusOK || res.Path != "/stock" {
		t.Fatalf("remove Stock Item = %d at %s, want the Stock page:\n%s", res.StatusCode, res.Path, text(res.Body))
	}
	if stock := text(res.Body); !strings.Contains(stock, "No Stock Items yet.") {
		t.Errorf("a removed Stock Item is still listed:\n%s", stock)
	}
}

func TestChangingAStockItemThatIsGoneIsNotFound(t *testing.T) {
	h := startStock(t)
	addStockItem(t, h, "lemon", "2", "fridge", "")
	id := stockItemID(t, h)
	form := apptest.Form(t, h.Get(t, "/stock").Body, "stock-item-"+id)
	h.PostForm(t, "/stock-items/"+id+"/delete", nil)

	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusNotFound {
		t.Errorf("change a removed Stock Item = %d, want 404", res.StatusCode)
	}
	form.Values.Set("quantity", "")
	if res := h.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusNotFound {
		t.Errorf("invalid change to a removed Stock Item = %d, want 404", res.StatusCode)
	}
}

// seedPortions adds a Prep Session of the Lemon chicken with rice Recipe
// whose two Portions have no Slot: one in the fridge, one in the freezer.
// Prep Sessions are only made by planning, which comes later.
func seedPortions(t *testing.T, h *apptest.Harness) {
	t.Helper()
	h.Exec(t, `
		WITH session AS (
			INSERT INTO prep_session (recipe_id)
			SELECT id FROM recipe WHERE name = 'Lemon chicken with rice' RETURNING id
		)
		INSERT INTO portion (prep_session_id, location)
		SELECT id, l FROM session, unnest(ARRAY['fridge', 'freezer']) l`)
}

// portionIDs are the ids of the Portions on the Stock page, in order.
func portionIDs(t *testing.T, h *apptest.Harness) []string {
	t.Helper()
	var ids []string
	page := h.Get(t, "/stock").Body
	for {
		i := strings.Index(page, `id="portion-`)
		if i < 0 {
			return ids
		}
		page = page[i+len(`id="portion-`):]
		ids = append(ids, page[:strings.Index(page, `"`)])
	}
}

func TestPortionsWithoutASlotAreListedWithTheirStorageLocation(t *testing.T) {
	h := startStock(t)
	if stock := text(h.Get(t, "/stock").Body); !strings.Contains(stock, "No Portions without a Slot.") {
		t.Errorf("Stock page does not say there are no Portions:\n%s", stock)
	}
	seedPortions(t, h)

	stock := text(h.Get(t, "/stock").Body)
	for _, want := range []string{"Lemon chicken with rice fridge", "Lemon chicken with rice freezer"} {
		if !strings.Contains(stock, want) {
			t.Errorf("Stock page does not show the Portion %q:\n%s", want, stock)
		}
	}
}

func TestPortionIsMovedToAnotherStorageLocation(t *testing.T) {
	h := startStock(t)
	seedPortions(t, h)
	ids := portionIDs(t, h)
	if len(ids) != 2 {
		t.Fatalf("Stock page lists %d Portions, want 2", len(ids))
	}

	// The fridge one won't be eaten in time: freeze it too.
	move := apptest.Form(t, h.Get(t, "/stock").Body, "portion-"+ids[0])
	if move.Get("location") != "fridge" {
		t.Fatalf("move form does not show the Portion's Storage Location: %v", move.Values)
	}
	move.Values.Set("location", "freezer")
	res := h.PostForm(t, move.Action, move.Values)
	if res.StatusCode != http.StatusOK || res.Path != "/stock" || res.Method != http.MethodGet {
		t.Fatalf("move Portion = %d %s %s, want the Stock page after a redirect:\n%s", res.StatusCode, res.Method, res.Path, text(res.Body))
	}
	stock := text(res.Body)
	if strings.Contains(stock, "Lemon chicken with rice fridge") || strings.Count(stock, "Lemon chicken with rice freezer") != 2 {
		t.Errorf("Stock page does not show both Portions in the freezer:\n%s", stock)
	}

	move.Values.Set("location", "garage")
	if res := h.PostForm(t, move.Action, move.Values); res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Must be pantry, fridge or freezer") {
		t.Errorf("move to an unknown Storage Location = %d, want 422 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	move.Values.Set("location", "fridge")
	if res := h.PostForm(t, "/portions/999999/move", move.Values); res.StatusCode != http.StatusNotFound {
		t.Errorf("move a Portion that doesn't exist = %d, want 404", res.StatusCode)
	}
	move.Values.Set("location", "garage")
	if res := h.PostForm(t, "/portions/999999/move", move.Values); res.StatusCode != http.StatusNotFound {
		t.Errorf("invalid move of a Portion that doesn't exist = %d, want 404", res.StatusCode)
	}
}

func TestStockIsLinkedFromTheAppShell(t *testing.T) {
	h := apptest.Start(t)

	if page := h.Get(t, "/").Body; !strings.Contains(page, `href="/stock"`) {
		t.Errorf("app shell does not link to the Stock page:\n%s", page)
	}
}

func TestRecipeWithPortionsIsNotDeleted(t *testing.T) {
	h := startStock(t)
	seedPortions(t, h)
	recipe := libraryRecipe(t, h)

	res := h.PostForm(t, recipe+"/delete", nil)
	if res.StatusCode != http.StatusConflict || !strings.Contains(text(res.Body), "Portions of this Recipe are still kept") {
		t.Fatalf("delete a Recipe with Portions = %d, want 409 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	if got := h.Get(t, recipe).StatusCode; got != http.StatusOK {
		t.Errorf("GET the Recipe after a refused delete = %d, want 200", got)
	}
	if stock := text(h.Get(t, "/stock").Body); strings.Count(stock, "Lemon chicken with rice") != 2 {
		t.Errorf("Stock page does not show both Portions of the Recipe after a refused delete:\n%s", stock)
	}
}

// isRow reports whether body is a single row of a list, swapped in on its
// own, and not a whole page.
func isRow(body string) bool {
	body = strings.TrimSpace(body)
	return strings.HasPrefix(body, "<li") && strings.HasSuffix(body, "</li>")
}

// A Stock Item's row is changed in place, so the user keeps their place in
// a long Stock page. See ADR 0009.
func TestStockItemChangesInPlaceUnderHTMX(t *testing.T) {
	h := startStock(t)
	addStockItem(t, h, "chicken breast", "600", "fridge", "2026-11-02")
	id := stockItemID(t, h)
	form := apptest.Form(t, h.Get(t, "/stock").Body, "stock-item-"+id)

	form.Values.Set("quantity", "300")
	form.Values.Set("location", "freezer")
	res := h.HTMXPostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusOK || res.Method != http.MethodPost || !isRow(res.Body) {
		t.Fatalf("HTMX change Stock Item = %d %s, want 200 and its row alone:\n%s", res.StatusCode, res.Method, res.Body)
	}
	if row := text(res.Body); !strings.Contains(row, "chicken breast 300 g freezer expires 2026-11-02") {
		t.Errorf("row does not show the changed Stock Item:\n%s", row)
	}

	form.Values.Set("quantity", "-5")
	res = h.HTMXPostForm(t, form.Action, form.Values)
	if res.StatusCode != http.StatusUnprocessableEntity || !isRow(res.Body) || !strings.Contains(text(res.Body), "Must be more than 0") {
		t.Fatalf("HTMX invalid change = %d, want 422 and its row saying why:\n%s", res.StatusCode, res.Body)
	}
	if !hasField(res.Body, "quantity", "-5") || !strings.Contains(text(res.Body), "chicken breast 300 g freezer") {
		t.Errorf("rejected row does not show the Stock Item and what was typed:\n%s", res.Body)
	}
}

func TestStockItemIsRemovedInPlaceUnderHTMX(t *testing.T) {
	h := startStock(t)
	addStockItem(t, h, "lemon", "2", "fridge", "")
	addStockItem(t, h, "olive oil", "500", "pantry", "")
	page := h.Get(t, "/stock").Body
	lemon := apptest.Form(t, page, "remove-stock-item-"+stockItemID(t, h))

	res := h.HTMXPostForm(t, lemon.Action, lemon.Values)
	if res.StatusCode != http.StatusOK || res.Method != http.MethodPost || strings.TrimSpace(res.Body) != "" {
		t.Fatalf("HTMX remove Stock Item = %d %s, want 200 and nothing, taking its row away:\n%s", res.StatusCode, res.Method, res.Body)
	}
	if stock := text(h.Get(t, "/stock").Body); strings.Contains(stock, "lemon 2") || !strings.Contains(stock, "olive oil 500 ml") {
		t.Errorf("Stock page does not show only the Stock Item left:\n%s", stock)
	}

	// The last one gone, the list's place says there are none.
	oil := apptest.Form(t, h.Get(t, "/stock").Body, "remove-stock-item-"+stockItemID(t, h))
	res = h.HTMXPostForm(t, oil.Action, oil.Values)
	if res.StatusCode != http.StatusOK || res.Header.Get("HX-Retarget") != "closest ul" || !strings.Contains(text(res.Body), "No Stock Items yet.") {
		t.Errorf("HTMX remove of the last Stock Item = %d, HX-Retarget %q, want the list swapped for saying there are none:\n%s", res.StatusCode, res.Header.Get("HX-Retarget"), res.Body)
	}
}

func TestPortionIsMovedInPlaceUnderHTMX(t *testing.T) {
	h := startStock(t)
	seedPortions(t, h)
	move := apptest.Form(t, h.Get(t, "/stock").Body, "portion-"+portionIDs(t, h)[0])

	move.Values.Set("location", "freezer")
	res := h.HTMXPostForm(t, move.Action, move.Values)
	if res.StatusCode != http.StatusOK || res.Method != http.MethodPost || !isRow(res.Body) {
		t.Fatalf("HTMX move Portion = %d %s, want 200 and its row alone:\n%s", res.StatusCode, res.Method, res.Body)
	}
	if row := text(res.Body); !strings.Contains(row, "Lemon chicken with rice freezer") {
		t.Errorf("row does not show the Portion in the freezer:\n%s", row)
	}

	move.Values.Set("location", "garage")
	res = h.HTMXPostForm(t, move.Action, move.Values)
	if res.StatusCode != http.StatusUnprocessableEntity || !isRow(res.Body) || !strings.Contains(text(res.Body), "Must be pantry, fridge or freezer") {
		t.Errorf("HTMX move to an unknown Storage Location = %d, want 422 and its row saying why:\n%s", res.StatusCode, res.Body)
	}
}

// A row whose Stock Item or Portion is gone, say removed in another tab,
// is taken away.
func TestChangingARowThatIsGoneUnderHTMXTakesItAway(t *testing.T) {
	h := startStock(t)
	addStockItem(t, h, "lemon", "2", "fridge", "")
	id := stockItemID(t, h)
	form := apptest.Form(t, h.Get(t, "/stock").Body, "stock-item-"+id)
	h.PostForm(t, "/stock-items/"+id+"/delete", nil)

	if res := h.HTMXPostForm(t, form.Action, form.Values); res.StatusCode != http.StatusNotFound || res.Body != "" {
		t.Errorf("HTMX change a removed Stock Item = %d, want 404 and nothing:\n%s", res.StatusCode, res.Body)
	}
	form.Values.Set("quantity", "")
	if res := h.HTMXPostForm(t, form.Action, form.Values); res.StatusCode != http.StatusNotFound || res.Body != "" {
		t.Errorf("HTMX invalid change to a removed Stock Item = %d, want 404 and nothing:\n%s", res.StatusCode, res.Body)
	}
	move := url.Values{"location": {"fridge"}}
	if res := h.HTMXPostForm(t, "/portions/999999/move", move); res.StatusCode != http.StatusNotFound || res.Body != "" {
		t.Errorf("HTMX move a Portion that doesn't exist = %d, want 404 and nothing:\n%s", res.StatusCode, res.Body)
	}
	move.Set("location", "garage")
	if res := h.HTMXPostForm(t, "/portions/999999/move", move); res.StatusCode != http.StatusNotFound || res.Body != "" {
		t.Errorf("HTMX invalid move of a Portion that doesn't exist = %d, want 404 and nothing:\n%s", res.StatusCode, res.Body)
	}
}

// Every row's form swaps only its row under htmx and still posts to its
// action without it.
func TestStockRowFormsSwapOnlyTheirRow(t *testing.T) {
	h := startStock(t)
	addStockItem(t, h, "lemon", "2", "fridge", "")
	seedPortions(t, h)
	page := h.Get(t, "/stock").Body

	for _, id := range []string{"stock-item-" + stockItemID(t, h), "remove-stock-item-" + stockItemID(t, h), "portion-" + portionIDs(t, h)[0]} {
		form := apptest.Form(t, page, id)
		i := strings.Index(page, `id="`+id+`"`)
		tag := page[i : i+strings.Index(page[i:], ">")]
		for _, want := range []string{`method="post"`, `hx-post="` + form.Action + `"`, `hx-target="closest li"`, `hx-swap="outerHTML"`} {
			if !strings.Contains(tag, want) {
				t.Errorf("form %s lacks %s: %s", id, want, tag)
			}
		}
	}
}
