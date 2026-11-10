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
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/profile"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
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
	// Runs is how agent Runs use the model; runs.New fills in what it leaves unset.
	Runs runs.Config
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
	recipes     *recipes.Service
	kitchen     *kitchen.Service
	ledger      *ledger.Service
	today       *today.Service
	runs        *runs.Engine
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
	a := &App{pool: pool, clock: cfg.Clock, spans: spans, log: log, auth: auth.New(pool, cfg.Auth, cfg.Clock, spans), profile: profile.New(pool), ingredients: ingredients.New(pool, recipes.RecipesUsing), ledger: ledger.New(pool, log)}
	a.runs = runs.New(pool, cfg.Clock, cfg.Runs, log)
	a.recipes = recipes.New(pool, a.ingredients, a.runs)
	a.kitchen = kitchen.New(pool, a.ingredients)
	a.today = today.New(a.profile, a.ledger)
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

// Close ends every Run going, then releases the database connections.
func (a *App) Close() {
	a.runs.Stop()
	a.pool.Close()
}

// Settle waits for every Run going to end, for a test reading what they left.
func (a *App) Settle() { a.runs.Wait() }

// Schedule starts the scheduler, which beats on the app's clock every ledger.BeatEvery
// and brings every User's kitchen up to now, until ctx ends. Each beat is a span.
func (a *App) Schedule(ctx context.Context) {
	tracer := a.spans.Tracer("github.com/simopzz/dandori/internal/app")
	a.clock.Every(ctx, ledger.BeatEvery, func(at time.Time) {
		ctx, span := tracer.Start(ctx, "kitchen beat")
		defer span.End()
		a.ledger.Beat(ctx, at)
	})
}

// Handler returns the app's HTTP routes. Every page speaks the signed-in User's Language,
// and asks them for it before anything else; pages before signing in speak the one they
// were switched to. A page shows the floating Notice the plain post before its redirect
// left. Every signed-in page first brings the User's kitchen up to now. Every
// request but the health check is a span with no parent: one a traceparent header names,
// sent from the internet, is ignored.
func (a *App) Handler() http.Handler {
	signIn := auth.Handlers{Service: a.auth, Clock: a.clock, Log: a.log}
	prof := profile.Handlers{Service: a.profile, Clock: a.clock, Log: a.log, Email: a.auth.Email}
	kit := kitchen.Handlers{Service: a.kitchen, Ledger: a.ledger, Clock: a.clock, Log: a.log}
	ings := ingredients.Handlers{Service: a.ingredients, Clock: a.clock, Log: a.log, CountNew: kit.CountNew}
	recs := recipes.Handlers{Service: a.recipes, Clock: a.clock, Log: a.log}
	run := runs.Handlers{Engine: a.runs, Clock: a.clock, Log: a.log, Kinds: map[string]runs.Kind{
		recipes.ParseKind: {
			Name: recipes.ParseKindName, AskedName: recipes.PastedName,
			ResultName: recipes.DraftName, Result: recs.DraftPage,
		},
	}}
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
	mux.Handle("GET /recipes", signedIn(recs.Library))
	mux.Handle("POST /recipes", signedIn(recs.Add))
	mux.Handle("POST /recipes/paste", signedIn(recs.Paste))
	mux.Handle("GET /recipe-drafts/{id}", signedIn(recs.ShowDraft))
	mux.Handle("GET /runs/{id}", signedIn(run.Show))
	mux.Handle("GET /recipes/new", signedIn(recs.ShowNew))
	mux.Handle("GET /recipes/list", signedIn(recs.ShowList))
	mux.Handle("GET /recipes/ingredient-row", signedIn(recs.ShowRow))
	mux.Handle("GET /recipes/ingredient-row/blank", signedIn(recs.ShowBlankRow))
	mux.Handle("GET /recipes/step-row/blank", signedIn(recs.ShowBlankStep))
	mux.Handle("GET /recipes/{id}", signedIn(recs.Show))
	mux.Handle("POST /recipes/{id}", signedIn(recs.Edit))
	mux.Handle("GET /recipes/{id}/edit", signedIn(recs.ShowEdit))
	mux.Handle("POST /recipes/{id}/delete", signedIn(recs.Delete))
	mux.Handle("GET /kitchen", signedIn(kit.Show))
	mux.Handle("GET /kitchen/stock", signedIn(kit.ShowContents))
	mux.Handle("GET /kitchen/movements", signedIn(kit.ShowMovements))
	mux.Handle("GET /kitchen/movements/sheet", signedIn(kit.ShowMovementsSheet))
	mux.Handle("GET /kitchen/count", signedIn(kit.ShowCount))
	mux.Handle("GET /kitchen/count/sheet", signedIn(kit.ShowCountSheet))
	mux.Handle("GET /kitchen/count/list", signedIn(kit.ShowCountList))
	mux.Handle("POST /kitchen/count", signedIn(kit.Count))
	mux.Handle("GET /kitchen/portions", signedIn(kit.ShowPortions))
	mux.Handle("GET /kitchen/portions/sheet", signedIn(kit.ShowPortionsSheet))
	mux.Handle("POST /kitchen/portions", signedIn(kit.AddPortions))
	mux.Handle("GET /kitchen/portions/movements", signedIn(kit.ShowLotMovements))
	mux.Handle("GET /kitchen/portions/movements/sheet", signedIn(kit.ShowLotMovementsSheet))
	mux.Handle("GET /kitchen/portions/count", signedIn(kit.ShowPortionCount))
	mux.Handle("GET /kitchen/portions/count/sheet", signedIn(kit.ShowPortionCountSheet))
	mux.Handle("POST /kitchen/portions/count", signedIn(kit.CountPortions))
	mux.Handle("GET /kitchen/portions/move", signedIn(kit.ShowPortionMove))
	mux.Handle("GET /kitchen/portions/move/sheet", signedIn(kit.ShowPortionMoveSheet))
	mux.Handle("POST /kitchen/portions/move", signedIn(kit.MovePortions))
	mux.Handle("GET /kitchen/move", signedIn(kit.ShowMove))
	mux.Handle("GET /kitchen/move/sheet", signedIn(kit.ShowMoveSheet))
	mux.Handle("POST /kitchen/move", signedIn(kit.Move))
	mux.Handle("GET /ingredients", signedIn(ings.Show))
	mux.Handle("GET /ingredients/sheet", signedIn(ings.ShowSheet))
	mux.Handle("GET /ingredients/list", signedIn(ings.ShowList))
	mux.Handle("GET /ingredients/suggestion", signedIn(ings.ShowSuggestion))
	mux.Handle("POST /ingredients", signedIn(ings.Add))
	mux.Handle("POST /ingredients/{id}", signedIn(ings.Edit))
	mux.Handle("POST /ingredients/{id}/delete", signedIn(ings.Delete))
	return otelhttp.NewHandler(web.Languages(web.Notices(named(mux))), "request",
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
