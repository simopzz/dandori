// Command dandori runs the Dandori app as a single binary.
//
//	dandori [serve]     migrate the database, then serve the app
//	dandori healthcheck ask the app serving on DANDORI_ADDR whether it is
//	                    healthy, for the container's health check
//
// Configuration comes from the environment: DANDORI_DATABASE_URL, DANDORI_SUPABASE_URL
// and DANDORI_SUPABASE_PUBLISHABLE_KEY (required to serve), DANDORI_ADDR (default
// 127.0.0.1:8080), DANDORI_TIMEZONE, the zone a day starts and ends in (default
// Europe/Rome), OTEL_EXPORTER_OTLP_ENDPOINT and OTEL_EXPORTER_OTLP_HEADERS, where spans
// and logs go and the token they are sent with (unset, they go nowhere),
// ANTHROPIC_API_KEY, which Runs ask Claude with (unset, every Run fails for having no
// model), DANDORI_MODEL, the Claude model (default claude-opus-5-5), and
// DANDORI_DEMO_LLM, which set to true answers every Run with recipes.DemoLLM instead,
// asking no real model and costing nothing, and DANDORI_FARO_URL, the Faro collector the
// pages send the browser's telemetry to (unset, they load no Faro).
//
// DANDORI_DEV_NOW runs the app on a manual clock at that time, with a fake Supabase Auth
// and the /_test/ routes a driver needs, instead of on the machine's time and the real
// Supabase Auth: DANDORI_DEV_USERS invites the emails it lists, each an email or an
// email and the User's id, as "ada@example.com" or "ada@example.com:<uuid>".
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

	"github.com/simopzz/dandori/internal/agent/anthropic"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/auth"
	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/devmode"
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
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
			FaroURL:     os.Getenv("DANDORI_FARO_URL"),
			Auth: auth.Config{
				SupabaseURL:    os.Getenv("DANDORI_SUPABASE_URL"),
				PublishableKey: os.Getenv("DANDORI_SUPABASE_PUBLISHABLE_KEY"),
			},
		}
		switch {
		case os.Getenv("DANDORI_DEMO_LLM") == "true":
			cfg.Runs = runs.Config{LLM: recipes.DemoLLM{}, Model: "a demo model"}
		case os.Getenv("ANTHROPIC_API_KEY") != "":
			cfg.Runs = runs.Config{LLM: anthropic.New(), Model: os.Getenv("DANDORI_MODEL")}
		}
		var dev *devmode.Dev
		if raw := os.Getenv("DANDORI_DEV_NOW"); raw != "" {
			at, err := devmode.Parse(raw, loc)
			if err != nil {
				return fmt.Errorf("DANDORI_DEV_NOW: %w", err)
			}
			dev, err = devmode.New(at, ledger.BeatEvery, os.Getenv("DANDORI_DEV_USERS"))
			if err != nil {
				return err
			}
		}
		required := map[string]string{"DANDORI_DATABASE_URL": cfg.DatabaseURL}
		if dev == nil {
			// The fake Supabase Auth stands in for the real one in dev mode.
			required["DANDORI_SUPABASE_URL"] = cfg.Auth.SupabaseURL
			required["DANDORI_SUPABASE_PUBLISHABLE_KEY"] = cfg.Auth.PublishableKey
		}
		for name, value := range required {
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
		return serve(ctx, telemetry.Logger(os.Stderr, tel.Logs), cfg, addr, dev)
	case "healthcheck":
		return healthcheck(ctx, addr)
	default:
		return fmt.Errorf("unknown command %q (want serve or healthcheck)", cmd)
	}
}

// serve runs the app on `addr`, on the machine's time and the real Supabase Auth, or, in
// dev mode, on a manual clock and a fake Supabase Auth.
func serve(ctx context.Context, log *slog.Logger, cfg app.Config, addr string, devMode *devmode.Dev) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	if devMode != nil {
		// The fake Supabase Auth is the app's own, under /_test/supabase.
		devMode.Listening(baseURL(ln.Addr()))
		cfg.Clock = devMode.Clock
		cfg.Auth = devMode.Auth.AuthConfig()
	}
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()
	a.Schedule(ctx)
	if devMode != nil {
		// The app wakes up at the dev time: what it assumed up to then is
		// recorded before the first request.
		devMode.Wake()
	}
	srv := &http.Server{
		Handler:           handler(a.Handler(), devMode),
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

// healthcheck fails unless the app on `addr` answers its health check with
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

// handler is the app's handler, with the dev routes mounted when the dev mode is on:
// production has none.
func handler(next http.Handler, devMode *devmode.Dev) http.Handler {
	if devMode == nil {
		return next
	}
	return devMode.Handler(next)
}

// baseURL is the address the app serves on, as a browser reaches it: the
// listener's, with a wildcard host turned into the loopback one.
func baseURL(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://" + addr.String()
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
