package planning

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ledger"
)

// BringUpToNow is the step that brings the signed-in User's kitchen up to `now`
// (ADR-0021): in one transaction, under the planning lock and then the kitchen lock, it
// records how far the kitchen has reached, takes every Delivery whose window has ended by
// `now` as arrived, and records what the Plan assumed happened since the kitchen was last
// brought up to now. A Cycle a Delivery closes has its pending Plan Proposal rejected and
// its planning Run cancelled, naming the Delivery, and each Delivery is put away at its
// arrival, its Grocery List, fixed then if it wasn't ordered, becoming Stock Items where
// the Plan's freezer splits say (domain.PutAway). Each Cycle's Plan, as far as it lasts,
// has its Prep Sessions use up stock and prepare their Portions, and the Portion filling
// each Slot eaten, at their Slots' times, interleaved with the arrivals
// (domain.Ledger.UpToNow). A Cycle with no Plan brings nothing. Run again for a moment it
// has reached, it changes nothing. Every request runs it, as the beat does.
func (s *Service) BringUpToNow(ctx context.Context, now time.Time) error {
	return s.upToNow(ctx, now, false)
}

// upToNow is BringUpToNow, which records the scheduler's beat at `now` too when `beat`.
func (s *Service) upToNow(ctx context.Context, now time.Time, beat bool) error {
	due, upTo, cycled, err := s.dueAt(ctx, now)
	if err != nil {
		return err
	}
	// What a Plan needs is read only once a Slot of a User with a Cycle has passed: the
	// kitchen reaches no further back than `upTo`. A User without one may have
	// no Profile.
	var times domain.Times
	if cycled {
		if times, err = s.profile.SlotTimes(ctx); err != nil {
			return err
		}
	}
	var library []domain.Recipe
	var known []domain.Ingredient
	loaded := false
	load := func() (err error) {
		if library, err = s.recipes.Search(ctx, ""); err != nil {
			return err
		}
		known, err = s.ingredients.List(ctx)
		loaded = err == nil
		return err
	}
	if cycled && domain.PassBetween(times, upTo, now) {
		if err := load(); err != nil {
			return err
		}
	}
	var stops []func(context.Context)
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockPlanning(ctx, tx); err != nil {
			return err
		}
		// Read before the kitchen records reaching `now`, so the timeline reaches back as
		// far as the kitchen had. Only this step moves it, under the planning lock.
		from, err := ledger.LoadUpTo(ctx, tx, now)
		if err != nil {
			return err
		}
		passed := cycled && domain.PassBetween(times, from, now)
		var t timeline
		if passed {
			if !loaded {
				// The kitchen reached less far than `upTo` said: a first request racing
				// another, before either made its row.
				if err := load(); err != nil {
					return err
				}
			}
			if t, err = loadTimeline(ctx, tx, now, library, known); err != nil {
				return err
			}
		}
		if _, err := ledger.SaveUpTo(ctx, tx, now, beat); err != nil {
			return err
		}
		busy, err := busyCycles(ctx, tx)
		if err != nil || (len(busy) == 0 && len(due) == 0 && !passed) {
			return err
		}
		cycles := t.cycles
		if !passed {
			if cycles, err = loadCycles(ctx, tx, now.Location()); err != nil {
				return err
			}
		}
		arrivals, err := putAwayDue(ctx, tx, cycles, due, now)
		if err != nil {
			return err
		}
		for _, a := range t.ordered {
			if a.At.After(now) { // one due by now is among `arrivals`
				arrivals = append(arrivals, a)
			}
		}
		if len(arrivals) > 0 || passed {
			err := ledger.Write(ctx, tx, func(l domain.Ledger) ([]domain.Movement, error) {
				written, made := l.UpToNow(t.plan, t.madeFrom, arrivals, times, from, now)
				return written, ledger.SavePrepared(ctx, tx, made, written)
			})
			if err != nil {
				return err
			}
		}
		for i, status := range domain.CycleStatuses(plain(cycles), now) {
			c := cycles[i]
			if status != domain.Closed || !slices.Contains(busy, c.ID) {
				continue
			}
			next := cycles[i+1].Delivery // closed: the next Cycle's Delivery has arrived
			why := cause{ClosedBy: &next}
			if err := endPending(ctx, tx, c.ID, why, now); err != nil {
				return err
			}
			stop, err := s.end(ctx, tx, c.ID, why)
			if err != nil {
				return err
			}
			stops = append(stops, stop)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, stop := range stops {
		stop(ctx)
	}
	return nil
}

// putAwayDue puts away at `now`, in `tx`, each of `cycles`' Deliveries `due` says arrived
// and isn't put away yet, and is their arrivals, for the Ledger. One whose state or fixed
// list changed since `due` was worked out, before the lock, waits for the next request.
func putAwayDue(ctx context.Context, tx pgx.Tx, cycles []Cycle, due map[int64]arrival, now time.Time) ([]domain.Arrival, error) {
	var arrivals []domain.Arrival
	for _, c := range cycles {
		a, ok := due[c.ID]
		if !ok || a.state != c.Delivery.State {
			continue
		}
		same, err := fixedAs(ctx, tx, c, a.list.Lines)
		if err != nil {
			return nil, err
		}
		if !same {
			continue
		}
		done, err := putAway(ctx, tx, c.ID, now, a.list.Lines)
		if err != nil {
			return nil, err
		}
		if done {
			arrivals = append(arrivals, domain.Arrival{At: c.Delivery.Arrives(), Put: domain.PutAway(a.list.Lines, a.splits), Ordered: a.state == domain.Ordered})
		}
	}
	return arrivals, nil
}

// fixedAs reports whether the Grocery List of `c`, read in `tx`, is still fixed as
// `lines` once its Delivery is ordered: undone and marked again, it may not be.
func fixedAs(ctx context.Context, tx pgx.Tx, c Cycle, lines []domain.GroceryLine) (bool, error) {
	if c.Delivery.State != domain.Ordered {
		return true, nil
	}
	fixed, err := loadGroceryLines(ctx, tx, c.ID)
	if err != nil || len(fixed) != len(lines) {
		return false, err
	}
	for _, l := range lines {
		if fixed[l.Ingredient.ID] != l.Quantity {
			return false, nil
		}
	}
	return true, nil
}

// arrival is what a Delivery due to be put away brings: its Grocery List, as it was read
// in `state`, and its Plan's freezer splits.
type arrival struct {
	state  domain.DeliveryState
	list   domain.GroceryList
	splits []domain.RecipeIngredient
}

// dueAt is what each of the signed-in User's Deliveries arrived by `now` and not put away
// yet brings, by its Cycle's id, nothing for a Cycle without a Plan, how far their
// kitchen has been brought up to now (ledger.SaveUpTo), and whether they have any Cycle.
func (s *Service) dueAt(ctx context.Context, now time.Time) (_ map[int64]arrival, upTo time.Time, cycled bool, err error) {
	var cycles []Cycle
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		if upTo, err = ledger.LoadUpTo(ctx, tx, now); err != nil {
			return err
		}
		cycles, err = loadCycles(ctx, tx, now.Location())
		return err
	})
	if err != nil {
		return nil, upTo, false, err
	}
	due := map[int64]arrival{}
	for _, c := range cycles {
		if c.Delivery.State == domain.Arrived || c.Delivery.Arrives().After(now) {
			continue
		}
		list, plan, err := s.groceryList(ctx, c, now)
		if err != nil && !errors.Is(err, ErrNoPlan) {
			return nil, upTo, true, err
		}
		due[c.ID] = arrival{state: c.Delivery.State, list: list, splits: plan.FreezerSplits}
	}
	return due, upTo, len(cycles) > 0, nil
}

// Beat brings every User's kitchen up to `at`, the scheduler's beat, and records the beat
// for each. One User's failure is logged and doesn't stop the others.
func (s *Service) Beat(ctx context.Context, at time.Time, log *slog.Logger) {
	users, err := db.UserIDs(ctx, s.pool)
	if err != nil {
		log.ErrorContext(ctx, "kitchen beat: list the Users", "err", err)
		return
	}
	for _, id := range users {
		if err := s.upToNow(db.WithUser(ctx, id), at, true); err != nil {
			log.ErrorContext(ctx, "kitchen beat", "user", id, "err", err)
		}
	}
}
