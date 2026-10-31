package planning

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
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
		g.List, g.Fixed, err = s.groceryListOf(ctx, tx, *c)
		return err
	})
}

// groceryListOf is c's Grocery List, and whether it is fixed: as stored once
// fixed, and computed from its Plan, the stock the kitchen will hold when
// its Delivery arrives and the Product Mappings until then.
func (s *Service) groceryListOf(ctx context.Context, q db.Querier, c Cycle) (domain.GroceryList, bool, error) {
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
	stock, err := s.stockAtDelivery(ctx, q, c)
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
	list, fixed, err := s.groceryListOf(ctx, q, c)
	if err != nil || fixed {
		return list, err
	}
	return list, storeGroceryList(ctx, q, c.ID, list)
}

// stockAtDelivery is the stock the kitchen will hold when the Delivery of
// c, the upcoming Cycle, arrives: the Stock Items left once the current
// Cycle's Prep Sessions not yet prepared, and before c starts, have used
// theirs up.
func (s *Service) stockAtDelivery(ctx context.Context, q db.Querier, c Cycle) ([]domain.StockItem, error) {
	stock, err := s.stock.ItemsIn(ctx, q)
	if err != nil {
		return nil, err
	}
	current, err := loadCycleWith(ctx, q, "current")
	if err != nil || current == nil {
		return stock, err
	}
	plan, _, err := loadPlan(ctx, q, current.ID)
	if err != nil {
		return nil, err
	}
	// Arrive has had those whose day has come use up their stock already;
	// c's Delivery drops those from its start on.
	var ahead []domain.PrepSession
	for _, ps := range plan.PrepSessions {
		if ps.At.On.After(s.today()) && ps.At.Before(c.Start) {
			ahead = append(ahead, ps)
		}
	}
	left, _ := domain.UseUp(ahead, stock)
	return left, nil
}
