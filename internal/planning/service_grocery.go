package planning

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

var (
	// ErrProposalPending is marking ordered the Delivery of a Cycle with a Plan
	// Proposal pending.
	ErrProposalPending = errors.New("a Plan Proposal of the Cycle is pending")
	// ErrBeingPlanned is marking ordered the Delivery of a Cycle with a planning
	// Run going.
	ErrBeingPlanned = errors.New("a planning Run of the Cycle is going")
	// ErrArrived is marking ordered, or undoing that, a Delivery that has arrived.
	ErrArrived = errors.New("the Delivery has arrived")
)

// GroceryListOf is the Grocery List of the signed-in User's Cycle `c` at `now`: once its
// Delivery is marked ordered, or has arrived, the lines fixed then; until then, its Plan
// against the kitchen forecast at its first Slot, once the approved Prep Sessions before
// it have used up theirs. The Staples to check are always those its Plan uses. It is
// ErrNoPlan while `c` has no Plan, as a Cycle with no Slot never does.
func (s *Service) GroceryListOf(ctx context.Context, c Cycle, now time.Time) (domain.GroceryList, error) {
	list, _, err := s.groceryList(ctx, c, now)
	return list, err
}

// groceryList is GroceryListOf `c` at `now`, with the Plan it follows.
func (s *Service) groceryList(ctx context.Context, c Cycle, now time.Time) (domain.GroceryList, domain.Plan, error) {
	times, err := s.profile.SlotTimes(ctx)
	if err != nil {
		return domain.GroceryList{}, domain.Plan{}, err
	}
	slots := c.Slots(times)
	if len(slots) == 0 {
		return domain.GroceryList{}, domain.Plan{}, ErrNoPlan
	}
	k, err := s.kitchenAt(ctx, c, slots[0], times, now)
	if err != nil {
		return domain.GroceryList{}, domain.Plan{}, err
	}
	if !k.planned {
		return domain.GroceryList{}, domain.Plan{}, ErrNoPlan
	}
	list := k.plan.GroceryList(k.forecast.Stock)
	if c.Delivery.State == domain.Tentative {
		return list, k.plan, nil
	}
	var fixed map[int64]domain.Quantity
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		fixed, err = loadGroceryLines(ctx, tx, c.ID)
		return err
	})
	list.Lines = nil
	for _, ing := range k.known { // in name order
		if q, ok := fixed[ing.ID]; ok {
			list.Lines = append(list.Lines, domain.GroceryLine{Ingredient: ing, Quantity: q})
		}
	}
	return list, k.plan, err
}

// findOrderable is the Cycle `id` while its Delivery can be marked ordered, or that
// undone, at `now`: ErrArrived once it has arrived, and ErrNoCycle unless it is upcoming
// or current.
func findOrderable(ctx context.Context, tx pgx.Tx, id int64, now time.Time) (Cycle, error) {
	c, err := findOpen(ctx, tx, id, now)
	if err == nil && c.Delivery.State == domain.Arrived {
		return c, ErrArrived
	}
	return c, err
}

// MarkOrdered marks the Delivery of the signed-in User's upcoming Cycle `id` ordered at
// `now`, fixing its Grocery List as it stands. It is ErrNoCycle when `id` isn't
// upcoming, ErrArrived once it has arrived, ErrProposalPending or ErrBeingPlanned while a
// Plan Proposal of it is pending or a planning Run of it going, and ErrNoPlan while it
// has no Plan. Once marked, it changes nothing.
func (s *Service) MarkOrdered(ctx context.Context, id int64, now time.Time) error {
	var c Cycle
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		c, err = findOrderable(ctx, tx, id, now)
		return err
	})
	if err != nil || c.Delivery.State == domain.Ordered {
		return err
	}
	// Read before the list: a replan pending or going by the time it is fixed is one
	// after ordering, which leaves the list as fixed.
	switch st, err := s.StandingOf(ctx, id); {
	case err != nil:
		return err
	case st == ProposalWaiting:
		return ErrProposalPending
	case st == BeingPlanned:
		return ErrBeingPlanned
	}
	list, err := s.GroceryListOf(ctx, c, now)
	if err != nil {
		return err
	}
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		// Drop's lock: a Cycle dropped meanwhile is ErrNoCycle, and one marked ordered
		// meanwhile is never dropped.
		if err := lockPlanning(ctx, tx); err != nil {
			return err
		}
		if _, err := findOrderable(ctx, tx, id, now); err != nil {
			return err
		}
		return markOrdered(ctx, tx, id, now, list.Lines)
	})
}

// UndoOrdered makes the Delivery of the signed-in User's upcoming Cycle `id` tentative
// again at `now`: its Grocery List follows its Plan again, and it can be dropped. It is
// ErrNoCycle when `id` isn't upcoming, and ErrArrived once its Delivery has arrived.
func (s *Service) UndoOrdered(ctx context.Context, id int64, now time.Time) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		// Arrive's lock: a Delivery being put away keeps the list it is put away with.
		if err := lockPlanning(ctx, tx); err != nil {
			return err
		}
		if _, err := findOrderable(ctx, tx, id, now); err != nil {
			return err
		}
		return undoOrdered(ctx, tx, id)
	})
}
