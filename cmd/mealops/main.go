// Command mealops runs the MealOps app as a single local binary.
//
//	mealops [serve]   serve the app, migrating the database on startup
//	mealops migrate   apply pending migrations and exit
//
// Configuration comes from the environment: MEALOPS_DATABASE_URL (required),
// MEALOPS_ADDR (default 127.0.0.1:8080), MEALOPS_MODEL (default the latest
// Claude model), MEALOPS_LLM_RETRIES (default 3) and MEALOPS_MAX_TURNS
// (default 50), where 0 keeps the default, the Claude credentials
// (ANTHROPIC_API_KEY, or a profile from `ant auth login`), and MEALOPS_DEMO=1
// to offer demo Runs on the Runs page.
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

	"github.com/simopzz/mealops/internal/agent/anthropic"
	"github.com/simopzz/mealops/internal/app"
	"github.com/simopzz/mealops/internal/db"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log, os.Args[1:]); err != nil {
		log.Error("mealops", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := app.Config{
		DatabaseURL: os.Getenv("MEALOPS_DATABASE_URL"),
		Model:       os.Getenv("MEALOPS_MODEL"),
		Demo:        os.Getenv("MEALOPS_DEMO") == "1",
	}
	if cfg.DatabaseURL == "" {
		return errors.New("MEALOPS_DATABASE_URL is not set")
	}
	var err error
	if cfg.LLMRetries, err = envInt("MEALOPS_LLM_RETRIES"); err != nil {
		return err
	}
	if cfg.MaxTurns, err = envInt("MEALOPS_MAX_TURNS"); err != nil {
		return err
	}

	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		return serve(ctx, log, cfg, envOr("MEALOPS_ADDR", "127.0.0.1:8080"))
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
	a, err := app.New(ctx, cfg, app.Ports{LLM: anthropic.New()}, log)
	if err != nil {
		return err
	}
	defer a.Close()

	// A paused database must not stop the app: it shows the error page and
	// migrates once the database is reachable.
	if err := a.EnsureDatabase(ctx); err != nil {
		log.Warn("database not ready at startup; will retry on request", "err", err)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Live trace streams end when MealOps is stopped.
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
