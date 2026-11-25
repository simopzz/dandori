// Package ledger is the Kitchen Ledger (ADR-0013, ADR-0019): the Movements of each User's
// kitchen, the kitchen lock every write of them takes, and the `kitchen` row with how far
// the kitchen is brought up to now and the beat, which `planning`'s step writes
// (ADR-0021). It sits below every feature that writes Movements, so it imports none
// of them.
package ledger

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// BeatEvery is how often the scheduler beats, on the clock.
const BeatEvery = 5 * time.Minute

// Service tells when the scheduler last beat.
type Service struct {
	pool *pgxpool.Pool
}

// New creates the Service.
func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Write holds the signed-in User's kitchen until `q`'s transaction ends, hands `write`
// the Kitchen Ledger as it then stands, and saves the Movements `write` makes of it,
// unless it fails. Every write of Movements goes through it, so none is checked against a
// Ledger another write has since changed.
func Write(ctx context.Context, q db.Querier, write func(domain.Ledger) ([]domain.Movement, error)) error {
	return writeWith(ctx, q, func() (domain.Ledger, error) {
		movements, err := LoadMovements(ctx, q)
		return domain.NewLedger(movements), err
	}, write)
}

// WriteIngredient is Write for `write`s that read and write only the Ingredient `id`'s
// Stock Items: it hands `write` the Ledger of that Ingredient's Movements alone, with the
// latest Movement's time, and holds the kitchen the same way.
func WriteIngredient(ctx context.Context, q db.Querier, id int64, write func(domain.Ledger) ([]domain.Movement, error)) error {
	return writeWith(ctx, q, func() (domain.Ledger, error) { return LoadIngredientLedger(ctx, q, id) }, write)
}

// writeWith holds the kitchen, hands `write` the Ledger `load` reads, and saves what
// `write` makes of it.
func writeWith(ctx context.Context, q db.Querier, load func() (domain.Ledger, error), write func(domain.Ledger) ([]domain.Movement, error)) error {
	if err := lock(ctx, q); err != nil {
		return err
	}
	l, err := load()
	if err != nil {
		return err
	}
	written, err := write(l)
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

// CountToNothing counts each Stock Item of the signed-in User's Ingredient `id` holding
// some of it to nothing at `now`, as marking it a Staple does, in `q`'s transaction, and
// is the Movements written, in Storage Location order, each with what its Stock Item
// held. It holds the kitchen, as a Count does, so no Count or move of it slips in before
// `q`'s transaction ends.
func CountToNothing(ctx context.Context, q db.Querier, now time.Time, id int64) (counted []domain.Movement, err error) {
	err = WriteIngredient(ctx, q, id, func(l domain.Ledger) ([]domain.Movement, error) {
		counted = l.CountToNothing(now, id)
		return counted, nil
	})
	return counted, err
}

// SavePortions saves how many Portions `p` says for the signed-in User, in `q`'s
// transaction, and is their ids, for the Movement that adds them, inside Write. It holds
// the kitchen first, as Write does, so the Portion Lots a Count or move read don't change
// under it.
func SavePortions(ctx context.Context, q db.Querier, p domain.NewPortions) ([]int64, error) {
	if err := lock(ctx, q); err != nil {
		return nil, err
	}
	return savePortions(ctx, q, p)
}

// SaveUpTo records, in `q`'s transaction, the signed-in User's kitchen as brought up to
// `now`, and the scheduler's beat at `now` too when `beat`, for `planning`'s step
// (ADR-0021). It makes their `kitchen` row and holds it, as Write does, until `q`'s
// transaction ends. Neither moves back, and a moment reached is not written again.
func SaveUpTo(ctx context.Context, q db.Querier, now time.Time, beat bool) error {
	if err := saveUpTo(ctx, q, now); err != nil {
		return err
	}
	if err := lock(ctx, q); err != nil || !beat {
		return err
	}
	return saveBeat(ctx, q, now)
}

// LastBeat is when the scheduler last brought the signed-in User's kitchen up to now;
// `ok` is false until it first has.
func (s *Service) LastBeat(ctx context.Context) (at time.Time, ok bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		at, ok, err = loadBeat(ctx, tx)
		return err
	})
	return at, ok, err
}
