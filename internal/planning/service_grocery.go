package planning

import (
	"context"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// GroceryListOf is the Grocery List of the signed-in User's Cycle `c` at `now`: its Plan
// against the kitchen forecast at its first Slot, once the approved Prep Sessions before
// it have used up theirs. It is ErrNoPlan while `c` has no Plan, as a Cycle with no Slot
// never does.
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
	return k.plan.GroceryList(k.forecast.Stock), nil
}
