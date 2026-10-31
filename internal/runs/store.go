package runs

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
)

func insertRun(ctx context.Context, q db.Querier, kind string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO run (kind) VALUES ($1) RETURNING id`, kind).Scan(&id)
	return id, err
}

// setStatus stores how the running Run with id ended, unless it was
// already recorded cancelled.
func setStatus(ctx context.Context, q db.Querier, id int64, status string) error {
	_, err := q.Exec(ctx, `UPDATE run SET status = $2, finished_at = now() WHERE id = $1 AND status = 'running'`, id, status)
	return err
}

// setCancelled records the Runs with ids still running as cancelled
// because of cause.
func setCancelled(ctx context.Context, q db.Querier, ids []int64, cause string) error {
	_, err := q.Exec(ctx, `
		UPDATE run SET status = 'cancelled', cancelled_because = $2, finished_at = now()
		WHERE id = ANY($1) AND status = 'running'`, ids, cause)
	return err
}

func insertEvent(ctx context.Context, q db.Querier, runID int64, ev TraceEvent) error {
	_, err := q.Exec(ctx, `
		INSERT INTO trace_event (run_id, seq, at, kind, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		runID, ev.Seq, ev.At, ev.Kind, ev.Summary, ev.Detail)
	return err
}

func loadRun(ctx context.Context, q db.Querier, id int64) (Run, bool, error) {
	run := Run{ID: id}
	err := q.QueryRow(ctx, `SELECT kind, status, cancelled_because, started_at FROM run WHERE id = $1`, id).
		Scan(&run.Kind, &run.Status, &run.CancelledBecause, &run.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	return run, true, nil
}

// latestRun is the id of the newest Run of any of kinds.
func latestRun(ctx context.Context, q db.Querier, kinds []string) (id int64, found bool, err error) {
	err = q.QueryRow(ctx, `SELECT id FROM run WHERE kind = ANY($1) ORDER BY id DESC LIMIT 1`, kinds).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

func loadEvents(ctx context.Context, q db.Querier, id int64, after int) ([]TraceEvent, error) {
	rows, _ := q.Query(ctx, `
		SELECT seq, at, kind, summary, detail FROM trace_event
		WHERE run_id = $1 AND seq > $2 ORDER BY seq`, id, after)
	return pgx.CollectRows(rows, pgx.RowToStructByPos[TraceEvent])
}

func listRuns(ctx context.Context, q db.Querier) ([]Run, error) {
	rows, _ := q.Query(ctx, `SELECT id, kind, status, started_at FROM run ORDER BY id DESC LIMIT 100`)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Run, error) {
		var run Run
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
