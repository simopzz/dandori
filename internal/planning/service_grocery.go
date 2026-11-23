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
)

// GroceryListOf is the Grocery List of the signed-in User's Cycle `c` at `now`: once its
// Delivery is marked ordered, the lines fixed then; until it is, its Plan against the
// kitchen forecast at its first Slot, once the approved Prep Sessions before it have
// used up theirs. The Staples to check are always those its Plan uses. It is ErrNoPlan
// while `c` has no Plan, as a Cycle with no Slot never does.
func (s *Service) GroceryListOf(ctx context.Context, c Cycle, now time.Time) (domain.GroceryList, error) {
	times, err := s.profile.SlotTimes(ctx)
	if err != nil {
		return domain.GroceryList{}, err
	}
	slots := c.Slots(times)
	if len(slots) == 0 {
		return domain.GroceryList{}, ErrNoPlan
	}
	k, err := s.kitchenAt(ctx, c, slots[0], times, now)
	if err != nil {
		return domain.GroceryList{}, err
	}
	if !k.planned {
		return domain.GroceryList{}, ErrNoPlan
	}
	list := k.plan.GroceryList(k.forecast.Stock)
	if !c.Ordered {
		return list, nil
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
	return list, err
}

// MarkOrdered marks the Delivery of the signed-in User's upcoming Cycle `id` ordered at
// `now`, fixing its Grocery List as it stands. It is ErrNoCycle when `id` isn't
// upcoming, ErrProposalPending or ErrBeingPlanned while a Plan Proposal of it is pending
// or a planning Run of it going, and ErrNoPlan while it has no Plan. Once marked, it
// changes nothing.
func (s *Service) MarkOrdered(ctx context.Context, id int64, now time.Time) error {
	c, err := s.Upcoming(ctx, id, now)
	if err != nil || c.Ordered {
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
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return markOrdered(ctx, tx, id, now, list.Lines) })
}

// UndoOrdered makes the Delivery of the signed-in User's upcoming Cycle `id` tentative
// again at `now`: its Grocery List follows its Plan again, and it can be dropped. It is
// ErrNoCycle when `id` isn't upcoming.
func (s *Service) UndoOrdered(ctx context.Context, id int64, now time.Time) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := findUpcoming(ctx, tx, id, now); err != nil {
			return err
		}
		return undoOrdered(ctx, tx, id)
	})
}
