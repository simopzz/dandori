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

// PlanAt is each of `slots` as the signed-in User's Plans fill them at `now`, in the same
// order, and the thaws of their kitchen on the evening of `now`'s day, after dinner:
// until then, worked out as the step will, on the kitchen as it holds `now`, through the
// Plan's Slots and the ordered Deliveries up to then; after, those it recorded. A User
// without a Cycle has none filled and no thaws.
func (s *Service) PlanAt(ctx context.Context, now time.Time, slots []domain.Slot) ([]Filled, []domain.Thaw, error) {
	out := make([]Filled, len(slots))
	for i, slot := range slots {
		out[i].Slot = slot
	}
	t, err := s.timeline(ctx, now)
	if err != nil || len(t.cycles) == 0 {
		return out, nil, err
	}
	held, eaten := t.ledger.PortionsAt(now), map[int64]bool{}
	for _, m := range t.movements {
		if m.Cause == domain.PortionEaten {
			for _, id := range m.Portions {
				eaten[id] = true
			}
		}
	}
	for i := range out {
		f := &out[i]
		fill(f, t.plan, t.portions)
		var sessions []domain.PrepSession
		for _, ps := range t.plan.PrepSessions {
			if ps.At.Equal(f.Slot) {
				sessions = append(sessions, ps)
			}
		}
		var from []map[int64][]domain.StorageLocation
		if f.Slot.At(t.times).After(now) {
			// shortcut: the kitchen as it holds now, before the day's earlier Prep
			// Sessions use their share; worth walking them if a Slot's
			// places mislead.
			from = t.ledger.TakesFrom(now, sessions)
		}
		for j, ps := range sessions {
			f.Sessions = append(f.Sessions, Session{PrepSession: ps})
			if from != nil {
				f.Sessions[j].TakesFrom = from[j]
			}
		}
		id, ok := t.plan.Filling(f.Slot, t.madeFrom)
		if !ok {
			continue // nothing fills it, or not prepared yet
		}
		at, isHeld := held[id]
		if isHeld {
			f.Location = at
		}
		f.Missing = !isHeld && !eaten[id]
	}
	thaws, err := s.thawsTonight(ctx, t, now)
	return out, thaws, err
}

// thawsTonight is the thaws of `t`'s kitchen on the evening of `now`'s day, as Today
// shows them.
func (s *Service) thawsTonight(ctx context.Context, t timeline, now time.Time) ([]domain.Thaw, error) {
	y, m, d := now.Date()
	tonight := domain.Slot{On: time.Date(y, m, d, 0, 0, 0, 0, now.Location()), Kind: domain.Dinner}.At(t.times)
	portions := t.portions // by id, and those the Plan prepares by now by PlannedID
	var thawed []domain.Movement
	if !now.Before(tonight) {
		err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
			thawed, err = ledger.LoadThawed(ctx, tx, tonight, now.Location())
			return err
		})
		return domain.ThawsOf(thawed, t.known, portions), err
	}
	// shortcut: a tentative Delivery due before tonight isn't put away here, as it will
	// be; worth upgrading if Today's list differs from what tonight thaws.
	written, made := t.ledger.UpToNow(t.plan, t.madeFrom, t.ordered, t.times, now, tonight)
	for _, p := range made {
		portions = append(portions, domain.Portion{ID: domain.PlannedID(p.Planned), Name: p.Name, Nutrition: p.Nutrition})
	}
	for _, m := range written {
		if m.Cause == domain.Thawed {
			thawed = append(thawed, m)
		}
	}
	return domain.ThawsOf(thawed, t.known, portions), nil
}

// fill gives `f` the name of the Portion `p` puts in its Slot, the Prep Session it comes
// from and where it is kept, `portions` naming one already in the kitchen.
func fill(f *Filled, p domain.Plan, portions []domain.Portion) {
	for _, slot := range p.PlannedSlots() {
		switch {
		case !slot.At.Equal(f.Slot):
		case slot.From != nil:
			f.Name, f.From, f.Location = slot.From.Name(), slot.From, slot.Planned.Location
			return
		case slot.Planned != nil: // another Cycle's Plan prepares it
			return
		default:
			if i := slices.IndexFunc(portions, func(p domain.Portion) bool { return p.ID == slot.Portion }); i >= 0 {
				f.Name = portions[i].Name
			}
			return
		}
	}
}
