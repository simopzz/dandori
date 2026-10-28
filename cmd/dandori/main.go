// Command dandori runs the Dandori app as a single local binary.
//
//	dandori [serve]   serve the app, migrating the database on startup
//	dandori migrate   apply pending migrations and exit
//
// Configuration comes from the environment: DANDORI_DATABASE_URL (required),
// DANDORI_CONTENT_LANGUAGE (it or en, default en; fixed once the database
// has been started with it), DANDORI_ADDR (default 127.0.0.1:8080),
// DANDORI_MODEL (default the latest Claude model), DANDORI_LLM_RETRIES
// (default 3) and DANDORI_MAX_TURNS (default 50), where 0 keeps the default,
// the Claude credentials (ANTHROPIC_API_KEY, or a profile from
// `ant auth login`), DANDORI_DEMO=1 to offer demo Runs on the Runs page,
// DANDORI_PLAYWRIGHT_MCP (the command that runs the Playwright MCP server
// on the browser logged in to Esselunga; unset, store Runs fail saying so),
// DANDORI_STORE_THROTTLE (the least time between store actions, default 3s,
// to which each wait adds up to half again at random),
// DANDORI_SEED_ORDERS (how many orders a Known Catalog seed reads,
// default 10), VOYAGE_API_KEY (to search the Recipe Library by meaning;
// unset, searches only filter) and DANDORI_EMBEDDING_MODEL (the Voyage
// model, default voyage-4-large).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/simopzz/dandori/internal/agent/anthropic"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/embedding/voyage"
	"github.com/simopzz/dandori/internal/esselunga/playwright"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log, os.Args[1:]); err != nil {
		log.Error("dandori", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := app.Config{
		DatabaseURL:     os.Getenv("DANDORI_DATABASE_URL"),
		ContentLanguage: domain.ContentLanguage(os.Getenv("DANDORI_CONTENT_LANGUAGE")),
		Model:           os.Getenv("DANDORI_MODEL"),
		Demo:            os.Getenv("DANDORI_DEMO") == "1",
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DANDORI_DATABASE_URL is not set")
	}
	var err error
	if cfg.LLMRetries, err = envInt("DANDORI_LLM_RETRIES"); err != nil {
		return err
	}
	if cfg.MaxTurns, err = envInt("DANDORI_MAX_TURNS"); err != nil {
		return err
	}
	if cfg.SeedOrders, err = envInt("DANDORI_SEED_ORDERS"); err != nil {
		return err
	}
	if v := os.Getenv("DANDORI_STORE_THROTTLE"); v != "" {
		if cfg.StoreThrottle, err = time.ParseDuration(v); err != nil || cfg.StoreThrottle <= 0 {
			return fmt.Errorf("DANDORI_STORE_THROTTLE must be a duration such as 3s, got %q", v)
		}
	}

	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		return serve(ctx, log, cfg, envOr("DANDORI_ADDR", "127.0.0.1:8080"))
	case "migrate":
		pool, err := db.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		if err := db.Migrate(ctx, pool); err != nil {
			return err
		}
		log.Info("migrations applied")
		return nil
	default:
		return fmt.Errorf("unknown command %q (want serve or migrate)", cmd)
	}
}

func serve(ctx context.Context, log *slog.Logger, cfg app.Config, addr string) error {
	ports := app.Ports{LLM: anthropic.New()}
	if key := os.Getenv("VOYAGE_API_KEY"); key != "" {
		ports.Embedder = voyage.New(key, os.Getenv("DANDORI_EMBEDDING_MODEL"))
	}
	if command := os.Getenv("DANDORI_PLAYWRIGHT_MCP"); command != "" {
		store := playwright.New(command)
		defer store.Close()
		ports.Store = store
	}
	a, err := app.New(ctx, cfg, ports, log)
	if err != nil {
		return err
	}
	defer a.Close()

	// A paused database must not stop the app: it shows the error page and
	// migrates once the database is reachable. A changed Content Language
	// does stop it: the setting has to be fixed first.
	if err := a.EnsureDatabase(ctx); errors.As(err, new(*app.ContentLanguageChangedError)) {
		return err
	} else if err != nil {
		log.Warn("database not ready at startup; will retry on request", "err", err)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Live trace streams end when Dandori is stopped.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("serving", "addr", ln.Addr().String())

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// envInt reads a whole number from the environment; unset is 0, the default.
func envInt(key string) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a whole number, got %q", key, v)
	}
	return n, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
