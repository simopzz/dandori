// Package app wires Dandori together: the database and the HTTP handlers.
// Tests start it in-process through internal/apptest.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/web"
)

// Config is what the app needs from its environment.
type Config struct {
	DatabaseURL string
}

// App is a running Dandori instance.
type App struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// New creates the app once its database is reachable and migrated. It
// fails rather than serve on a database it can't use: Docker Compose
// restarts it, and a deploy that never comes up healthy fails.
func New(ctx context.Context, cfg Config, log *slog.Logger) (*App, error) {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a := &App{pool: pool, log: log}
	if err := a.ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	migrateCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := db.Migrate(migrateCtx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return a, nil
}

// Close releases the database connections.
func (a *App) Close() { a.pool.Close() }

// Handler returns the app's HTTP routes.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	web.Assets(mux)
	mux.HandleFunc("GET /healthz", a.health)
	mux.Handle("GET /{$}", templ.Handler(web.Home()))
	return mux
}

func (a *App) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := a.pool.Ping(ctx); err != nil {
		return fmt.Errorf("database unreachable: %w", err)
	}
	return nil
}

// health answers whether the app can reach its database, for the
// container's health check.
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	status, code := "ok", http.StatusOK
	if err := a.ping(r.Context()); err != nil {
		a.log.Warn("health check", "err", err)
		status, code = "unavailable", http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"database": status})
}
