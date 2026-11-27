package planning

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ledger"
)

// Thawing is one of tonight's thaws as Today lists it: what, Name, how much, and the Slot
// it is For. Quantity is in Unit for a Stock Item, and how many for Portions.
type Thawing struct {
	Name     string
	Quantity domain.Quantity
	Unit     domain.Unit
	Portions bool
	For      domain.Slot
}

// ThawsTonight is the thaws of the signed-in User's kitchen on the evening of `now`'s
// day, after dinner: until then, worked out as the step will, on the kitchen as it holds
// `now`, through the Plan's Slots and the ordered Deliveries up to then; after, those it
// recorded. A User without a Cycle has none.
func (s *Service) ThawsTonight(ctx context.Context, now time.Time) ([]Thawing, error) {
	var cycles []Cycle
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		cycles, err = loadCycles(ctx, tx, now.Location())
		return err
	})
	if err != nil || len(cycles) == 0 { // a User without a Cycle may have no Profile
		return nil, err
	}
	times, err := s.profile.SlotTimes(ctx)
	if err != nil {
		return nil, err
	}
	library, err := s.recipes.Search(ctx, "")
	if err != nil {
		return nil, err
	}
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return nil, err
	}
	y, m, d := now.Date()
	tonight := domain.Slot{On: time.Date(y, m, d, 0, 0, 0, 0, now.Location()), Kind: domain.Dinner}.At(times)
	var thawed []domain.Movement
	names := map[int64]string{} // Portions' names, by id or PlannedID
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		portions, err := ledger.LoadPortions(ctx, tx)
		if err != nil {
			return err
		}
		for _, p := range portions {
			names[p.ID] = p.Name
		}
		if !now.Before(tonight) {
			thawed, err = ledger.LoadThawed(ctx, tx, tonight, now.Location())
			return err
		}
		// shortcut: a tentative Delivery due before tonight isn't put away here, as it
		// will be; worth upgrading if Today's list differs from what tonight thaws.
		plan, ordered, err := plansBetween(ctx, tx, cycles, library, known, times, now, tonight.AddDate(0, 0, 2))
		if err != nil {
			return err
		}
		movements, err := ledger.LoadMovements(ctx, tx)
		if err != nil {
			return err
		}
		madeFrom, err := ledger.LoadMadeFrom(ctx, tx)
		if err != nil {
			return err
		}
		written, made := domain.NewLedger(movements).UpToNow(plan, madeFrom, ordered, times, now, tonight)
		for _, p := range made {
			names[domain.PlannedID(p.Planned)] = p.Name
		}
		for _, m := range written {
			if m.Cause == domain.Thawed {
				thawed = append(thawed, m)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	ingredients := map[int64]domain.Ingredient{}
	for _, ing := range known {
		ingredients[ing.ID] = ing
	}
	thawings := make([]Thawing, len(thawed))
	for i, m := range thawed {
		t := Thawing{Quantity: m.Change, For: m.Slot}
		if len(m.Portions) > 0 {
			t.Name, t.Portions = names[m.Portions[0]], true
		} else {
			ing := ingredients[m.Key.Ingredient]
			t.Name, t.Unit = ing.Name, ing.Unit
		}
		thawings[i] = t
	}
	return thawings, nil
}
