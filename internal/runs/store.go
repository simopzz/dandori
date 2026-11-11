package runs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
)

func insertRun(ctx context.Context, q db.Querier, kind, asked string, at time.Time) (id int64, err error) {
	err = q.QueryRow(ctx, `INSERT INTO run (kind, asked, started_at) VALUES ($1, $2, $3) RETURNING id`,
		kind, asked, at).Scan(&id)
	return id, err
}

// setStatus stores how the Run id ended, stored false when it had already.
func setStatus(ctx context.Context, q db.Querier, id int64, status Status, at time.Time) (stored bool, err error) {
	tag, err := q.Exec(ctx, `UPDATE run SET status = $2, ended_at = $3 WHERE id = $1 AND status = $4`,
		id, status, at, Going)
	return tag.RowsAffected() == 1, err
}

func insertTraceEvent(ctx context.Context, q db.Querier, runID int64, ev StoredTraceEvent) error {
	data, err := json.Marshal(ev.TraceEvent)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO trace_event (run_id, seq, at, kind, data) VALUES ($1, $2, $3, $4, $5)`,
		runID, ev.Seq, ev.At, ev.Kind, data)
	return err
}

// listRuns is the signed-in User's Runs, newest first, without their Traces, each with
// the first line of what it was asked.
func listRuns(ctx context.Context, q db.Querier) ([]Run, error) {
	rows, _ := q.Query(ctx, `
		SELECT id, kind, status, split_part(ltrim(asked, E' \t\r\n'), E'\n', 1), started_at FROM run
		ORDER BY started_at DESC, id DESC`)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (r Run, err error) {
		return r, row.Scan(&r.ID, &r.Kind, &r.Status, &r.Asked, &r.StartedAt)
	})
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
	run.Trace, err = loadTrace(ctx, q, id, 0)
	return run, err == nil, err
}

// loadTrace is the Trace Events of the signed-in User's Run id after the seq-th.
func loadTrace(ctx context.Context, q db.Querier, id int64, seq int) ([]StoredTraceEvent, error) {
	rows, _ := q.Query(ctx, `SELECT seq, at, kind, data FROM trace_event WHERE run_id = $1 AND seq > $2 ORDER BY seq`, id, seq)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (StoredTraceEvent, error) {
		var s StoredTraceEvent
		var data []byte
		if err := row.Scan(&s.Seq, &s.At, &s.Kind, &data); err != nil {
			return s, err
		}
		return s, json.Unmarshal(data, &s.TraceEvent)
	})
}
