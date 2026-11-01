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

// cartFillURL is where Cart Fill is started for a Cycle.
func cartFillURL(id int64) string { return fmt.Sprintf("/cycles/%d/cart-fill", id) }

// ShowGroceries shows a Cycle's Grocery List page.
func (h Handlers) ShowGroceries(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	h.renderGroceries(w, r, id, "", http.StatusOK)
}

// renderGroceries shows the Grocery List page of the Cycle with id, with
// problem, why what the user asked for didn't happen, if it didn't.
func (h Handlers) renderGroceries(w http.ResponseWriter, r *http.Request, id int64, problem string, status int) {
	g, err := h.Service.GroceryList(r.Context(), id)
	switch {
	case errors.Is(err, ErrNoCycle):
		http.NotFound(w, r)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "load grocery list", err)
		return
	}
	page := GroceriesPage{Title: "For the Cycle delivered " + g.Cycle.Delivery.On.Format(longDate), Problem: problem}
	if g.CartFill.CanFill {
		page.FillAction = cartFillURL(id)
	}
	if run := g.CartFill.Run; run != nil {
		page.FillRun = runURL(run.ID)
		page.FillRunning = run.Running()
		page.FillSummary = run.Notes()
		page.FillFailed = run.Failure()
	}
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
	templ.Handler(GroceriesView(page), templ.WithStatus(status)).ServeHTTP(w, r)
}

// FillCart starts Cart Fill for a Cycle, and sends the user to its Run.
func (h Handlers) FillCart(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	run, err := h.Service.StartCartFill(r.Context(), id)
	var cannot *CannotFillError
	switch {
	case errors.Is(err, ErrNoCycle):
		http.NotFound(w, r)
	case errors.As(err, &cannot) && cannot.RunID != 0:
		// Started twice, as by a double tap: follow the Run already going.
		http.Redirect(w, r, runURL(cannot.RunID), http.StatusSeeOther)
	case errors.As(err, &cannot):
		h.renderGroceries(w, r, id, "The cart can't be filled: "+cannot.Reason+".", http.StatusConflict)
	case err != nil:
		httpx.ServerError(w, h.Log, "start cart fill", err)
	default:
		http.Redirect(w, r, runURL(run), http.StatusSeeOther)
	}
}

// formatQuantity is a quantity to one decimal, without trailing zeros: a
// Recipe scaled to its Portions can need 166.666 g.
func formatQuantity(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64) }
