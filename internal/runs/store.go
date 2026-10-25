package runs

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/web"
)

func insertRun(ctx context.Context, q db.Querier, kind string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO run (kind) VALUES ($1) RETURNING id`, kind).Scan(&id)
	return id, err
}

func setStatus(ctx context.Context, q db.Querier, id int64, status string) error {
	_, err := q.Exec(ctx, `UPDATE run SET status = $2, finished_at = now() WHERE id = $1`, id, status)
	return err
}

func insertEvent(ctx context.Context, q db.Querier, runID int64, ev web.TraceEvent) error {
	_, err := q.Exec(ctx, `
		INSERT INTO trace_event (run_id, seq, at, kind, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		runID, ev.Seq, ev.At, ev.Kind, ev.Summary, ev.Detail)
	return err
}

func loadRun(ctx context.Context, q db.Querier, id int64) (web.Run, bool, error) {
	run := web.Run{ID: id}
	err := q.QueryRow(ctx, `SELECT kind, status, started_at FROM run WHERE id = $1`, id).
		Scan(&run.Kind, &run.Status, &run.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return web.Run{}, false, nil
	}
	if err != nil {
		return web.Run{}, false, err
	}
	return run, true, nil
}

func loadEvents(ctx context.Context, q db.Querier, id int64, after int) ([]web.TraceEvent, error) {
	rows, _ := q.Query(ctx, `
		SELECT seq, at, kind, summary, detail FROM trace_event
		WHERE run_id = $1 AND seq > $2 ORDER BY seq`, id, after)
	return pgx.CollectRows(rows, pgx.RowToStructByPos[web.TraceEvent])
}

func listRuns(ctx context.Context, q db.Querier) ([]web.Run, error) {
	rows, _ := q.Query(ctx, `SELECT id, kind, status, started_at FROM run ORDER BY id DESC LIMIT 100`)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (web.Run, error) {
		var run web.Run
		err := row.Scan(&run.ID, &run.Kind, &run.Status, &run.StartedAt)
		return run, err
	})
}

// abandonRunning marks every running Run failed for reason, and ends its
// Trace with an error and a finish.
func abandonRunning(ctx context.Context, q db.Querier, reason string) error {
	_, err := q.Exec(ctx, `
		WITH abandoned AS (
			UPDATE run SET status = 'failed', finished_at = now()
			WHERE status = 'running'
			RETURNING id
		)
		INSERT INTO trace_event (run_id, seq, at, kind, summary, detail)
		SELECT id, coalesce((SELECT max(seq) FROM trace_event WHERE run_id = abandoned.id), 0) + e.n,
		       now(), e.kind, e.summary, e.detail
		FROM abandoned, (VALUES (1, 'error', 'Run failed', $1), (2, 'finished', 'Run failed', '')) AS e (n, kind, summary, detail)`,
		reason)
	return err
}
