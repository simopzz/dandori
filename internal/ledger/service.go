// Package ledger is the Kitchen Ledger (ADR-0013, ADR-0019): the Movements of each User's
// kitchen, the kitchen lock every write of them takes, and the step that brings a kitchen
// up to now, run first on every request and by the scheduler's beat. That step will write
// the assumed Movements whose time has passed; until a Plan gives it any, it only records
// how far it has reached. It sits below every feature that writes Movements, so it
// imports none of them.
package ledger

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// BeatEvery is how often the scheduler beats, on the clock.
const BeatEvery = 5 * time.Minute

// Service brings kitchens up to now, on every request and the scheduler's beat.
type Service struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// New creates the Service.
func New(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{pool: pool, log: log}
}

// Write holds the signed-in User's kitchen until q's transaction ends, hands write the
// Kitchen Ledger as it then stands, and saves the Movements write makes of it, unless it
// fails. Every write of Movements goes through it, so none is checked against a Ledger
// another write has since changed.
func Write(ctx context.Context, q db.Querier, write func(domain.Ledger) ([]domain.Movement, error)) error {
	if err := lock(ctx, q); err != nil {
		return err
	}
	movements, err := LoadMovements(ctx, q)
	if err != nil {
		return err
	}
	written, err := write(domain.NewLedger(movements))
	if err != nil {
		return err
	}
	for _, m := range written {
		if err := saveMovement(ctx, q, m); err != nil {
			return err
		}
	}
	return nil
}

// CountToNothing counts each Stock Item of the signed-in User's Ingredient id holding
// some of it to nothing at now, as marking it a Staple does, in q's transaction, and is
// the Movements written, in Storage Location order, each with what its Stock Item held.
// It holds the kitchen, as a Count does, so no Count or move of it slips in before q's
// transaction ends.
func CountToNothing(ctx context.Context, q db.Querier, now time.Time, id int64) (counted []domain.Movement, err error) {
	err = Write(ctx, q, func(l domain.Ledger) ([]domain.Movement, error) {
		counted = l.CountToNothing(now, id)
		return counted, nil
	})
	return counted, err
}

// BringUpToNow brings the signed-in User's kitchen up to now. Run again for a moment it
// has reached, it changes nothing.
func (s *Service) BringUpToNow(ctx context.Context, now time.Time) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveUpTo(ctx, tx, now) })
}

// Beat brings every User's kitchen up to at, the scheduler's beat, and records the beat
// for each. One User's failure is logged and doesn't stop the others.
func (s *Service) Beat(ctx context.Context, at time.Time) {
	users, err := loadUserIDs(ctx, s.pool)
	if err != nil {
		s.log.ErrorContext(ctx, "kitchen beat: list the Users", "err", err)
		return
	}
	for _, id := range users {
		err := db.AsUser(db.WithUser(ctx, id), s.pool, func(tx pgx.Tx) error {
			if err := saveUpTo(ctx, tx, at); err != nil {
				return err
			}
			return saveBeat(ctx, tx, at)
		})
		if err != nil {
			s.log.ErrorContext(ctx, "kitchen beat", "user", id, "err", err)
		}
	}
}

// LastBeat is when the scheduler last brought the signed-in User's kitchen up to now; ok
// is false until it first has.
func (s *Service) LastBeat(ctx context.Context) (at time.Time, ok bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		at, ok, err = loadBeat(ctx, tx)
		return err
	})
	return at, ok, err
}
