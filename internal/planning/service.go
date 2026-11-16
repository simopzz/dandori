// Package planning is planning the User's meals: for now, opening a Cycle with its
// Delivery and dropping the upcoming one. Later slices plan it into a Plan.
package planning

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/profile"
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

// Service is the signed-in User's Cycles.
type Service struct {
	pool *pgxpool.Pool
}

// New creates the Service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

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

// Open opens a Cycle for the signed-in User at now, as typed by field in l, unless a
// field is refused: then it opens nothing and is the problem with each. It is
// ErrNoTargets before their Targets are set, and ErrUpcoming while a Cycle is upcoming.
func (s *Service) Open(ctx context.Context, l domain.Language, typed map[string]string, now time.Time) (map[string]domain.Problem, error) {
	c, problems := domain.ParseCycle(l, typed, now)
	return problems, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		// Locked, so two Cycles opened at once can't both find none upcoming.
		if err := lockOpening(ctx, tx); err != nil {
			return err
		}
		if err := refusal(ctx, tx, now); err != nil || len(problems) > 0 {
			return err
		}
		return saveCycle(ctx, tx, c)
	})
}

// Drop deletes the signed-in User's Cycle id while it is upcoming at now, and is
// ErrNoCycle otherwise.
func (s *Service) Drop(ctx context.Context, id int64, now time.Time) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := findUpcoming(ctx, tx, id, now); err != nil {
			return err
		}
		return deleteCycle(ctx, tx, id)
	})
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
