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
			cause_kind, coalesce(believed, 0), coalesce(stated, 0), coalesce(note, '')
		FROM movement ORDER BY happened_at, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Movement, error) {
		var (
			m                   domain.Movement
			location, to, cause string
			change, believed    float64
			stated              float64
		)
		err := row.Scan(&m.At, &m.Key.Ingredient, &location, &to, &change, &cause, &believed, &stated, &m.Note)
		m.Key.Location, m.To = domain.StorageLocation(location), domain.StorageLocation(to)
		m.Change, m.Cause = domain.QuantityOf(change), domain.Cause(cause)
		m.Believed, m.Stated = domain.QuantityOf(believed), domain.QuantityOf(stated)
		return m, err
	})
}

// saveMovement appends m to the signed-in User's Kitchen Ledger. A move keeps nothing of
// what was believed or stated, and a blank Storage Location to or note is none.
func saveMovement(ctx context.Context, q db.Querier, m domain.Movement) error {
	var believed, stated *float64
	if m.Cause != domain.Moved {
		b, s := m.Believed.Number(), m.Stated.Number()
		believed, stated = &b, &s
	}
	_, err := q.Exec(ctx, `
		INSERT INTO movement (happened_at, ingredient_id, storage_location, moved_to, change, cause_kind, believed, stated, note)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, NULLIF($9, ''))`,
		m.At, m.Key.Ingredient, string(m.Key.Location), string(m.To), m.Change.Number(), string(m.Cause),
		believed, stated, m.Note)
	return err
}

// HasMovements says whether any Movement of the signed-in User's refers to the Ingredient
// id, for the Ingredients, whose unit is then fixed. It holds the kitchen first, as a
// Count or move does, so none of them writes a Movement of it until q's transaction ends.
func HasMovements(ctx context.Context, q db.Querier, id int64) (bool, error) {
	if err := lockKitchen(ctx, q); err != nil {
		return false, err
	}
	var has bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM movement WHERE ingredient_id = $1)`, id).Scan(&has)
	return has, err
}

// WithMovements is the ids of the signed-in User's Ingredients that some Movement refers
// to, in q's transaction, for the Ingredients, whose rows link to their Movements.
func WithMovements(ctx context.Context, q db.Querier) (map[int64]bool, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT ingredient_id FROM movement`)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	with := make(map[int64]bool, len(ids))
	for _, id := range ids {
		with[id] = true
	}
	return with, err
}
