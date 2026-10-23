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
	pool  *pgxpool.Pool
	cfg   Config
	ports Ports
	log   *slog.Logger

	mu       sync.Mutex
	migrated bool

	runs *runs
}

// New creates the app. It does not connect to the database: a paused or
// unreachable database is reported by the app rather than stopping it.
func New(ctx context.Context, cfg Config, ports Ports, log *slog.Logger) (*App, error) {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a := &App{pool: pool, cfg: cfg.withDefaults(), ports: ports, log: log}
	a.runs = newRuns()
	if cfg.Demo {
		a.RegisterJob(demo.Kind, demo.NewJob)
	}
	return a, nil
}

// Close cancels the Runs still going, waits for them to stop, and releases
// the database connections.
func (a *App) Close() {
	a.runs.stop()
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
	if err := a.abandonInterruptedRuns(ctx); err != nil {
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
	mux := http.NewServeMux()
	web.Assets(mux)
	mux.HandleFunc("GET /healthz", a.health)
	mux.Handle("GET /{$}", a.needsDatabase(templ.Handler(web.Home())))
	mux.Handle("GET /profile", a.needsDatabase(http.HandlerFunc(a.showProfile)))
	mux.Handle("POST /profile", a.needsDatabase(http.HandlerFunc(a.saveProfile)))
	mux.Handle("GET /runs", a.needsDatabase(http.HandlerFunc(a.listRuns)))
	mux.Handle("POST /runs", a.needsDatabase(http.HandlerFunc(a.startRunHandler)))
	mux.Handle("GET /runs/{id}", a.needsDatabase(http.HandlerFunc(a.showRun)))
	mux.Handle("GET /runs/{id}/events", a.needsDatabase(http.HandlerFunc(a.streamRun)))
	mux.Handle("POST /runs/{id}/cancel", a.needsDatabase(http.HandlerFunc(a.cancelRun)))
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
			if isHTMX(r) {
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

func (a *App) serverError(w http.ResponseWriter, what string, err error) {
	a.log.Error(what, "err", err)
	http.Error(w, "Something went wrong; see the server log.", http.StatusInternalServerError)
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	status, code := "ok", http.StatusOK
	if err := a.ping(r.Context()); err != nil {
		status, code = "unavailable", http.StatusServiceUnavailable
	} else if isHTMX(r) {
		// The database-unavailable page polls this; reload it once we're back.
		w.Header().Set("HX-Refresh", "true")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"database": status})
}
