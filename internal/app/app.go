// Package app wires Dandori together: the database and the HTTP handlers.
// Tests start it in-process through internal/apptest.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/auth"
	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/profile"
	"github.com/simopzz/dandori/internal/web"
)

// Config is what the app needs from its environment.
type Config struct {
	DatabaseURL string
	// Clock is the only time the app reads: the real one in production,
	// one a test sets and steps in apptest.
	Clock clock.Clock
	// Auth is the Supabase Auth Users sign in through.
	Auth auth.Config
}

// App is a running Dandori instance.
type App struct {
	pool    *pgxpool.Pool
	clock   clock.Clock
	log     *slog.Logger
	auth    *auth.Service
	profile *profile.Service
}

// New creates the app once its database is reachable and migrated. It
// fails rather than serve on a database it can't use: Docker Compose
// restarts it, and a deploy that never comes up healthy fails.
func New(ctx context.Context, cfg Config, log *slog.Logger) (*App, error) {
	if cfg.Clock == nil {
		return nil, errors.New("app.Config has no Clock")
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a := &App{pool: pool, clock: cfg.Clock, log: log, auth: auth.New(pool, cfg.Auth, cfg.Clock), profile: profile.New(pool)}
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

// Handler returns the app's HTTP routes. Every page speaks the signed-in
// User's Language, and asks them for it before anything else; pages
// before signing in speak the one they were switched to.
func (a *App) Handler() http.Handler {
	signIn := auth.Handlers{Service: a.auth, Clock: a.clock, Log: a.log}
	prof := profile.Handlers{Service: a.profile, Clock: a.clock, Log: a.log, Email: a.auth.Email}
	choosing := func(h http.HandlerFunc) http.Handler { return signIn.SignedIn(h) }
	signedIn := func(h http.HandlerFunc) http.Handler { return signIn.SignedIn(prof.LanguageChosen(h)) }
	mux := http.NewServeMux()
	web.Assets(mux)
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /sign-in", signIn.ShowSignIn)
	mux.HandleFunc("POST /sign-in", signIn.SendLink)
	mux.HandleFunc("GET /sign-in/sent", signIn.ShowSent)
	mux.HandleFunc("GET /auth/confirm", signIn.ShowConfirm)
	mux.HandleFunc("POST /auth/confirm", signIn.Confirm)
	mux.HandleFunc("POST /sign-out", signIn.SignOut)
	mux.Handle("GET /language", choosing(prof.ShowChoice))
	mux.Handle("POST /language", choosing(prof.Choose))
	mux.Handle("GET /{$}", signedIn(a.home))
	mux.Handle("GET /profile", signedIn(prof.Show))
	mux.Handle("POST /profile/language", signedIn(prof.ChangeLanguage))
	return web.Languages(mux)
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

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	if err := web.Home(a.clock.Now()).Render(r.Context(), w); err != nil {
		a.log.Error("render Home", "err", err)
	}
}
