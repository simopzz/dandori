package planning

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ledger"
)

// GroceryListOf is the Grocery List of the signed-in User's Cycle `c` at `now`: its Plan
// against the kitchen forecast at its first Slot, once the approved Prep Sessions before
// it have used up theirs. `planned` is false, and the list empty, while `c` has no Plan.
func (s *Service) GroceryListOf(ctx context.Context, c Cycle, now time.Time) (list domain.GroceryList, planned bool, err error) {
	times, err := s.profile.SlotTimes(ctx)
	if err != nil {
		return list, false, err
	}
	slots := c.Slots(times)
	if len(slots) == 0 {
		return list, false, nil
	}
	var own, approved []*storedProposal
	var portions []domain.Portion
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		if own, err = loadPlanOf(ctx, tx, c.ID, now.Location()); err != nil || len(own) == 0 {
			return err
		}
		if approved, err = loadApproved(ctx, tx, slots[0].On.AddDate(0, 0, -7), now.Location()); err != nil {
			return err
		}
		portions, err = ledger.LoadPortions(ctx, tx)
		return err
	})
	if err != nil || len(own) == 0 {
		return list, false, err
	}
	library, err := s.recipes.Search(ctx, "")
	if err != nil {
		return list, false, err
	}
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return list, false, err
	}
	held, err := s.kitchen.Contents(ctx, now)
	if err != nil {
		return list, false, err
	}
	forecast := approvedPlan(approved, library, known, portions).Forecast(stockOf(held.Held), held.Portions, times, now, slots[0])
	return cyclePlan(c, own, library, known, portions, times).GroceryList(forecast.Stock), true, nil
}
