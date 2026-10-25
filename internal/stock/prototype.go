// PROTOTYPE (design system): throwaway, lives only on prototype/design-system.

package stock

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

var protoVariants = []web.ProtoVariant{
	{Key: "D", Name: "Ma — Japanese-minimal"},
	{Key: "D2", Name: "Ma — on white paper"},
	{Key: "A", Name: "Editorial — warm"},
	{Key: "B", Name: "Ledger — calm utility"},
	{Key: "E", Name: "Block — bold modern"},
	{Key: "now", Name: "Current page"},
}

type protoItem struct {
	ID       int64
	Name     string
	Quantity string
	Location string
	Expires  string // the date as typed, empty when none
	DaysLeft int    // meaningful only when Expires is set
}

type protoPortion struct {
	ID       int64
	Recipe   string
	Location string
}

type protoPage struct {
	Today       string
	Items       []protoItem
	Portions    []protoPortion
	Ingredients []IngredientOption
	Fixtures    bool // the data shown is made up, the real Stock being too thin
}

func (p protoPage) at(loc string) []protoItem {
	var out []protoItem
	for _, it := range p.Items {
		if it.Location == loc {
			out = append(out, it)
		}
	}
	return out
}

func (p protoPage) portionsAt(loc string) []protoPortion {
	var out []protoPortion
	for _, po := range p.Portions {
		if po.Location == loc {
			out = append(out, po)
		}
	}
	return out
}

// soon is every Stock Item expiring within three days, or already expired.
func (p protoPage) soon() []protoItem {
	var out []protoItem
	for _, it := range p.Items {
		if it.Expires != "" && it.DaysLeft <= 3 {
			out = append(out, it)
		}
	}
	return out
}

func (it protoItem) urgent() bool { return it.Expires != "" && it.DaysLeft <= 3 }

func (it protoItem) when() string {
	switch {
	case it.Expires == "":
		return ""
	case it.DaysLeft < 0:
		return "expired"
	case it.DaysLeft == 0:
		return "today"
	case it.DaysLeft == 1:
		return "tomorrow"
	case it.DaysLeft <= 14:
		return fmt.Sprintf("in %d days", it.DaysLeft)
	}
	return it.Expires
}

var locations = []string{"pantry", "fridge", "freezer"}

func locIcon(loc string) string {
	return map[string]string{"pantry": "package", "fridge": "refrigerator", "freezer": "snowflake"}[loc]
}

func title(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

func (h Handlers) showPrototype(w http.ResponseWriter, r *http.Request, variant string) {
	if variant == "now" {
		h.render(w, r, rejection{}, http.StatusOK)
		web.ProtoSwitcher(protoVariants, "now").Render(r.Context(), w)
		return
	}
	p := h.protoData(r)
	var c templ.Component
	switch variant {
	case "D2":
		c = variantD(p)
	case "A":
		c = variantA(p)
	case "B":
		c = variantB(p)
	case "E":
		c = variantE(p)
	default:
		variant, c = "D", variantD(p)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	web.ProtoDoc("Dandori — Stock (prototype "+variant+")", "/static/prototype/"+variant+".css", protoVariants, variant).Render(templ.WithChildren(r.Context(), c), w)
}

// protoData is the real Stock, or fixtures when it is too thin to judge a
// design by (or the database is unreachable).
func (h Handlers) protoData(r *http.Request) protoPage {
	today := time.Now()
	p := protoPage{Today: today.Format("Monday 2 January")}
	items, err1 := h.Service.Items(r.Context())
	portions, err2 := h.Service.Portions(r.Context())
	ings, err3 := h.Service.Ingredients(r.Context())
	if err1 == nil && err2 == nil && err3 == nil && len(items) >= 6 {
		for _, it := range items {
			pi := protoItem{ID: it.ID, Name: it.Ingredient.Name, Quantity: formatAmount(it.Quantity) + " " + string(it.Ingredient.Unit), Location: string(it.Location)}
			if it.ExpiresOn != nil {
				pi.Expires = it.ExpiresOn.Format(dateLayout)
				pi.DaysLeft = int(it.ExpiresOn.Sub(today.Truncate(24*time.Hour)).Hours() / 24)
			}
			p.Items = append(p.Items, pi)
		}
		for _, po := range portions {
			rec := po.Recipe
			if rec == "" {
				rec = "Ingredients as they are"
			}
			p.Portions = append(p.Portions, protoPortion{ID: po.ID, Recipe: rec, Location: string(po.Location)})
		}
		for _, ing := range ings {
			p.Ingredients = append(p.Ingredients, IngredientOption{Name: ing.Name, Unit: string(ing.Unit)})
		}
		return p
	}
	p.Fixtures = true
	day := func(n int) (string, int) { return today.AddDate(0, 0, n).Format(dateLayout), n }
	item := func(id int64, name, qty string, loc domain.StorageLocation, days int) protoItem {
		it := protoItem{ID: id, Name: name, Quantity: qty, Location: string(loc)}
		if days != 999 {
			it.Expires, it.DaysLeft = day(days)
		}
		return it
	}
	p.Items = []protoItem{
		item(1, "basmati rice", "1200 g", domain.Pantry, 999),
		item(2, "dried chickpeas", "500 g", domain.Pantry, 999),
		item(3, "passata", "700 g", domain.Pantry, 120),
		item(4, "olive oil", "750 ml", domain.Pantry, 999),
		item(5, "rolled oats", "800 g", domain.Pantry, 200),
		item(6, "greek yogurt", "500 g", domain.Fridge, 2),
		item(7, "egg", "6 piece", domain.Fridge, 9),
		item(8, "baby spinach", "200 g", domain.Fridge, 1),
		item(9, "parmigiano reggiano", "300 g", domain.Fridge, 30),
		item(10, "chicken thighs", "900 g", domain.Fridge, 0),
		item(11, "frozen peas", "450 g", domain.Freezer, 999),
		item(12, "salmon fillet", "2 piece", domain.Freezer, 60),
	}
	p.Portions = []protoPortion{
		{1, "Chickpea & spinach curry", "freezer"},
		{2, "Chickpea & spinach curry", "fridge"},
		{3, "Overnight oats with yogurt", "fridge"},
		{4, "Baked chicken thighs with rice", "freezer"},
		{5, "Ingredients as they are", "pantry"},
	}
	for _, it := range p.Items {
		p.Ingredients = append(p.Ingredients, IngredientOption{Name: it.Name, Unit: strings.Fields(it.Quantity)[1]})
	}
	return p
}
