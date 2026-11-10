package runs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/db"
)

func insertRun(ctx context.Context, q db.Querier, kind, asked string, at time.Time) (id int64, err error) {
	err = q.QueryRow(ctx, `INSERT INTO run (kind, asked, started_at) VALUES ($1, $2, $3) RETURNING id`,
		kind, asked, at).Scan(&id)
	return id, err
}

// setStatus stores how the Run id, still going, ended.
func setStatus(ctx context.Context, q db.Querier, id int64, status Status, at time.Time) error {
	_, err := q.Exec(ctx, `UPDATE run SET status = $2, ended_at = $3 WHERE id = $1 AND status = $4`,
		id, status, at, Going)
	return err
}

func insertTraceEvent(ctx context.Context, q db.Querier, runID int64, seq int, at time.Time, ev agent.TraceEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO trace_event (run_id, seq, at, kind, data) VALUES ($1, $2, $3, $4, $5)`,
		runID, seq, at, ev.Kind, data)
	return err
}

// loadRun is the signed-in User's Run id with its Trace.
func loadRun(ctx context.Context, q db.Querier, id int64) (run Run, found bool, err error) {
	run.ID = id
	err = q.QueryRow(ctx, `SELECT kind, status, asked, started_at FROM run WHERE id = $1`, id).
		Scan(&run.Kind, &run.Status, &run.Asked, &run.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, false, nil
	}
	if err != nil {
		return run, false, err
	}
	rows, _ := q.Query(ctx, `SELECT at, kind, data FROM trace_event WHERE run_id = $1 ORDER BY seq`, id)
	run.Trace, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (StoredTraceEvent, error) {
		var s StoredTraceEvent
		var data []byte
		if err := row.Scan(&s.At, &s.Kind, &data); err != nil {
			return s, err
		}
		return s, json.Unmarshal(data, &s.TraceEvent)
	})
	return run, err == nil, err
}
