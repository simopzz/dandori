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
	"slices"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/catalog"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/demo"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/embedding"
	"github.com/simopzz/dandori/internal/esselunga"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/planning"
	"github.com/simopzz/dandori/internal/profile"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
	"github.com/simopzz/dandori/internal/stock"
	"github.com/simopzz/dandori/internal/web"
)

// Config is what the app needs from its environment. Zero values take the
// defaults below.
type Config struct {
	DatabaseURL string

	// ContentLanguage is the language every food term is written in,
	// recorded in the database on first start; English when empty.
	ContentLanguage domain.ContentLanguage

	// Model is the Claude model agent Runs use.
	Model string
	// LLMRetries is how many times a failed request to the model is retried
	// when the failure is worth retrying; LLMRetryBackoff is the first wait.
	LLMRetries      int
	LLMRetryBackoff time.Duration
	// MaxTurns bounds the requests to the model in one Run.
	MaxTurns int
	// PlanAttempts is how many Plans breaking the rules a planning Run may
	// submit; the last one fails it.
	PlanAttempts int

	// StoreThrottle is the least time between two actions on Esselunga;
	// each wait adds up to half again at random.
	StoreThrottle time.Duration
	// SeedOrders is how many of the latest Esselunga orders a Known
	// Catalog seed reads.
	SeedOrders int

	// Demo offers demo Runs on the Runs page, for trying the agent by hand.
	Demo bool

	// Now is the clock: the Plan is taken to happen as dated, so today
	// decides what is cooked and what is eaten, and a Delivery arrives at
	// the end of its window. time.Now when nil.
	Now func() time.Time
}

// DefaultStoreThrottle is the least time between two actions on Esselunga,
// so Dandori behaves politely and the user's account isn't flagged.
const DefaultStoreThrottle = 3 * time.Second

// DefaultModel is the latest Claude model.
const DefaultModel = "claude-opus-5-5"

func (c Config) withDefaults() Config {
	if c.ContentLanguage == "" {
		c.ContentLanguage = domain.English
	}
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
	if c.PlanAttempts == 0 {
		c.PlanAttempts = 3
	}
	if c.StoreThrottle == 0 {
		c.StoreThrottle = DefaultStoreThrottle
	}
	if c.SeedOrders == 0 {
		c.SeedOrders = 10
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Ports are the outbound dependencies that talk to the outside world. Tests
// replace them with fakes.
type Ports struct {
	LLM agent.LLM
	// Embedder embeds Recipes for searching the Recipe Library by meaning;
	// without one, searches only filter.
	Embedder embedding.Embedder
	// Store is the user's Esselunga session; without one, Runs that need
	// it fail saying so.
	Store esselunga.Store
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
	stock   *stock.Service
	plan    *planning.Service
	catalog *catalog.Service
}

// New creates the app. It does not connect to the database: a paused or
// unreachable database is reported by the app rather than stopping it.
func New(ctx context.Context, cfg Config, ports Ports, log *slog.Logger) (*App, error) {
	cfg = cfg.withDefaults()
	if !slices.Contains(domain.ContentLanguages, cfg.ContentLanguage) {
		return nil, fmt.Errorf("DANDORI_CONTENT_LANGUAGE is %q; it must be it or en", cfg.ContentLanguage)
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a := &App{pool: pool, cfg: cfg, log: log}
	a.runs = runs.New(pool, runs.Config{
		LLM:      ports.LLM,
		Model:    a.cfg.Model,
		Retries:  a.cfg.LLMRetries,
		Backoff:  a.cfg.LLMRetryBackoff,
		MaxTurns: a.cfg.MaxTurns,
	}, log)
	a.profile = profile.New(pool)
	today := func() time.Time { return domain.DayOf(a.cfg.Now()) }
	a.stock = stock.New(pool, today)
	a.recipes = recipes.New(pool, recipes.Config{
		Runs:     a.runs,
		Language: a.cfg.ContentLanguage,
		Embedder: ports.Embedder,
		Stock:    a.stock,
		Today:    today,
	})
	a.catalog = catalog.New(pool, a.runs, catalog.Config{
		Store:    esselunga.Throttle(ports.Store, a.cfg.StoreThrottle),
		Orders:   a.cfg.SeedOrders,
		Language: a.cfg.ContentLanguage,
		Now:      a.cfg.Now,
	})
	a.plan = planning.New(pool, planning.Config{
		Profile:      a.profile,
		Recipes:      a.recipes,
		Stock:        a.stock,
		Catalog:      a.catalog,
		Runs:         a.runs,
		Language:     a.cfg.ContentLanguage,
		PlanAttempts: a.cfg.PlanAttempts,
		Now:          a.cfg.Now,
	})
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
	if err := a.checkContentLanguage(ctx); err != nil {
		return err
	}
	if err := a.runs.AbandonInterrupted(ctx); err != nil {
		return err
	}
	a.migrated = true
	return nil
}

// ContentLanguageChangedError means the configured Content Language is not
// the one the database was started with.
type ContentLanguageChangedError struct {
	Recorded, Configured domain.ContentLanguage
}

func (e *ContentLanguageChangedError) Error() string {
	return fmt.Sprintf("the database's Content Language is %s, but DANDORI_CONTENT_LANGUAGE is %s: switching language means a new database", e.Recorded, e.Configured)
}

// checkContentLanguage records the configured Content Language on first
// start, and refuses a later start with a different one.
func (a *App) checkContentLanguage(ctx context.Context) error {
	recorded, err := db.RecordContentLanguage(ctx, a.pool, a.cfg.ContentLanguage)
	if err != nil {
		return err
	}
	if recorded != a.cfg.ContentLanguage {
		return &ContentLanguageChangedError{Recorded: recorded, Configured: a.cfg.ContentLanguage}
	}
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
	stock := stock.Handlers{Service: a.stock, Log: a.log}
	plan := planning.Handlers{Service: a.plan, Log: a.log}
	catalog := catalog.Handlers{Service: a.catalog, Log: a.log}
	mux := http.NewServeMux()
	web.Assets(mux)
	mux.HandleFunc("GET /healthz", a.health)
	mux.Handle("GET /{$}", a.needsDatabase(a.settles(templ.Handler(web.Home()))))
	mux.Handle("GET /plan", a.needsDatabase(a.settles(http.HandlerFunc(plan.Show))))
	mux.Handle("POST /cycles", a.needsDatabase(a.settles(http.HandlerFunc(plan.OpenCycle))))
	mux.Handle("POST /cycles/{id}/delivery", a.needsDatabase(a.settles(http.HandlerFunc(plan.ChangeDelivery))))
	mux.Handle("POST /cycles/{id}/drop", a.needsDatabase(a.settles(http.HandlerFunc(plan.Drop))))
	mux.Handle("POST /cycles/{id}/plan", a.needsDatabase(a.settles(http.HandlerFunc(plan.Plan))))
	mux.Handle("GET /cycles/{id}/exceptions", a.needsDatabase(a.settles(http.HandlerFunc(plan.ShowExceptions))))
	mux.Handle("POST /cycles/{id}/exceptions", a.needsDatabase(a.settles(http.HandlerFunc(plan.LogException))))
	mux.Handle("GET /cycles/{id}/groceries", a.needsDatabase(a.settles(http.HandlerFunc(plan.ShowGroceries))))
	mux.Handle("POST /exceptions/remove", a.needsDatabase(a.settles(http.HandlerFunc(plan.RemoveExceptionBetweenCycles))))
	mux.Handle("GET /plan-proposals/{id}", a.needsDatabase(a.settles(http.HandlerFunc(plan.ShowProposal))))
	mux.Handle("POST /plan-proposals/{id}/approve", a.needsDatabase(a.settles(http.HandlerFunc(plan.Approve))))
	mux.Handle("POST /plan-proposals/{id}/reject", a.needsDatabase(a.settles(http.HandlerFunc(plan.Reject))))
	mux.Handle("GET /plan-proposals/{proposal}/recipe-drafts/{id}", a.needsDatabase(a.settles(http.HandlerFunc(recipes.ShowPlannedDraft))))
	mux.Handle("POST /plan-proposals/{proposal}/recipe-drafts/{id}", a.needsDatabase(a.settles(http.HandlerFunc(recipes.AcceptPlannedDraft))))
	mux.Handle("POST /plan-proposals/{proposal}/recipe-drafts/{id}/discard", a.needsDatabase(a.settles(http.HandlerFunc(recipes.DiscardPlannedDraft))))
	mux.Handle("GET /profile", a.needsDatabase(a.settles(http.HandlerFunc(profile.Show))))
	mux.Handle("POST /profile", a.needsDatabase(a.settles(http.HandlerFunc(profile.Save))))
	mux.Handle("POST /profile/brief", a.needsDatabase(a.settles(http.HandlerFunc(profile.SaveBrief))))
	mux.Handle("GET /recipes", a.needsDatabase(a.settles(http.HandlerFunc(recipes.Library))))
	mux.Handle("POST /recipes/parse", a.needsDatabase(a.settles(http.HandlerFunc(recipes.Parse))))
	mux.Handle("GET /recipe-drafts/{id}", a.needsDatabase(a.settles(http.HandlerFunc(recipes.ShowDraft))))
	mux.Handle("POST /recipe-drafts/{id}", a.needsDatabase(a.settles(http.HandlerFunc(recipes.AcceptDraft))))
	mux.Handle("POST /recipe-drafts/{id}/discard", a.needsDatabase(a.settles(http.HandlerFunc(recipes.DiscardDraft))))
	mux.Handle("GET /recipes/{id}", a.needsDatabase(a.settles(http.HandlerFunc(recipes.Show))))
	mux.Handle("POST /recipes/{id}", a.needsDatabase(a.settles(http.HandlerFunc(recipes.Update))))
	mux.Handle("GET /recipes/{id}/edit", a.needsDatabase(a.settles(http.HandlerFunc(recipes.Edit))))
	mux.Handle("POST /recipes/{id}/delete", a.needsDatabase(a.settles(http.HandlerFunc(recipes.Delete))))
	mux.Handle("GET /stock", a.needsDatabase(a.settles(http.HandlerFunc(stock.Show))))
	mux.Handle("POST /stock-items", a.needsDatabase(a.settles(http.HandlerFunc(stock.Add))))
	mux.Handle("POST /stock-items/{id}", a.needsDatabase(a.settles(http.HandlerFunc(stock.Update))))
	mux.Handle("POST /stock-items/{id}/delete", a.needsDatabase(a.settles(http.HandlerFunc(stock.Remove))))
	mux.Handle("POST /portions/{id}/move", a.needsDatabase(a.settles(http.HandlerFunc(stock.MovePortion))))
	mux.Handle("GET /catalog", a.needsDatabase(a.settles(http.HandlerFunc(catalog.Show))))
	mux.Handle("GET /catalog/runs/{id}", a.needsDatabase(a.settles(http.HandlerFunc(catalog.ShowRun))))
	mux.Handle("POST /catalog/seed", a.needsDatabase(a.settles(http.HandlerFunc(catalog.Seed))))
	mux.Handle("POST /catalog/refresh", a.needsDatabase(a.settles(http.HandlerFunc(catalog.Refresh))))
	mux.Handle("POST /catalog/propose-mappings", a.needsDatabase(a.settles(http.HandlerFunc(catalog.Propose))))
	mux.Handle("POST /product-mappings/{id}", a.needsDatabase(a.settles(http.HandlerFunc(catalog.SetMapping))))
	mux.Handle("GET /runs", a.needsDatabase(a.settles(http.HandlerFunc(runs.List))))
	mux.Handle("POST /runs", a.needsDatabase(a.settles(http.HandlerFunc(runs.Start))))
	mux.Handle("GET /runs/{id}", a.needsDatabase(a.settles(http.HandlerFunc(runs.Show))))
	mux.Handle("GET /runs/{id}/events", a.needsDatabase(a.settles(http.HandlerFunc(runs.Stream))))
	mux.Handle("POST /runs/{id}/cancel", a.needsDatabase(a.settles(http.HandlerFunc(runs.Cancel))))
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
				// Reload into the error page rather than swap it in, which would
				// push the request's URL: for a form, not a page to reload.
				w.Header().Set("HX-Refresh", "true")
			}
			templ.Handler(web.DatabaseUnavailable(), templ.WithStatus(http.StatusServiceUnavailable)).ServeHTTP(w, r)
			return
		case errors.As(err, new(*ContentLanguageChangedError)):
			// A setting to fix, not a bug: say which.
			a.log.Error("database not ready", "err", err)
			http.Error(w, "Dandori can't use its database: "+err.Error()+".", http.StatusInternalServerError)
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

// settles takes a Delivery whose window has ended as arrived, and has the
// Prep Sessions whose day has come use up their stock, before next serves,
// so what it shows and changes is as of now. Every route needing the
// database settles, after needsDatabase; use cases never do (ADR-0010).
func (a *App) settles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := a.plan.Arrive(r.Context()); err != nil {
			httpx.ServerError(w, a.log, "settle the kitchen", err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RecipeSearchTool is the Recipe Library search as an agent tool, for the
// planner agent.
func (a *App) RecipeSearchTool() agent.Tool { return a.recipes.SearchTool() }

// ExceptionsTool lists the Slots logged as a Meal Out or skipped as an
// agent tool, for the planner agent.
func (a *App) ExceptionsTool() agent.Tool { return a.plan.ExceptionsTool() }

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
