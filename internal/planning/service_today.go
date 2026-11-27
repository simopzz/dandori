package planning

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ledger"
)

// Filled is a Slot as the Plan fills it, for Today.
type Filled struct {
	Slot domain.Slot
	// Name is the Portion filling the Slot's, "" when none does: no Plan holds the Slot.
	Name string
	// From is the Prep Session the Portion comes from, nil for one already in the
	// kitchen, and Location where it is kept: where the kitchen holds it, once it does.
	From     *domain.PrepSession
	Location domain.StorageLocation
	// Missing is whether the kitchen doesn't hold the Portion, prepared or kept, nor has
	// eaten it.
	Missing bool
	// Sessions are the Prep Sessions at the Slot.
	Sessions []Session
}

// Session is a Prep Session at a Slot Today shows, with where each of its Ingredients
// would be taken from, by Ingredient id (domain.Ledger.TakesFrom): nil once its Slot's
// time has passed, as it took them.
type Session struct {
	domain.PrepSession
	TakesFrom map[int64][]domain.StorageLocation
}

// FilledAt is each of `slots` as the signed-in User's Plans fill them at `now`, in the
// same order. A User without a Cycle has none filled.
func (s *Service) FilledAt(ctx context.Context, now time.Time, slots []domain.Slot) ([]Filled, error) {
	out := make([]Filled, len(slots))
	for i, slot := range slots {
		out[i].Slot = slot
	}
	var cycles []Cycle
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		cycles, err = loadCycles(ctx, tx, now.Location())
		return err
	})
	if err != nil || len(cycles) == 0 { // a User without a Cycle may have no Profile
		return out, err
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
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		plan, _, err := plansBetween(ctx, tx, cycles, library, known, times, now.AddDate(0, 0, -1), now.AddDate(0, 0, 2))
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
		portions, err := ledger.LoadPortions(ctx, tx)
		if err != nil {
			return err
		}
		l := domain.NewLedger(movements)
		held, eaten := l.PortionsAt(now), map[int64]bool{}
		for _, m := range movements {
			if m.Cause == domain.PortionEaten {
				for _, id := range m.Portions {
					eaten[id] = true
				}
			}
		}
		for i := range out {
			f := &out[i]
			fill(f, plan, portions)
			var sessions []domain.PrepSession
			for _, ps := range plan.PrepSessions {
				if ps.At.Equal(f.Slot) {
					sessions = append(sessions, ps)
				}
			}
			var from []map[int64][]domain.StorageLocation
			if f.Slot.At(times).After(now) {
				// shortcut: the kitchen as it holds now, before the day's earlier Prep
				// Sessions use their share; worth walking them if a Slot's
				// places mislead.
				from = l.TakesFrom(now, sessions)
			}
			for j, ps := range sessions {
				f.Sessions = append(f.Sessions, Session{PrepSession: ps})
				if from != nil {
					f.Sessions[j].TakesFrom = from[j]
				}
			}
			id, ok := plan.Filling(f.Slot, madeFrom)
			if !ok {
				continue // nothing fills it, or not prepared yet
			}
			at, isHeld := held[id]
			if isHeld {
				f.Location = at
			}
			f.Missing = !isHeld && !eaten[id]
		}
		return nil
	})
	return out, err
}

// fill gives `f` the name of the Portion `p` puts in its Slot, the Prep Session it comes
// from and where it is kept, `portions` naming one already in the kitchen.
func fill(f *Filled, p domain.Plan, portions []domain.Portion) {
	planned := func(id int64) bool {
		for i, ps := range p.PrepSessions {
			for _, pp := range ps.Portions {
				if pp.ID == id {
					f.Name, f.From, f.Location = ps.Name(), &p.PrepSessions[i], pp.Location
					return true
				}
			}
		}
		return false
	}
	for _, ps := range p.PrepSessions {
		for _, pp := range ps.Portions {
			if pp.EatAt != nil && pp.EatAt.Equal(f.Slot) && planned(pp.ID) {
				return
			}
		}
	}
	for _, k := range p.Kept {
		switch {
		case !k.At.Equal(f.Slot):
		case k.Portion < 0:
			planned(-k.Portion) // a Planned Portion by its PlannedID
			return
		default:
			if i := slices.IndexFunc(portions, func(p domain.Portion) bool { return p.ID == k.Portion }); i >= 0 {
				f.Name = portions[i].Name
			}
			return
		}
	}
}
