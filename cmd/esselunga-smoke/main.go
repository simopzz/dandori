// Command esselunga-smoke checks the Playwright store against the real
// Esselunga site, by hand: it is not part of the automated suite. It only
// reads (the session, the latest order and its first products) and never
// touches the cart.
//
//	DANDORI_PLAYWRIGHT_MCP="npx @playwright/mcp@0.0.83 --executable-path /usr/bin/brave --user-data-dir ~/.dandori/browser" \
//	  go run ./cmd/esselunga-smoke
//
// Without --executable-path it launches Google Chrome. When the session is
// logged out, as the first time, it waits for you to log in to Esselunga in
// the browser window it opened. Log in there, not by opening the profile by
// hand: Playwright can't read cookies the browser saved with the desktop
// keyring. Store actions are throttled
// DANDORI_STORE_THROTTLE apart (default 3s), as in Dandori; the trace is
// printed as it goes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/esselunga"
	"github.com/simopzz/dandori/internal/esselunga/playwright"
)

// products is how many products of the latest order are looked up.
const products = 3

// loginWait is how long a logged-out session is given to log in.
const loginWait = 10 * time.Minute

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("OK")
}

// printed is a Trace that prints each Trace Event.
type printed struct{}

func (printed) Emit(e agent.Event) {
	fmt.Printf("%s  %-12s %s\n", time.Now().Format("15:04:05"), e.Kind, e.Summary)
}

func run() error {
	command := os.Getenv("DANDORI_PLAYWRIGHT_MCP")
	if command == "" {
		return errors.New("set DANDORI_PLAYWRIGHT_MCP to the command that runs the Playwright MCP server")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	throttle := 3 * time.Second
	if v := os.Getenv("DANDORI_STORE_THROTTLE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return fmt.Errorf("DANDORI_STORE_THROTTLE must be a duration such as 3s, got %q", v)
		}
		throttle = d
	}
	pw := playwright.New(command)
	defer pw.Close()
	store := esselunga.Throttle(pw, throttle)
	tr := printed{}

	if err := store.CheckSession(ctx, tr); errors.Is(err, esselunga.ErrLoggedOut) {
		fmt.Printf("The session is logged out. Log in to Esselunga in the browser window that opened; waiting up to %s.\n", loginWait)
		loginCtx, cancel := context.WithTimeout(ctx, loginWait)
		defer cancel()
		if err := pw.WaitForLogin(loginCtx, 5*time.Second); err != nil {
			return fmt.Errorf("no login: %w", err)
		}
		fmt.Println("Logged in.")
	} else if err != nil {
		return err
	}
	orders, err := store.OrderHistory(ctx, tr, 1)
	if err != nil {
		return err
	}
	if len(orders) == 0 {
		return errors.New("the order history is empty: nothing to look up")
	}
	lines, err := store.OrderLines(ctx, tr, orders[0].ID)
	if err != nil {
		return err
	}
	show("latest order", map[string]any{"order": orders[0], "lines": lines})
	for _, line := range lines[:min(products, len(lines))] {
		p, err := store.LookUpProduct(ctx, tr, line.Code)
		if err != nil {
			return fmt.Errorf("look up %s (%s): %w", line.Name, line.Code, err)
		}
		show("product", p)
		if p.Name == "" || p.Price <= 0 {
			return fmt.Errorf("product %s came back without a name or price", line.Code)
		}
	}
	return nil
}

func show(what string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Printf("%s:\n%s\n", what, b)
}
