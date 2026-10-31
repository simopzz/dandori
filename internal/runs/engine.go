// Package runs carries out agent Runs in the background: it records each Run
// and its Trace, streams the Trace live, and serves the Runs pages. Features
// start their kinds of Run through the Engine.
package runs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/db"
)

// Config is how Runs use the model.
type Config struct {
	// LLM answers every Run whose job brings no LLM of its own.
	LLM   agent.LLM
	Model string
	// Retries is how many times a failed request to the model is retried
	// when the failure is worth retrying; Backoff is the first wait.
	Retries int
	Backoff time.Duration
	// MaxTurns bounds the requests to the model in one Run.
	MaxTurns int
}

// NewJob builds the agent job of a Run from the form that started it.
type NewJob func(form url.Values) (agent.Job, error)

// Engine runs the Runs going on in this process, and knows the jobs that
// POST /runs can start.
type Engine struct {
	pool *pgxpool.Pool
	cfg  Config
	log  *slog.Logger

	ctx    context.Context // parent of every Run's context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu   sync.Mutex
	jobs map[string]NewJob
	live map[int64]*liveRun
}

// New creates an Engine with no Runs going on.
func New(pool *pgxpool.Pool, cfg Config, log *slog.Logger) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		pool: pool, cfg: cfg, log: log,
		ctx: ctx, cancel: cancel,
		jobs: map[string]NewJob{}, live: map[int64]*liveRun{},
	}
}

// Register makes Runs of kind startable with POST /runs.
func (e *Engine) Register(kind string, newJob NewJob) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jobs[kind] = newJob
}

func (e *Engine) job(kind string) (NewJob, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	newJob, ok := e.jobs[kind]
	return newJob, ok
}

// Stop cancels every Run and waits for them to record how they ended.
func (e *Engine) Stop() {
	e.mu.Lock()
	e.cancel()
	e.mu.Unlock()
	e.wg.Wait()
}

func (e *Engine) add(r *liveRun) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ctx.Err() != nil {
		return errors.New("Dandori is shutting down") //nolint:staticcheck // product name shown to the user
	}
	e.live[r.id] = r
	e.wg.Add(1)
	return nil
}

func (e *Engine) get(id int64) *liveRun {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.live[id]
}

func (e *Engine) remove(id int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.live, id)
}

// Live reports whether the Run is still going on in this process.
func (e *Engine) Live(id int64) bool { return e.get(id) != nil }

// Cancel cancels the Run if it is going on, reporting whether it was.
func (e *Engine) Cancel(id int64) bool {
	r := e.get(id)
	if r == nil {
		return false
	}
	r.cancel()
	return true
}

// CancelIn records the Runs with ids still running as cancelled because
// of cause, as part of q, so they stop as part of what q does: a Run
// recorded cancelled stores nothing more. Once q commits, Cancel stops
// them.
func (e *Engine) CancelIn(ctx context.Context, q db.Querier, ids []int64, cause string) error {
	return setCancelled(ctx, q, ids, cause)
}

// Get loads the Run with id, without its Trace.
func (e *Engine) Get(ctx context.Context, id int64) (Run, bool, error) {
	return loadRun(ctx, e.pool, id)
}

// Trace loads the Run's Trace Events after the one numbered after.
func (e *Engine) Trace(ctx context.Context, id int64, after int) ([]TraceEvent, error) {
	return loadEvents(ctx, e.pool, id, after)
}

// Latest loads the newest Run of any of kinds, with its Trace. found is
// false when there is none.
func (e *Engine) Latest(ctx context.Context, kinds ...string) (run Run, found bool, err error) {
	id, found, err := latestRun(ctx, e.pool, kinds)
	if err != nil || !found {
		return Run{}, false, err
	}
	if run, found, err = e.Get(ctx, id); err != nil || !found {
		return Run{}, false, err
	}
	run.Events, err = e.Trace(ctx, id, 0)
	return run, err == nil, err
}

// List loads the latest Runs, newest first.
func (e *Engine) List(ctx context.Context) ([]Run, error) {
	return listRuns(ctx, e.pool)
}

// Start records a new Run of kind and runs job in the background.
func (e *Engine) Start(ctx context.Context, kind string, job agent.Job) (int64, error) {
	id, err := insertRun(ctx, e.pool, kind)
	if err != nil {
		return 0, err
	}
	runCtx, cancel := context.WithCancel(e.ctx)
	r := &liveRun{id: id, cancel: cancel, q: e.pool, log: e.log, subs: map[chan TraceEvent]struct{}{}}
	if err := e.add(r); err != nil {
		cancel()
		// Never started, so it must not be left looking like it is running.
		if uerr := setStatus(ctx, e.pool, id, "failed"); uerr != nil {
			e.log.Error("store run status", "run", id, "err", uerr)
		}
		return 0, err
	}
	go e.execute(runCtx, r, kind, job)
	return id, nil
}

func (e *Engine) execute(ctx context.Context, r *liveRun, kind string, job agent.Job) {
	defer e.wg.Done()
	defer r.cancel()

	model := e.cfg.Model
	if job.LLM != nil {
		model = "a scripted model"
	}
	r.Emit(agent.Event{Kind: agent.EventStarted, Summary: fmt.Sprintf("%s run with %s", kind, model)})
	err := e.runJob(context.WithValue(ctx, idKey{}, r.id), r, job)

	status := "succeeded"
	switch {
	case e.ctx.Err() != nil:
		// Not the user's doing: recorded like a Run found interrupted.
		status = "failed"
		r.Emit(agent.Event{Kind: agent.EventError, Summary: "Run failed", Detail: interrupted})
	case ctx.Err() != nil:
		status = "cancelled"
	case err != nil:
		status = "failed"
		r.Emit(agent.Event{Kind: agent.EventError, Summary: "Run failed", Detail: err.Error()})
	}
	r.Emit(agent.Event{Kind: agent.EventFinished, Summary: "Run " + status})

	storeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := setStatus(storeCtx, e.pool, r.id, status); err != nil {
		e.log.Error("store run status", "run", r.id, "err", err)
	}
	e.remove(r.id)
	r.finish()
}

// idKey carries the id of the Run a job's tools are running in, for tools
// that store their result with the Run.
type idKey struct{}

// IDFrom is the id of the Run a job's tool is running in.
func IDFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(idKey{}).(int64)
	return id
}

// runJob carries out job with the agent loop. A panic, say in a tool, fails
// the Run rather than Dandori.
func (e *Engine) runJob(ctx context.Context, tr agent.Trace, job agent.Job) (err error) {
	defer func() {
		if p := recover(); p != nil {
			e.log.Error("run panicked", "panic", p, "stack", string(debug.Stack()))
			err = fmt.Errorf("Dandori crashed while running it: %v", p) //nolint:staticcheck // product name shown to the user
		}
	}()
	llm := e.cfg.LLM
	if job.LLM != nil {
		llm = job.LLM
	}
	if llm == nil {
		return errors.New("no LLM is configured")
	}
	loop := agent.Loop{
		LLM:      llm,
		Model:    e.cfg.Model,
		Retries:  e.cfg.Retries,
		Backoff:  e.cfg.Backoff,
		MaxTurns: e.cfg.MaxTurns,
	}
	return loop.Run(ctx, job, tr)
}

// interrupted is why a Run fails when Dandori stops under it.
const interrupted = "Dandori stopped while it was running"

// AbandonInterrupted marks Runs left running by a previous process as
// failed, since nothing is running them any more. Their Traces end as if
// the Run had been stopped by a shutdown.
func (e *Engine) AbandonInterrupted(ctx context.Context) error {
	if err := abandonRunning(ctx, e.pool, interrupted); err != nil {
		return fmt.Errorf("abandon interrupted runs: %w", err)
	}
	return nil
}
