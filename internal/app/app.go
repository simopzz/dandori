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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/simopzz/dandori/internal/auth"
	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/kitchen"
	"github.com/simopzz/dandori/internal/profile"
	"github.com/simopzz/dandori/internal/today"
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
	// Spans makes the spans of every request, query, beat and call to Supabase
	// Auth; without it, none are made.
	Spans trace.TracerProvider
}

// App is a running Dandori instance.
type App struct {
	pool        *pgxpool.Pool
	clock       clock.Clock
	spans       trace.TracerProvider
	log         *slog.Logger
	auth        *auth.Service
	profile     *profile.Service
	ingredients *ingredients.Service
	kitchen     *kitchen.Service
	today       *today.Service
}

// New creates the app once its database is reachable and migrated. It
// fails rather than serve on a database it can't use: Docker Compose
// restarts it, and a deploy that never comes up healthy fails.
func New(ctx context.Context, cfg Config, log *slog.Logger) (*App, error) {
	if cfg.Clock == nil {
		return nil, errors.New("app.Config has no Clock")
	}
	spans := cfg.Spans
	if spans == nil {
		spans = noop.NewTracerProvider()
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL, spans)
	if err != nil {
		return nil, err
	}
	a := &App{pool: pool, clock: cfg.Clock, spans: spans, log: log, auth: auth.New(pool, cfg.Auth, cfg.Clock, spans), profile: profile.New(pool), ingredients: ingredients.New(pool), kitchen: kitchen.New(pool, log)}
	a.today = today.New(a.profile, a.kitchen)
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

// Schedule starts the scheduler, which beats on the app's clock every kitchen.BeatEvery
// and brings every User's kitchen up to now, until ctx ends. Each beat is a span.
func (a *App) Schedule(ctx context.Context) {
	tracer := a.spans.Tracer("github.com/simopzz/dandori/internal/app")
	a.clock.Every(ctx, kitchen.BeatEvery, func(at time.Time) {
		ctx, span := tracer.Start(ctx, "kitchen beat")
		defer span.End()
		a.kitchen.Beat(ctx, at)
	})
}

// Handler returns the app's HTTP routes. Every page speaks the signed-in User's Language,
// and asks them for it before anything else; pages before signing in speak the one they
// were switched to. Every signed-in page first brings the User's kitchen up to now. Every
// request but the health check is a span with no parent: one a traceparent header names,
// sent from the internet, is ignored.
func (a *App) Handler() http.Handler {
	signIn := auth.Handlers{Service: a.auth, Clock: a.clock, Log: a.log}
	prof := profile.Handlers{Service: a.profile, Clock: a.clock, Log: a.log, Email: a.auth.Email}
	kit := kitchen.Handlers{Service: a.kitchen, Clock: a.clock, Log: a.log}
	ings := ingredients.Handlers{Service: a.ingredients, Clock: a.clock, Log: a.log}
	day := today.Handlers{Service: a.today, Clock: a.clock, Log: a.log}
	choosing := func(h http.HandlerFunc) http.Handler { return signIn.SignedIn(kit.UpToNow(h)) }
	signedIn := func(h http.HandlerFunc) http.Handler {
		return signIn.SignedIn(kit.UpToNow(prof.LanguageChosen(h)))
	}
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
	mux.Handle("GET /{$}", signedIn(day.Show))
	mux.Handle("GET /profile", signedIn(prof.Show))
	mux.Handle("POST /profile/language", signedIn(prof.ChangeLanguage))
	mux.Handle("GET /kitchen", signedIn(kit.Show))
	mux.Handle("GET /ingredients", signedIn(ings.Show))
	mux.Handle("GET /ingredients/sheet", signedIn(ings.ShowSheet))
	mux.Handle("GET /ingredients/list", signedIn(ings.ShowList))
	mux.Handle("GET /ingredients/suggestion", signedIn(ings.ShowSuggestion))
	mux.Handle("POST /ingredients", signedIn(ings.Add))
	return otelhttp.NewHandler(web.Languages(named(mux)), "request",
		otelhttp.WithTracerProvider(a.spans),
		otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		otelhttp.WithFilter(func(r *http.Request) bool { return r.URL.Path != "/healthz" }))
}

// named names the request's span after the route mux matched. otelhttp would, but it sees
// only the request web.Languages copies before mux routes it.
func named(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		if r.Pattern != "" {
			span := trace.SpanFromContext(r.Context())
			span.SetName(r.Pattern)
			if _, route, ok := strings.Cut(r.Pattern, " "); ok {
				span.SetAttributes(attribute.String("http.route", route))
			}
		}
	})
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
		a.log.WarnContext(r.Context(), "health check", "err", err)
		status, code = "unavailable", http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"database": status})
}
