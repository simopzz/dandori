// Package planning is planning the User's meals: opening a Cycle with its Delivery,
// dropping the upcoming one, and planning it into a pending Plan Proposal after the
// Kitchen Check.
package planning

import (
	"cmp"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/kitchen"
	"github.com/simopzz/dandori/internal/profile"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
)

var (
	// ErrUpcoming is opening a Cycle while another is upcoming.
	ErrUpcoming = errors.New("a Cycle is already upcoming")
	// ErrNoTargets is opening a Cycle before the User has set their Targets.
	ErrNoTargets = errors.New("the Targets aren't set")
	// ErrNoCycle is dropping a Cycle the User has none upcoming of.
	ErrNoCycle = errors.New("no such upcoming Cycle")
)

// Cycle is one of the User's Cycles, by its id.
type Cycle struct {
	ID int64
	domain.Cycle
}

// Service is the signed-in User's Cycles and planning them. It reads the Profile, the
// Recipe Library and the kitchen through their Services, and starts planning Runs.
type Service struct {
	pool        *pgxpool.Pool
	runs        *runs.Engine
	profile     *profile.Service
	recipes     *recipes.Service
	ingredients *ingredients.Service
	kitchen     *kitchen.Service
	clock       clock.Clock
	// checks is how many times a planning Run may call check_plan.
	checks int
}

// New creates the Service, its planning Runs allowed checks calls to check_plan each,
// DefaultChecks when 0.
func New(pool *pgxpool.Pool, c clock.Clock, engine *runs.Engine, prof *profile.Service, recs *recipes.Service, ings *ingredients.Service, kit *kitchen.Service, checks int) *Service {
	return &Service{pool: pool, clock: c, runs: engine, profile: prof, recipes: recs, ingredients: ings, kitchen: kit, checks: cmp.Or(checks, DefaultChecks)}
}

// Now is the signed-in User's current and upcoming Cycles at now, each nil when they
// have none.
func (s *Service) Now(ctx context.Context, now time.Time) (current, upcoming *Cycle, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		current, upcoming, err = loadNow(ctx, tx, now)
		return err
	})
	return current, upcoming, err
}

// Refusal is why the signed-in User can't open a Cycle at now, ErrNoTargets or
// ErrUpcoming, or nil when they can.
func (s *Service) Refusal(ctx context.Context, now time.Time) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return refusal(ctx, tx, now) })
}

// refusal is why the signed-in User can't open a Cycle at now, read in tx, or nil.
func refusal(ctx context.Context, tx pgx.Tx, now time.Time) error {
	_, set, err := profile.LoadTargets(ctx, tx)
	if err != nil {
		return err
	}
	if !set {
		return ErrNoTargets
	}
	_, upcoming, err := loadNow(ctx, tx, now)
	if err != nil {
		return err
	}
	if upcoming != nil {
		return ErrUpcoming
	}
	return nil
}

// Open opens a Cycle for the signed-in User at now, as typed by field in l, and is its
// id, unless a field is refused: then it opens nothing and is the problem with each.
// Before their Targets are set it is ErrNoTargets, and ErrUpcoming while a Cycle is.
func (s *Service) Open(ctx context.Context, l domain.Language, typed map[string]string, now time.Time) (id int64, problems map[string]domain.Problem, err error) {
	c, problems := domain.ParseCycle(l, typed, now)
	return id, problems, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		// Locked, so two Cycles opened at once can't both find none upcoming.
		if err := lockOpening(ctx, tx); err != nil {
			return err
		}
		if err := refusal(ctx, tx, now); err != nil || len(problems) > 0 {
			return err
		}
		id, err = saveCycle(ctx, tx, c)
		return err
	})
}

// Drop deletes the signed-in User's Cycle id while it is upcoming at now, with its Plan
// Proposals, its planning Run going cancelled, and is ErrNoCycle otherwise.
func (s *Service) Drop(ctx context.Context, id int64, now time.Time) error {
	var stop func(context.Context)
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		if _, err := findUpcoming(ctx, tx, id, now); err != nil {
			return err
		}
		if stop, err = s.end(ctx, tx, id); err != nil {
			return err
		}
		return deleteCycle(ctx, tx, id)
	})
	if err != nil {
		return err
	}
	stop(ctx)
	return nil
}

// Find is the signed-in User's Cycle id while it is upcoming or current at now; once it
// has closed, or when they have none, it is ErrNoCycle.
func (s *Service) Find(ctx context.Context, id int64, now time.Time) (c Cycle, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		current, upcoming, err := loadNow(ctx, tx, now)
		for _, open := range []*Cycle{current, upcoming} {
			if open != nil && open.ID == id {
				c = *open
				return err
			}
		}
		return cmp.Or(err, ErrNoCycle)
	})
	return c, err
}

// Upcoming is the signed-in User's Cycle id while it is upcoming at now; otherwise it
// is ErrNoCycle.
func (s *Service) Upcoming(ctx context.Context, id int64, now time.Time) (c Cycle, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		c, err = findUpcoming(ctx, tx, id, now)
		return err
	})
	return c, err
}

// findUpcoming is the Cycle id while it is upcoming at now, and ErrNoCycle otherwise.
func findUpcoming(ctx context.Context, tx pgx.Tx, id int64, now time.Time) (Cycle, error) {
	_, upcoming, err := loadNow(ctx, tx, now)
	if err != nil {
		return Cycle{}, err
	}
	if upcoming == nil || upcoming.ID != id {
		return Cycle{}, ErrNoCycle
	}
	return *upcoming, nil
}
