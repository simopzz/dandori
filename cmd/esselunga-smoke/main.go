// Command esselunga-smoke checks the Playwright store against the real
// Esselunga site, by hand: it is not part of the automated suite. It only
// reads (the session, the latest order and its first products) and never
// touches the cart.
//
//	DANDORI_PLAYWRIGHT_MCP="npx @playwright/mcp@0.0.83 --user-data-dir ~/.dandori/browser" \
//	  go run ./cmd/esselunga-smoke
//
// Log in to Esselunga in that browser first. Each store action is
// throttled 3s apart, as in Dandori; the trace is printed as it goes.
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

	const throttle = 3 * time.Second
	pw := playwright.New(command, throttle)
	defer pw.Close()
	store := esselunga.Throttle(pw, throttle)
	tr := printed{}

	if err := store.CheckSession(ctx, tr); err != nil {
		return err
	}
	orders, err := store.OrderHistory(ctx, tr, 1)
	if err != nil {
		return err
	}
	if len(orders) == 0 || len(orders[0].Lines) == 0 {
		return errors.New("the order history is empty: nothing to look up")
	}
	show("latest order", orders[0])
	for _, line := range orders[0].Lines[:min(products, len(orders[0].Lines))] {
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
