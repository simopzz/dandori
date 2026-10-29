package planning

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

// Groceries is a Cycle's Grocery List.
type Groceries struct {
	Cycle Cycle
	List  domain.GroceryList
	// HasPlan is whether the Cycle has a Plan to buy for.
	HasPlan bool
	// Fixed is whether the list no longer follows the Plan, stock and
	// Product Mappings: its Delivery was ordered, or has arrived.
	Fixed bool
}

// GroceryList loads the Grocery List of the Cycle with id: what its Plan
// needs beyond stock, rounded up to whole packs, until its Delivery is
// ordered or arrives, and what was fixed then after.
func (s *Service) GroceryList(ctx context.Context, id int64) (Groceries, error) {
	var g Groceries
	if err := s.Arrive(ctx); err != nil {
		return g, err
	}
	return g, pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		c, err := findCycle(ctx, tx, id)
		if err != nil {
			return err
		}
		if c == nil {
			return ErrNoCycle
		}
		g.Cycle = *c
		if g.HasPlan, err = hasPlan(ctx, tx, id); err != nil {
			return err
		}
		g.List, g.Fixed, err = s.groceryList(ctx, tx, *c)
		return err
	})
}

// groceryList is c's Grocery List, and whether it is fixed: as stored once
// fixed, and computed from its Plan, the Stock Items and the Product
// Mappings until then.
func (s *Service) groceryList(ctx context.Context, q db.Querier, c Cycle) (domain.GroceryList, bool, error) {
	fixed, err := groceryListFixed(ctx, q, c.ID)
	if err != nil {
		return nil, false, err
	}
	if fixed {
		list, err := loadGroceryList(ctx, q, c.ID)
		return list, true, err
	}
	plan, _, err := loadPlan(ctx, q, c.ID)
	if err != nil {
		return nil, false, err
	}
	stock, err := s.stock.ItemsIn(ctx, q)
	if err != nil {
		return nil, false, err
	}
	mappings, err := s.catalog.ProductMappings(ctx, q)
	if err != nil {
		return nil, false, err
	}
	return domain.NewGroceryList(plan, stock, mappings, c.Delivery.On), false, nil
}

// fixGroceryList stores c's Grocery List as it stands, unless it is fixed
// already, and returns it.
func (s *Service) fixGroceryList(ctx context.Context, q db.Querier, c Cycle) (domain.GroceryList, error) {
	list, fixed, err := s.groceryList(ctx, q, c)
	if err != nil || fixed {
		return list, err
	}
	return list, storeGroceryList(ctx, q, c.ID, list)
}

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
	case !g.HasPlan && !g.Fixed:
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
