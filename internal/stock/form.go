package stock

import (
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/simopzz/dandori/internal/domain"
)

// formFields maps domain.StockItem field names to form field names.
var formFields = map[string]string{"Quantity": "quantity", "Location": "location"}

// readStockItem reads a Stock Item form, returning the Stock Item and what
// is wrong with it keyed by form field name. The add form also names the
// Ingredient; a change form keeps the Stock Item's own.
func readStockItem(form url.Values) (domain.StockItem, map[string]string) {
	get := func(name string) string { return strings.TrimSpace(form.Get(name)) }
	problems := map[string]string{}
	item := domain.StockItem{
		Ingredient: domain.Ingredient{Name: domain.IngredientName(get("ingredient"))},
		Location:   domain.StorageLocation(get("location")),
	}

	if raw := get("quantity"); raw == "" {
		problems["quantity"] = "Required"
	} else if v, err := strconv.ParseFloat(strings.Replace(raw, ",", ".", 1), 64); err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		problems["quantity"] = "Must be a number"
	} else {
		item.Quantity = v
	}

	for field, msg := range item.Validate() {
		if name := formFields[field]; problems[name] == "" {
			problems[name] = msg
		}
	}
	return item, problems
}

// typed is what was submitted, so a rejected form is shown back as typed.
func typed(form url.Values) map[string]string {
	values := map[string]string{}
	for name := range form {
		values[name] = form.Get(name)
	}
	return values
}

func itemValues(it Item) map[string]string {
	return map[string]string{
		"quantity": formatAmount(it.Quantity),
		"location": string(it.Location),
	}
}

func formatAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
