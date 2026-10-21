// Command mealops runs the MealOps app as a single local binary.
//
//	mealops [serve]   serve the app, migrating the database on startup
//	mealops migrate   apply pending migrations and exit
//
// Configuration comes from the environment: MEALOPS_DATABASE_URL (required)
// and MEALOPS_ADDR (default 127.0.0.1:8080).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	cfg := app.Config{DatabaseURL: os.Getenv("MEALOPS_DATABASE_URL")}
	if cfg.DatabaseURL == "" {
		return errors.New("MEALOPS_DATABASE_URL is not set")
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
	a, err := app.New(ctx, cfg, app.Ports{}, log)
	if err != nil {
		return err
	}
	defer a.Close()

	// A paused database must not stop the app: it shows the error page and
	// migrates once the database is reachable.
	if err := a.EnsureDatabase(ctx); err != nil {
		log.Warn("database not ready at startup; will retry on request", "err", err)
	}

	srv := &http.Server{Addr: addr, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("serving", "url", "http://"+addr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
