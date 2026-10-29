package planning

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/httpx"
)

func groceriesURL(id int64) string { return fmt.Sprintf("/cycles/%d/groceries", id) }

// ShowGroceries shows a Cycle's Grocery List page.
func (h Handlers) ShowGroceries(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	g, err := h.Service.GroceryList(r.Context(), id)
	switch {
	case errors.Is(err, ErrNoCycle):
		http.NotFound(w, r)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "load grocery list", err)
		return
	}
	page := GroceriesPage{Title: "For the Cycle delivered " + g.Cycle.Delivery.On.Format(longDate)}
	switch {
	case g.Fixed && g.Cycle.Status == "upcoming":
		page.FixedBecause = "Fixed when its Delivery was marked ordered."
	case g.Fixed:
		page.FixedBecause = "Its Delivery has arrived: everything on it became Stock Items."
	}
	switch {
	case len(g.List) > 0:
	case !g.HasPlan && g.Fixed:
		page.EmptyBecause = "Its Delivery arrived before a Plan was approved."
	case !g.HasPlan:
		page.EmptyBecause = "No Plan yet: approve a Plan Proposal, and what it needs beyond stock shows here."
	default:
		page.EmptyBecause = "Stock covers everything the Plan needs."
	}
	for _, a := range g.List.ByAisle() {
		aisle := GroceryAisle{Title: a.Aisle, Unmapped: a.Unmapped}
		switch {
		case a.Unmapped:
			aisle.Title = "No Product Mapping"
		case a.Aisle == "":
			aisle.Title = "Other aisles"
		}
		for _, l := range a.Lines {
			unit := " " + string(l.Ingredient.Unit)
			row := GroceryRow{Ingredient: l.Ingredient.Name, Needed: formatQuantity(l.Needed) + unit + " needed"}
			if m := l.Mapping; m != nil {
				row.Bought = formatQuantity(l.Bought()) + unit + " bought"
				row.Packs = fmt.Sprintf("%d × %s", l.Packs, m.Product.Name)
				row.Proposed = m.Proposed
				page.AnyProposed = page.AnyProposed || m.Proposed
			}
			aisle.Lines = append(aisle.Lines, row)
		}
		page.Aisles = append(page.Aisles, aisle)
	}
	templ.Handler(GroceriesView(page)).ServeHTTP(w, r)
}

// formatQuantity is a quantity to one decimal, without trailing zeros: a
// Recipe scaled to its Portions can need 166.666 g.
func formatQuantity(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64) }
