package planning

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/profile"
)

// timeline is the signed-in User's Plan by time, as one use case reads it: their Cycles,
// the Plans of those not closed before the earlier of how far the kitchen has been
// brought up to now and a day before `now`, together (each Cycle's as its approved Plan
// Proposals leave it, Until the next Cycle's Delivery arrives; days between Cycles are in
// none), those Cycles' Deliveries ordered and not put away yet, as they will be, the
// Ledger and its Movements, the Portion each Planned Portion was prepared as, the
// Portions, the known Ingredients and the Slot times. Its readers keep what falls in the
// time they look at. A User without a Cycle has no more than that: they may have
// no Profile.
type timeline struct {
	cycles    []Cycle
	plan      domain.Plan
	ordered   []domain.Arrival
	movements []domain.Movement
	ledger    domain.Ledger
	madeFrom  map[int64]int64
	portions  []domain.Portion
	known     []domain.Ingredient
	times     domain.Times
}

// timeline is the signed-in User's timeline at `now`, its Recipes and Ingredients found
// among their Recipe Library and Ingredients.
func (s *Service) timeline(ctx context.Context, now time.Time) (t timeline, err error) {
	library, err := s.recipes.Search(ctx, "")
	if err != nil {
		return t, err
	}
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return t, err
	}
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		t, err = loadTimeline(ctx, tx, now, library, known)
		return err
	})
	return t, err
}

// loadTimeline is the signed-in User's timeline at `now`, read in `q`, its Recipes and
// Ingredients found among `library` and `known`.
func loadTimeline(ctx context.Context, q db.Querier, now time.Time, library []domain.Recipe, known []domain.Ingredient) (t timeline, err error) {
	t.known = known
	if t.cycles, err = loadCycles(ctx, q, now.Location()); err != nil || len(t.cycles) == 0 {
		return t, err
	}
	if t.times, err = profile.LoadSlotTimes(ctx, q); err != nil {
		return t, err
	}
	since, err := ledger.LoadUpTo(ctx, q, now)
	if err != nil {
		return t, err
	}
	if dayBefore := now.AddDate(0, 0, -1); dayBefore.Before(since) {
		since = dayBefore
	}
	for _, c := range t.cycles {
		if !c.Until.IsZero() && !c.Until.After(since) {
			continue
		}
		own, err := loadPlanOf(ctx, q, c.ID, now.Location())
		if err != nil {
			return t, err
		}
		p := cyclePlan(c, own, library, known, nil, t.times)
		t.plan.PrepSessions = append(t.plan.PrepSessions, p.PrepSessions...)
		t.plan.Kept = append(t.plan.Kept, p.Kept...)
		if c.Delivery.State != domain.Ordered {
			continue
		}
		fixed, err := loadGroceryLines(ctx, q, c.ID)
		if err != nil {
			return t, err
		}
		lines := groceryLinesOf(known, fixed)
		t.ordered = append(t.ordered, domain.Arrival{At: c.Delivery.Arrives(), Put: domain.PutAway(lines, p.FreezerSplits), Ordered: true})
	}
	if t.movements, err = ledger.LoadMovements(ctx, q); err != nil {
		return t, err
	}
	t.ledger = domain.NewLedger(t.movements)
	if t.madeFrom, err = ledger.LoadMadeFrom(ctx, q); err != nil {
		return t, err
	}
	t.portions, err = ledger.LoadPortions(ctx, q)
	return t, err
}
