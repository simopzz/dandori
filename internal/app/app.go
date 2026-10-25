// Package app wires Dandori together: the database, the outbound ports and
// the HTTP handlers. Tests start it in-process through internal/apptest.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/demo"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/profile"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
	"github.com/simopzz/dandori/internal/web"
)

// Config is what the app needs from its environment. Zero values take the
// defaults below.
type Config struct {
	DatabaseURL string

	// Model is the Claude model agent Runs use.
	Model string
	// LLMRetries is how many times a failed request to the model is retried
	// when the failure is worth retrying; LLMRetryBackoff is the first wait.
	LLMRetries      int
	LLMRetryBackoff time.Duration
	// MaxTurns bounds the requests to the model in one Run.
	MaxTurns int

	// Demo offers demo Runs on the Runs page, for trying the agent by hand.
	Demo bool
}

// DefaultModel is the latest Claude model.
const DefaultModel = "claude-opus-5-5"

func (c Config) withDefaults() Config {
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if c.LLMRetries == 0 {
		c.LLMRetries = 3
	}
	if c.LLMRetryBackoff == 0 {
		c.LLMRetryBackoff = 2 * time.Second
	}
	if c.MaxTurns == 0 {
		c.MaxTurns = 50
	}
	return c
}

// Ports are the outbound dependencies that talk to the outside world. Tests
// replace them with fakes. The store and embeddings ports are added by the
// slices that need them.
type Ports struct {
	LLM agent.LLM
}

// App is a running Dandori instance.
type App struct {
	pool *pgxpool.Pool
	cfg  Config
	log  *slog.Logger

	mu       sync.Mutex
	migrated bool

	runs    *runs.Engine
	recipes *recipes.Service
	profile *profile.Service
}

// New creates the app. It does not connect to the database: a paused or
// unreachable database is reported by the app rather than stopping it.
func New(ctx context.Context, cfg Config, ports Ports, log *slog.Logger) (*App, error) {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a := &App{pool: pool, cfg: cfg.withDefaults(), log: log}
	a.runs = runs.New(pool, runs.Config{
		LLM:      ports.LLM,
		Model:    a.cfg.Model,
		Retries:  a.cfg.LLMRetries,
		Backoff:  a.cfg.LLMRetryBackoff,
		MaxTurns: a.cfg.MaxTurns,
	}, log)
	a.recipes = recipes.New(pool, a.runs)
	a.profile = profile.New(pool)
	if cfg.Demo {
		a.RegisterJob(demo.Kind, demo.NewJob)
	}
	return a, nil
}

// Close cancels the Runs still going, waits for them to stop, and releases
// the database connections.
func (a *App) Close() {
	a.runs.Stop()
	a.pool.Close()
}

// ErrDatabaseUnreachable means the database is paused or can't be reached.
var ErrDatabaseUnreachable = errors.New("database unreachable")

// EnsureDatabase checks the database is reachable and, the first time it is,
// applies pending migrations.
func (a *App) EnsureDatabase(ctx context.Context) error {
	if err := a.ping(ctx); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.migrated {
		return nil
	}
	migrateCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := db.Migrate(migrateCtx, a.pool); err != nil {
		// The database may have dropped mid-migration; that is a pause, not a bug.
		if pingErr := a.ping(ctx); pingErr != nil {
			return pingErr
		}
		return err
	}
	if err := a.runs.AbandonInterrupted(ctx); err != nil {
		return err
	}
	a.migrated = true
	return nil
}

func (a *App) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := a.pool.Ping(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrDatabaseUnreachable, err)
	}
	return nil
}

// Handler returns the app's HTTP routes.
func (a *App) Handler() http.Handler {
	runs := runs.Handlers{Engine: a.runs, Log: a.log, Demo: a.cfg.Demo}
	recipes := recipes.Handlers{Service: a.recipes, Log: a.log}
	profile := profile.Handlers{Service: a.profile, Log: a.log}
	mux := http.NewServeMux()
	web.Assets(mux)
	mux.HandleFunc("GET /healthz", a.health)
	mux.Handle("GET /{$}", a.needsDatabase(templ.Handler(web.Home())))
	mux.Handle("GET /profile", a.needsDatabase(http.HandlerFunc(profile.Show)))
	mux.Handle("POST /profile", a.needsDatabase(http.HandlerFunc(profile.Save)))
	mux.Handle("GET /recipes", a.needsDatabase(http.HandlerFunc(recipes.Library)))
	mux.Handle("POST /recipes/parse", a.needsDatabase(http.HandlerFunc(recipes.Parse)))
	mux.Handle("GET /recipe-drafts/{id}", a.needsDatabase(http.HandlerFunc(recipes.ShowDraft)))
	mux.Handle("POST /recipe-drafts/{id}", a.needsDatabase(http.HandlerFunc(recipes.AcceptDraft)))
	mux.Handle("POST /recipe-drafts/{id}/discard", a.needsDatabase(http.HandlerFunc(recipes.DiscardDraft)))
	mux.Handle("GET /recipes/{id}", a.needsDatabase(http.HandlerFunc(recipes.Show)))
	mux.Handle("POST /recipes/{id}", a.needsDatabase(http.HandlerFunc(recipes.Update)))
	mux.Handle("GET /recipes/{id}/edit", a.needsDatabase(http.HandlerFunc(recipes.Edit)))
	mux.Handle("POST /recipes/{id}/delete", a.needsDatabase(http.HandlerFunc(recipes.Delete)))
	mux.Handle("GET /runs", a.needsDatabase(http.HandlerFunc(runs.List)))
	mux.Handle("POST /runs", a.needsDatabase(http.HandlerFunc(runs.Start)))
	mux.Handle("GET /runs/{id}", a.needsDatabase(http.HandlerFunc(runs.Show)))
	mux.Handle("GET /runs/{id}/events", a.needsDatabase(http.HandlerFunc(runs.Stream)))
	mux.Handle("POST /runs/{id}/cancel", a.needsDatabase(http.HandlerFunc(runs.Cancel)))
	return mux
}

// needsDatabase serves next only once the database is ready; while it can't
// be reached, it shows the recoverable database-unavailable page instead.
func (a *App) needsDatabase(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := a.EnsureDatabase(r.Context())
		switch {
		case errors.Is(err, ErrDatabaseUnreachable):
			a.log.Warn("database unavailable", "err", err)
			if httpx.IsHTMX(r) {
				// Reload into the error page rather than swap it into a fragment.
				w.Header().Set("HX-Refresh", "true")
			}
			templ.Handler(web.DatabaseUnavailable(), templ.WithStatus(http.StatusServiceUnavailable)).ServeHTTP(w, r)
			return
		case err != nil:
			// Reachable but not usable, e.g. a failing migration: a bug, not a pause.
			a.log.Error("database not ready", "err", err)
			http.Error(w, "Dandori could not prepare its database; see the server log.", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RegisterJob makes Runs of kind startable with POST /runs.
func (a *App) RegisterJob(kind string, newJob runs.NewJob) { a.runs.Register(kind, newJob) }

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	status, code := "ok", http.StatusOK
	if err := a.ping(r.Context()); err != nil {
		status, code = "unavailable", http.StatusServiceUnavailable
	} else if httpx.IsHTMX(r) {
		// The database-unavailable page polls this; reload it once we're back.
		w.Header().Set("HX-Refresh", "true")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"database": status})
}
