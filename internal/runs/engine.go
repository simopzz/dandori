// Package runs carries out agent Runs in the background, each as the User it belongs
// to, records each Run and its Trace, and serves a Run's page. Features start their
// kinds of Run through the Engine.
package runs

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/db"
)

// Config is how Runs use the model.
type Config struct {
	// LLM answers every Run; without one, every Run fails for agent.NoModel.
	LLM   agent.LLM
	Model string
	// Retries is how many times a failed request to the model is retried when the
	// failure is worth retrying; Backoff is the first wait.
	Retries int
	Backoff time.Duration
	// MaxTurns bounds the requests to the model in one Run.
	MaxTurns int
}

// Status is where a Run is: going until it succeeds, fails or is cancelled.
type Status string

// The statuses a Run has, as the database checks them.
const (
	Going     Status = "going"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Cancelled Status = "cancelled"
)

// Engine runs the Runs going in this process.
type Engine struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	cfg   Config
	log   *slog.Logger

	mu     sync.Mutex
	ctx    context.Context // parent of every Run's context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DefaultModel is the model every Run uses unless Config names another.
const DefaultModel = "claude-opus-5-5"

// New creates an Engine with no Runs going, filling in what cfg leaves unset.
func New(pool *pgxpool.Pool, c clock.Clock, cfg Config, log *slog.Logger) *Engine {
	cfg.Model = cmp.Or(cfg.Model, DefaultModel)
	cfg.Retries = cmp.Or(cfg.Retries, 3)
	cfg.Backoff = cmp.Or(cfg.Backoff, 2*time.Second)
	cfg.MaxTurns = cmp.Or(cfg.MaxTurns, 50)
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{pool: pool, clock: c, cfg: cfg, log: log, ctx: ctx, cancel: cancel}
}

// Stop cancels every Run and waits for them to record how they ended.
func (e *Engine) Stop() {
	e.mu.Lock()
	e.cancel()
	e.mu.Unlock()
	e.wg.Wait()
}

// Wait waits for every Run going to end, for a test that reads what it left.
func (e *Engine) Wait() { e.wg.Wait() }

// Start records a new Run of kind for the signed-in User, asked asked, and carries out b
// in the background as that User. The tools of b find the Run's id with IDFrom.
func (e *Engine) Start(ctx context.Context, kind, asked string, b agent.Brief) (int64, error) {
	userID, ok := db.UserID(ctx)
	if !ok {
		return 0, errors.New("runs.Start: no signed-in User")
	}
	var id int64
	err := db.AsUser(ctx, e.pool, func(tx pgx.Tx) (err error) {
		id, err = insertRun(ctx, tx, kind, asked, e.clock.Now())
		return err
	})
	if err != nil {
		return 0, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ctx.Err() != nil {
		return 0, errors.New("Dandori is shutting down") //nolint:staticcheck // the product's name
	}
	runCtx := context.WithValue(db.WithUser(e.ctx, userID), idKey{}, id)
	e.wg.Add(1)
	go e.execute(runCtx, &trace{e: e, userID: userID, runID: id}, b)
	return id, nil
}

func (e *Engine) execute(ctx context.Context, tr *trace, b agent.Brief) {
	defer e.wg.Done()
	tr.Emit(agent.TraceEvent{Kind: agent.Started, Model: e.cfg.Model})
	err := e.carryOut(ctx, tr, b)

	status := Succeeded
	if err != nil {
		status = Failed
		failure, ok := errors.AsType[*agent.Failure](err)
		switch {
		case e.ctx.Err() != nil:
			failure = &agent.Failure{Reason: agent.Interrupted}
		case !ok:
			failure = &agent.Failure{Reason: agent.Crashed, Detail: err.Error()}
		}
		tr.Emit(agent.TraceEvent{Kind: agent.Failed, Reason: failure.Reason, Text: failure.Detail})
	}
	tr.Emit(agent.TraceEvent{Kind: agent.Ended, Status: string(status)})

	storeCtx, cancel := tr.storeContext()
	defer cancel()
	err = db.AsUser(storeCtx, e.pool, func(tx pgx.Tx) error {
		return setStatus(storeCtx, tx, tr.runID, status, e.clock.Now())
	})
	if err != nil {
		e.log.Error("store the Run's status", "run", tr.runID, "err", err)
	}
}

// carryOut carries out b with the agent loop. A panic, say in a tool, fails the Run
// rather than Dandori.
func (e *Engine) carryOut(ctx context.Context, tr agent.Trace, b agent.Brief) (err error) {
	defer func() {
		if p := recover(); p != nil {
			e.log.Error("a Run panicked", "panic", p, "stack", string(debug.Stack()))
			err = &agent.Failure{Reason: agent.Crashed, Detail: fmt.Sprint(p)}
		}
	}()
	if e.cfg.LLM == nil {
		return &agent.Failure{Reason: agent.NoModel}
	}
	loop := agent.Loop{LLM: e.cfg.LLM, Model: e.cfg.Model, Retries: e.cfg.Retries, Backoff: e.cfg.Backoff, MaxTurns: e.cfg.MaxTurns}
	return loop.Run(ctx, b, tr)
}

// idKey carries the id of the Run a Brief's tools are running in.
type idKey struct{}

// IDFrom is the id of the Run a Brief's tool is running in.
func IDFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(idKey{}).(int64)
	return id
}

// Find is the signed-in User's Run id with its Trace, found false when they have none.
func (e *Engine) Find(ctx context.Context, id int64) (run Run, found bool, err error) {
	err = db.AsUser(ctx, e.pool, func(tx pgx.Tx) (err error) {
		run, found, err = loadRun(ctx, tx, id)
		return err
	})
	return run, found, err
}

// trace is a Run's Trace: each Trace Event it is given is stored with the Run, in order,
// as the Run's User.
type trace struct {
	e      *Engine
	userID string
	runID  int64

	mu  sync.Mutex
	seq int
}

// Emit implements agent.Trace.
func (t *trace) Emit(ev agent.TraceEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	ctx, cancel := t.storeContext()
	defer cancel()
	err := db.AsUser(ctx, t.e.pool, func(tx pgx.Tx) error {
		return insertTraceEvent(ctx, tx, t.runID, t.seq, t.e.clock.Now(), ev)
	})
	if err != nil {
		t.e.log.Error("store a Trace Event", "run", t.runID, "seq", t.seq, "err", err)
	}
}

// storeContext is the Run's User's, but not the Run's: a cancelled Run still records how
// it ended.
func (t *trace) storeContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(db.WithUser(context.Background(), t.userID), 10*time.Second)
}
