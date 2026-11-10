package runs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/db"
)

func insertRun(ctx context.Context, pool *pgxpool.Pool, kind, asked string, at time.Time) (id int64, err error) {
	err = db.AsUser(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO run (kind, asked, started_at) VALUES ($1, $2, $3) RETURNING id`,
			kind, asked, at).Scan(&id)
	})
	return id, err
}

// setStatus stores how the Run id, still going, ended.
func setStatus(ctx context.Context, pool *pgxpool.Pool, id int64, status string, at time.Time) error {
	return db.AsUser(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE run SET status = $2, ended_at = $3 WHERE id = $1 AND status = 'going'`, id, status, at)
		return err
	})
}

func insertTraceEvent(ctx context.Context, pool *pgxpool.Pool, runID int64, seq int, at time.Time, ev agent.TraceEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return db.AsUser(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO trace_event (run_id, seq, at, kind, data) VALUES ($1, $2, $3, $4, $5)`,
			runID, seq, at, ev.Kind, data)
		return err
	})
}

// loadRun is the signed-in User's Run id with its Trace.
func loadRun(ctx context.Context, pool *pgxpool.Pool, id int64) (run Run, found bool, err error) {
	err = db.AsUser(ctx, pool, func(tx pgx.Tx) error {
		run.ID = id
		err := tx.QueryRow(ctx, `SELECT kind, status, started_at FROM run WHERE id = $1`, id).
			Scan(&run.Kind, &run.Status, &run.StartedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		rows, _ := tx.Query(ctx, `SELECT at, kind, data FROM trace_event WHERE run_id = $1 ORDER BY seq`, id)
		run.Trace, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (StoredTraceEvent, error) {
			var s StoredTraceEvent
			var data []byte
			if err := row.Scan(&s.At, &s.Kind, &data); err != nil {
				return s, err
			}
			return s, json.Unmarshal(data, &s.TraceEvent)
		})
		return err
	})
	return run, found, err
}
