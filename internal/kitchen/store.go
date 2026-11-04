package kitchen

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
)

// loadUserIDs is every User's id. Run outside db.AsUser, as the app's own role, it sees
// them all.
func loadUserIDs(ctx context.Context, q db.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM "user" ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// saveUpTo records the signed-in User's kitchen as brought up to now, unless it already
// is: it never moves back, and a moment reached is not written again.
func saveUpTo(ctx context.Context, q db.Querier, now time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO kitchen (up_to) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET up_to = excluded.up_to
		WHERE kitchen.up_to < excluded.up_to`,
		now)
	return err
}

// saveBeat records the scheduler's beat at now for the signed-in User, whose kitchen row
// saveUpTo has made, unless a beat as late is recorded.
func saveBeat(ctx context.Context, q db.Querier, now time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE kitchen SET beaten_at = $1
		WHERE beaten_at IS NULL OR beaten_at < $1`,
		now)
	return err
}

// loadBeat is the signed-in User's last beat, if any. It has no WHERE on purpose: run in
// db.AsUser, row-level security leaves only their row.
func loadBeat(ctx context.Context, q db.Querier) (time.Time, bool, error) {
	var at *time.Time
	err := q.QueryRow(ctx, `SELECT beaten_at FROM kitchen`).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil || at == nil {
		return time.Time{}, false, err
	}
	return *at, true, nil
}
