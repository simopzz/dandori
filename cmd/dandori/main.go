// Command dandori runs the Dandori app as a single binary.
//
//	dandori [serve]     migrate the database, then serve the app
//	dandori healthcheck ask the app serving on DANDORI_ADDR whether it is
//	                    healthy, for the container's health check
//
// Configuration comes from the environment: DANDORI_DATABASE_URL, DANDORI_SUPABASE_URL
// and DANDORI_SUPABASE_PUBLISHABLE_KEY (required to serve), DANDORI_ADDR (default
// 127.0.0.1:8080), DANDORI_TIMEZONE, the zone a day starts and ends in (default
// Europe/Rome), and OTEL_EXPORTER_OTLP_ENDPOINT and OTEL_EXPORTER_OTLP_HEADERS, where
// spans and logs go and the token they are sent with (unset, they go nowhere).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/auth"
	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/telemetry"
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

	addr := envOr("DANDORI_ADDR", "127.0.0.1:8080")
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		loc, err := time.LoadLocation(envOr("DANDORI_TIMEZONE", clock.DefaultZone))
		if err != nil {
			return fmt.Errorf("DANDORI_TIMEZONE: %w", err)
		}
		cfg := app.Config{
			DatabaseURL: os.Getenv("DANDORI_DATABASE_URL"),
			Clock:       clock.Real(loc),
			Auth: auth.Config{
				SupabaseURL:    os.Getenv("DANDORI_SUPABASE_URL"),
				PublishableKey: os.Getenv("DANDORI_SUPABASE_PUBLISHABLE_KEY"),
			},
		}
		for name, value := range map[string]string{
			"DANDORI_DATABASE_URL":             cfg.DatabaseURL,
			"DANDORI_SUPABASE_URL":             cfg.Auth.SupabaseURL,
			"DANDORI_SUPABASE_PUBLISHABLE_KEY": cfg.Auth.PublishableKey,
		} {
			if value == "" {
				return fmt.Errorf("%s is not set", name)
			}
		}
		tel, err := telemetry.Setup(ctx)
		if err != nil {
			return fmt.Errorf("telemetry: %w", err)
		}
		defer func() {
			// ctx has ended by now: what is still buffered gets a moment of its own.
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := tel.Shutdown(flushCtx); err != nil {
				log.Warn("flush telemetry", "err", err)
			}
		}()
		cfg.Spans = tel.Spans
		return serve(ctx, telemetry.Logger(os.Stderr, tel.Logs), cfg, addr)
	case "healthcheck":
		return healthcheck(ctx, addr)
	default:
		return fmt.Errorf("unknown command %q (want serve or healthcheck)", cmd)
	}
}

func serve(ctx context.Context, log *slog.Logger, cfg app.Config, addr string) error {
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()
	a.Schedule(ctx)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
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

// healthcheck fails unless the app on addr answers its health check with
// 200: serving, with its database reachable.
func healthcheck(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("health check answered %s", res.Status)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
