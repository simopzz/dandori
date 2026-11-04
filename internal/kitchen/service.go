// Package kitchen brings each User's kitchen up to now (ADR-0013): one step, run first on
// every request and by the scheduler's beat, which will write the assumed Movements whose
// time has passed once the Kitchen Ledger exists. Until then it only records how far it
// has brought the kitchen.
package kitchen

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
)

// BeatEvery is how often the scheduler beats, on the clock.
const BeatEvery = 5 * time.Minute

// Service is the step that brings a kitchen up to now.
type Service struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// New creates the Service.
func New(pool *pgxpool.Pool, log *slog.Logger) *Service { return &Service{pool: pool, log: log} }

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
		s.log.Error("kitchen beat: list the Users", "err", err)
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
			s.log.Error("kitchen beat", "user", id, "err", err)
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
