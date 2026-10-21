// Package app wires MealOps together: the database, the outbound ports and
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

	"github.com/simopzz/mealops/internal/db"
	"github.com/simopzz/mealops/internal/web"
)

// Config is what the app needs from its environment.
type Config struct {
	DatabaseURL string
}

// Ports are the outbound dependencies that talk to the outside world. Tests
// replace them with fakes. The LLM, store and embeddings ports are added by
// the slices that need them.
type Ports struct{}

// App is a running MealOps instance.
type App struct {
	pool  *pgxpool.Pool
	ports Ports
	log   *slog.Logger

	mu       sync.Mutex
	migrated bool
}

// New creates the app. It does not connect to the database: a paused or
// unreachable database is reported by the app rather than stopping it.
func New(ctx context.Context, cfg Config, ports Ports, log *slog.Logger) (*App, error) {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	return &App{pool: pool, ports: ports, log: log}, nil
}

// Close releases the database connections.
func (a *App) Close() { a.pool.Close() }

// ErrDatabaseUnreachable means the database is paused or can't be reached.
var ErrDatabaseUnreachable = errors.New("database unreachable")

// EnsureDatabase checks the database is reachable and, the first time it is,
// applies pending migrations.
func (a *App) EnsureDatabase(ctx context.Context) error {
	if err := a.ping(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.migrated {
		return nil
	}
	if err := db.Migrate(ctx, a.pool); err != nil {
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
	return mux
}

// needsDatabase shows the recoverable database-unavailable page instead of
// next while the database can't be reached.
func (a *App) needsDatabase(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := a.EnsureDatabase(r.Context())
		switch {
		case errors.Is(err, ErrDatabaseUnreachable):
			a.log.Warn("database unavailable", "err", err)
			templ.Handler(web.DatabaseUnavailable(), templ.WithStatus(http.StatusServiceUnavailable)).ServeHTTP(w, r)
			return
		case err != nil:
			// Reachable but not usable, e.g. a failing migration: a bug, not a pause.
			a.log.Error("database not ready", "err", err)
			http.Error(w, "MealOps could not prepare its database; see the server log.", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	status, code := "ok", http.StatusOK
	if err := a.ping(r.Context()); err != nil {
		status, code = "unavailable", http.StatusServiceUnavailable
	} else if r.Header.Get("HX-Request") == "true" {
		// The database-unavailable page polls this; reload it once we're back.
		w.Header().Set("HX-Refresh", "true")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"database": status})
}
