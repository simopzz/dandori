package today

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
)

// visit records the signed-in User's visit to Today at `now`, and is when they visited
// before, `ok` false if they never did. Run in db.AsUser, row-level security leaves only
// their row.
func visit(ctx context.Context, q db.Querier, now time.Time) (before time.Time, ok bool, err error) {
	err = q.QueryRow(ctx, `SELECT visited_at FROM today_visit`).Scan(&before)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	} else {
		ok = err == nil
	}
	if err != nil {
		return before, ok, err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO today_visit (visited_at) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET visited_at = excluded.visited_at`, now)
	return before, ok, err
}
