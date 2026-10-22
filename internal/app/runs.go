package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/mealops/internal/agent"
	"github.com/simopzz/mealops/internal/web"
)

// NewJob builds the agent job of a Run from the form that started it.
type NewJob func(form url.Values) (agent.Job, error)

// RegisterJob makes Runs of kind startable with POST /runs.
func (a *App) RegisterJob(kind string, newJob NewJob) {
	a.runs.mu.Lock()
	defer a.runs.mu.Unlock()
	a.runs.jobs[kind] = newJob
}

// runs are the Runs going on in this process, and the jobs they can run.
type runs struct {
	ctx    context.Context // parent of every Run's context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu   sync.Mutex
	jobs map[string]NewJob
	live map[int64]*liveRun
}

func newRuns() *runs {
	ctx, cancel := context.WithCancel(context.Background())
	return &runs{ctx: ctx, cancel: cancel, jobs: map[string]NewJob{}, live: map[int64]*liveRun{}}
}

// stop cancels every Run and waits for them to record how they ended.
func (rs *runs) stop() {
	rs.mu.Lock()
	rs.cancel()
	rs.mu.Unlock()
	rs.wg.Wait()
}

func (rs *runs) add(r *liveRun) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.ctx.Err() != nil {
		return errors.New("MealOps is shutting down")
	}
	rs.live[r.id] = r
	rs.wg.Add(1)
	return nil
}

func (rs *runs) get(id int64) *liveRun {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.live[id]
}

func (rs *runs) remove(id int64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	delete(rs.live, id)
}

// liveRun is a Run in progress. It is the Run's trace sink: every Trace
// Event is stored with the Run and fanned out to the live streams.
type liveRun struct {
	id     int64
	cancel context.CancelFunc
	pool   *pgxpool.Pool
	log    *slog.Logger

	mu   sync.Mutex
	seq  int
	subs map[chan web.TraceEvent]struct{}
	done bool
}

// Emit implements agent.Trace.
func (r *liveRun) Emit(e agent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	ev := web.TraceEvent{
		Seq: r.seq,
		// Postgres keeps microseconds; so does the live copy.
		At:      time.Now().Truncate(time.Microsecond),
		Kind:    string(e.Kind),
		Summary: e.Summary,
		Detail:  e.Detail,
	}
	// Not the Run's context: a cancelled Run still records how it ended.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO trace_event (run_id, seq, at, kind, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		r.id, ev.Seq, ev.At, ev.Kind, ev.Summary, ev.Detail,
	); err != nil {
		r.log.Error("store trace event", "run", r.id, "seq", ev.Seq, "err", err)
	}
	for ch := range r.subs {
		select {
		case ch <- ev:
		default:
			// Too slow to keep up: hang up, and let the browser reconnect
			// and catch up from the stored trace.
			delete(r.subs, ch)
			close(ch)
		}
	}
}

// subscribe follows the Run's Trace Events from now on. It returns false
// once the Run has ended.
func (r *liveRun) subscribe() (chan web.TraceEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return nil, false
	}
	ch := make(chan web.TraceEvent, 64)
	r.subs[ch] = struct{}{}
	return ch, true
}

func (r *liveRun) unsubscribe(ch chan web.TraceEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.subs[ch]; ok {
		delete(r.subs, ch)
		close(ch)
	}
}

func (r *liveRun) ended() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done
}

// finish ends every live stream of the Run.
func (r *liveRun) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = true
	for ch := range r.subs {
		delete(r.subs, ch)
		close(ch)
	}
}

// startRun records a new Run of kind and runs job in the background.
func (a *App) startRun(ctx context.Context, kind string, job agent.Job) (int64, error) {
	var id int64
	if err := a.pool.QueryRow(ctx, `INSERT INTO run (kind) VALUES ($1) RETURNING id`, kind).Scan(&id); err != nil {
		return 0, err
	}
	runCtx, cancel := context.WithCancel(a.runs.ctx)
	r := &liveRun{id: id, cancel: cancel, pool: a.pool, log: a.log, subs: map[chan web.TraceEvent]struct{}{}}
	if err := a.runs.add(r); err != nil {
		cancel()
		return 0, err
	}
	go a.execute(runCtx, r, kind, job)
	return id, nil
}

func (a *App) execute(ctx context.Context, r *liveRun, kind string, job agent.Job) {
	defer a.runs.wg.Done()
	defer r.cancel()

	r.Emit(agent.Event{Kind: agent.EventStarted, Summary: fmt.Sprintf("%s run with %s", kind, a.cfg.Model)})
	var err error
	if a.ports.LLM == nil {
		err = errors.New("no LLM is configured")
	} else {
		loop := agent.Loop{
			LLM:      a.ports.LLM,
			Model:    a.cfg.Model,
			Retries:  a.cfg.LLMRetries,
			Backoff:  a.cfg.LLMRetryBackoff,
			MaxTurns: a.cfg.MaxTurns,
		}
		err = loop.Run(ctx, job, r)
	}

	status := "succeeded"
	switch {
	case a.runs.ctx.Err() != nil:
		// Not the user's doing: recorded like a Run found interrupted.
		status = "failed"
		r.Emit(agent.Event{Kind: agent.EventError, Summary: "Run failed", Detail: "MealOps stopped while it was running"})
	case ctx.Err() != nil:
		status = "cancelled"
	case err != nil:
		status = "failed"
		r.Emit(agent.Event{Kind: agent.EventError, Summary: "Run failed", Detail: err.Error()})
	}
	r.Emit(agent.Event{Kind: agent.EventFinished, Summary: "Run " + status})

	storeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := a.pool.Exec(storeCtx, `UPDATE run SET status = $2, finished_at = now() WHERE id = $1`, r.id, status); err != nil {
		a.log.Error("store run status", "run", r.id, "err", err)
	}
	a.runs.remove(r.id)
	r.finish()
}

// abandonInterruptedRuns marks Runs left running by a previous process as
// failed, since nothing is running them any more.
func (a *App) abandonInterruptedRuns(ctx context.Context) error {
	_, err := a.pool.Exec(ctx, `
		WITH abandoned AS (
			UPDATE run SET status = 'failed', finished_at = now()
			WHERE status = 'running'
			RETURNING id
		)
		INSERT INTO trace_event (run_id, seq, at, kind, summary)
		SELECT id, coalesce((SELECT max(seq) FROM trace_event WHERE run_id = abandoned.id), 0) + 1,
		       now(), 'finished', 'Run failed: MealOps stopped while it was running'
		FROM abandoned`)
	if err != nil {
		return fmt.Errorf("abandon interrupted runs: %w", err)
	}
	return nil
}

func (a *App) startRunHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	kind := r.PostForm.Get("kind")
	a.runs.mu.Lock()
	newJob, ok := a.runs.jobs[kind]
	a.runs.mu.Unlock()
	if !ok {
		http.Error(w, fmt.Sprintf("There is no kind of Run called %q.", kind), http.StatusBadRequest)
		return
	}
	job, err := newJob(r.PostForm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := a.startRun(r.Context(), kind, job)
	if err != nil {
		a.serverError(w, "start run", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/runs/%d", id), http.StatusSeeOther)
}

func (a *App) listRuns(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.pool.Query(r.Context(), `SELECT id, kind, status, started_at FROM run ORDER BY id DESC LIMIT 100`)
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (web.Run, error) {
		var run web.Run
		err := row.Scan(&run.ID, &run.Kind, &run.Status, &run.StartedAt)
		return run, err
	})
	if err != nil {
		a.serverError(w, "list runs", err)
		return
	}
	templ.Handler(web.RunsPage(list)).ServeHTTP(w, r)
}

func (a *App) showRun(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	run, found, err := a.loadRun(r.Context(), id)
	if err != nil {
		a.serverError(w, "load run", err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	run.Events, err = a.loadEvents(r.Context(), id, 0)
	if err != nil {
		a.serverError(w, "load trace", err)
		return
	}
	templ.Handler(web.RunPage(run)).ServeHTTP(w, r)
}

func (a *App) cancelRun(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	if live := a.runs.get(id); live != nil {
		live.cancel()
		http.Redirect(w, r, fmt.Sprintf("/runs/%d", id), http.StatusSeeOther)
		return
	}
	_, found, err := a.loadRun(r.Context(), id)
	switch {
	case err != nil:
		a.serverError(w, "load run", err)
	case !found:
		http.NotFound(w, r)
	default:
		http.Error(w, "This Run has already ended.", http.StatusConflict)
	}
}

// streamRun sends a Run's Trace Events as server-sent events: the stored
// ones after the last the client has (Last-Event-ID, else ?after=), then
// live ones until the Run ends, then an "end" event with its status.
func (a *App) streamRun(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	run, found, err := a.loadRun(r.Context(), id)
	if err != nil {
		a.serverError(w, "load run", err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	if last := r.Header.Get("Last-Event-ID"); last != "" {
		after, _ = strconv.Atoi(last)
	}

	// Follow before reading the stored trace, so no event falls between.
	var live chan web.TraceEvent
	lr := a.runs.get(id)
	if lr != nil {
		if ch, ok := lr.subscribe(); ok {
			live = ch
			defer lr.unsubscribe(ch)
		}
	}
	stored, err := a.loadEvents(r.Context(), id, after)
	if err != nil {
		a.serverError(w, "load trace", err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher := http.NewResponseController(w)
	send := func(ev web.TraceEvent) error {
		var html bytes.Buffer
		if err := web.TraceEventItem(ev).Render(r.Context(), &html); err != nil {
			return err
		}
		writeEvent(w, strconv.Itoa(ev.Seq), "trace", html.String())
		after = ev.Seq
		return nil
	}
	for _, ev := range stored {
		if err := send(ev); err != nil {
			a.log.Error("render trace event", "err", err)
			return
		}
	}
	if err := flusher.Flush(); err != nil {
		return
	}

	for live != nil {
		select {
		case ev, open := <-live:
			if !open {
				// The Run ended, or we were dropped for being slow; either
				// way, catch up from the stored trace.
				ended := lr.ended()
				missed, err := a.loadEvents(r.Context(), id, after)
				if err != nil {
					a.log.Error("load trace", "err", err)
					return
				}
				for _, ev := range missed {
					if err := send(ev); err != nil {
						a.log.Error("render trace event", "err", err)
						return
					}
				}
				if !ended {
					flusher.Flush()
					return // the browser reconnects and follows on
				}
				live = nil
				continue
			}
			if ev.Seq <= after {
				continue
			}
			if err := send(ev); err != nil {
				a.log.Error("render trace event", "err", err)
				return
			}
			if err := flusher.Flush(); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
	// The Run has ended, so its status is final.
	run, _, err = a.loadRun(r.Context(), id)
	if err != nil {
		a.log.Error("load run", "err", err)
		return
	}
	writeEvent(w, "", "end", run.Status)
	flusher.Flush()
}

// writeEvent writes one server-sent event.
func writeEvent(w io.Writer, id, name, data string) {
	if id != "" {
		fmt.Fprintf(w, "id: %s\n", id)
	}
	fmt.Fprintf(w, "event: %s\n", name)
	data = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(data)
	for line := range strings.SplitSeq(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func runID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

func (a *App) loadRun(ctx context.Context, id int64) (web.Run, bool, error) {
	run := web.Run{ID: id}
	err := a.pool.QueryRow(ctx, `SELECT kind, status, started_at FROM run WHERE id = $1`, id).
		Scan(&run.Kind, &run.Status, &run.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return web.Run{}, false, nil
	}
	if err != nil {
		return web.Run{}, false, err
	}
	return run, true, nil
}

func (a *App) loadEvents(ctx context.Context, id int64, after int) ([]web.TraceEvent, error) {
	rows, _ := a.pool.Query(ctx, `
		SELECT seq, at, kind, summary, detail FROM trace_event
		WHERE run_id = $1 AND seq > $2 ORDER BY seq`, id, after)
	return pgx.CollectRows(rows, pgx.RowToStructByPos[web.TraceEvent])
}
