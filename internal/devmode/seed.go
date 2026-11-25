package devmode

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
)

// seedIngredient is an Ingredient the seed adds, counted to `stated` in its usual Storage
// Location unless `stated` is "".
type seedIngredient struct {
	name, usual, kcal, protein, carbs, fat, stated string
}

// The seed's kitchen, in whole numbers, which every Language's forms read the same.
var (
	seedIngredients = []seedIngredient{
		{"rice", "pantry", "350", "7", "78", "1", "800"},
		{"oats", "pantry", "380", "13", "60", "7", "500"},
		{"chicken breast", "fridge", "120", "23", "0", "3", ""},
		{"broccoli", "fridge", "34", "3", "7", "0", ""},
		{"yogurt", "fridge", "60", "10", "4", "0", ""},
	}
	seedRecipes = []url.Values{
		recipe("Chicken rice bowl", []string{"lunch", "dinner"}, []string{"chicken breast", "300", "rice", "160", "broccoli", "200"},
			"Cook the rice.", "Grill the chicken.", "Steam the broccoli."),
		recipe("Overnight oats", []string{"breakfast"}, []string{"oats", "100", "yogurt", "300"},
			"Mix the oats into the yogurt and leave them in the fridge overnight."),
	}
)

// recipe is the Recipe form for two Portions of `name`, eaten at `slots`, of `rows`, each
// an Ingredient's name then its quantity, made with `steps` on the hob.
func recipe(name string, slots, rows []string, steps ...string) url.Values {
	f := url.Values{
		"name": {name}, "portions": {"2"}, "active": {"15"}, "total": {"30"},
		"slots": slots, "appliances": {"hob"}, "steps": steps,
	}
	for i := 0; i < len(rows); i += 2 {
		f.Set(fmt.Sprintf("ing_name_%d", i/2), rows[i])
		f.Set(fmt.Sprintf("ing_quantity_%d", i/2), rows[i+1])
	}
	return f
}

// Seed gives the signed-in User a kitchen ready to plan: Targets, appliances, the
// Ingredients above with rice and oats counted, and two Recipes. It posts the app's own
// forms to `next`, as the User, so what it writes is what they would have typed. It
// answers 204, 409 before the User has signed in and chosen a Language, and 422 naming
// the first post the app refused, such as an Ingredient they already have.
func Seed(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		send := func(method, path string, form url.Values) *httptest.ResponseRecorder {
			req := httptest.NewRequestWithContext(r.Context(), method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for _, c := range r.Cookies() {
				req.AddCookie(c)
			}
			rec := httptest.NewRecorder()
			next.ServeHTTP(rec, req)
			return rec
		}
		if send(http.MethodGet, "/profile", nil).Code != http.StatusOK {
			http.Error(w, "seed: sign in and choose a Language first", http.StatusConflict)
			return
		}
		// A post the app takes answers a plain browser with a redirect.
		post := func(path string, form url.Values) (*httptest.ResponseRecorder, bool) {
			rec := send(http.MethodPost, path, form)
			if rec.Code != http.StatusSeeOther {
				http.Error(w, fmt.Sprintf("seed: POST %s answered %d", path, rec.Code), http.StatusUnprocessableEntity)
				return rec, false
			}
			return rec, true
		}
		if _, ok := post("/profile/targets", url.Values{"kcal": {"2200"}, "protein": {"130"}, "fibre": {"30"}, "sugar": {"50"}, "tolerance": {"8"}}); !ok {
			return
		}
		if _, ok := post("/profile/appliances", url.Values{"appliances": {"hob", "oven", "microwave"}}); !ok {
			return
		}
		for _, ing := range seedIngredients {
			added, ok := post("/ingredients", url.Values{
				"name": {ing.name}, "unit": {"g"}, "usual": {ing.usual},
				"kcal": {ing.kcal}, "protein": {ing.protein}, "carbs": {ing.carbs}, "fat": {ing.fat}, "counting": {"1"},
			})
			if !ok {
				return
			}
			if ing.stated == "" {
				continue
			}
			// Added to count, it redirects to its Count, which names its id.
			to, err := url.Parse(added.Header().Get("Location"))
			if err != nil {
				http.Error(w, "seed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			count := url.Values{"ingredient": {to.Query().Get("ingredient")}, "location": {ing.usual}, "stated": {ing.stated}}
			if _, ok := post("/kitchen/count", count); !ok {
				return
			}
		}
		for _, f := range seedRecipes {
			if _, ok := post("/recipes", f); !ok {
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
