package planning

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// lockOpening locks the signed-in User's Profile until the transaction ends, so opening
// Cycles waits its turn.
func lockOpening(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM profile FOR UPDATE`)
	return err
}

// loadNow reads the signed-in User's current and upcoming Cycles at now, each nil when
// they have none, its dates as days in now's zone. It has no WHERE on purpose: run in
// db.AsUser, row-level security leaves only their rows.
func loadNow(ctx context.Context, q db.Querier, now time.Time) (current, upcoming *Cycle, err error) {
	rows, err := q.Query(ctx, `
		SELECT id, delivery_on, extract(epoch FROM window_from)::int / 60, extract(epoch FROM window_to)::int / 60, ends_on
		FROM cycle ORDER BY delivery_on, window_to`)
	if err != nil {
		return nil, nil, err
	}
	day := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
	}
	cycles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Cycle, error) {
		var c Cycle
		err := row.Scan(&c.ID, &c.Delivery.On, &c.Delivery.From, &c.Delivery.To, &c.Ends)
		c.Delivery.On, c.Ends = day(c.Delivery.On), day(c.Ends)
		return c, err
	})
	if err != nil {
		return nil, nil, err
	}
	plain := make([]domain.Cycle, len(cycles))
	for i, c := range cycles {
		plain[i] = c.Cycle
	}
	for i, status := range domain.CycleStatuses(plain, now) {
		switch status {
		case domain.Current:
			current = &cycles[i]
		case domain.Upcoming:
			upcoming = &cycles[i]
		}
	}
	return current, upcoming, nil
}

// saveCycle adds c to the signed-in User's Cycles.
func saveCycle(ctx context.Context, q db.Querier, c domain.Cycle) error {
	_, err := q.Exec(ctx, `INSERT INTO cycle (delivery_on, window_from, window_to, ends_on) VALUES ($1, $2, $3, $4)`,
		c.Delivery.On.Format(time.DateOnly), c.Delivery.From.String(), c.Delivery.To.String(), c.Ends.Format(time.DateOnly))
	return err
}

// deleteCycle deletes the signed-in User's Cycle id.
func deleteCycle(ctx context.Context, q db.Querier, id int64) error {
	_, err := q.Exec(ctx, `DELETE FROM cycle WHERE id = $1`, id)
	return err
}
