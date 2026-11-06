package kitchen

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
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

// lockKitchen holds the signed-in User's kitchen until the transaction ends, so what one
// Count or move read isn't changed by another before it writes. UpToNow made their row.
func lockKitchen(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM kitchen FOR UPDATE`)
	return err
}

// loadMovements is the signed-in User's Kitchen Ledger in the order it happened, those of
// one moment in the order they were written. It has no WHERE on purpose: run in
// db.AsUser, row-level security leaves only their rows.
func loadMovements(ctx context.Context, q db.Querier) ([]domain.Movement, error) {
	rows, err := q.Query(ctx, `
		SELECT happened_at, ingredient_id, storage_location, coalesce(moved_to, ''), change,
			cause_kind, coalesce(believed, 0), coalesce(note, '')
		FROM movement ORDER BY happened_at, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Movement, error) {
		var (
			m                   domain.Movement
			location, to, cause string
			change, believed    float64
		)
		err := row.Scan(&m.At, &m.Key.Ingredient, &location, &to, &change, &cause, &believed, &m.Note)
		m.Key.Location, m.To = domain.StorageLocation(location), domain.StorageLocation(to)
		m.Change, m.Cause, m.Believed = domain.QuantityOf(change), domain.Cause(cause), domain.QuantityOf(believed)
		return m, err
	})
}

// loadCount is what the signed-in User's Count id was of, and what they stated; ok is
// false if they have no such Count.
func loadCount(ctx context.Context, q db.Querier, id int64) (ingredient int64, location domain.StorageLocation, stated domain.Quantity, ok bool, err error) {
	var (
		where  string
		amount float64
	)
	err = q.QueryRow(ctx, `
		SELECT ingredient_id, storage_location, stated FROM movement
		WHERE id = $1 AND cause_kind = $2`, id, string(domain.Counted)).Scan(&ingredient, &where, &amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", 0, false, nil
	}
	return ingredient, domain.StorageLocation(where), domain.QuantityOf(amount), err == nil, err
}

// saveCount appends a Count of the Ingredient id in location, which keeps both what
// Dandori believed and what the User stated, and is its id. A blank note is none.
func saveCount(ctx context.Context, q db.Querier, at time.Time, id int64, location domain.StorageLocation, believed, stated domain.Quantity, note string) (int64, error) {
	var counted int64
	err := q.QueryRow(ctx, `
		INSERT INTO movement (happened_at, ingredient_id, storage_location, change, cause_kind, believed, stated, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''))
		RETURNING id`,
		at, id, string(location), domain.CountDifference(believed, stated).Number(), string(domain.Counted),
		believed.Number(), stated.Number(), note).Scan(&counted)
	return counted, err
}

// saveMove appends a move of moved of the Ingredient id from one Storage Location to
// another. A blank note is none.
func saveMove(ctx context.Context, q db.Querier, at time.Time, id int64, from, to domain.StorageLocation, moved domain.Quantity, note string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO movement (happened_at, ingredient_id, storage_location, moved_to, change, cause_kind, note)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))`,
		at, id, string(from), string(to), moved.Number(), string(domain.Moved), note)
	return err
}
