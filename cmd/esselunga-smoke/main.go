// Command esselunga-smoke checks the Playwright store against the real
// Esselunga site, by hand: it is not part of the automated suite. It reads
// the session, the latest order and its first products. With -cart CODE it
// also puts one more of that product in the cart, checks the cart page, and
// sets it back to what the cart held.
//
//	DANDORI_PLAYWRIGHT_MCP="npx @playwright/mcp@0.0.83 --executable-path /usr/bin/brave --user-data-dir ~/.dandori/browser" \
//	  go run ./cmd/esselunga-smoke [-cart 310496]
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
	"flag"
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
	cart := flag.String("cart", "", "the code of a product to put one more of in the cart, and take back out")
	flag.Parse()
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
	if *cart != "" {
		return checkCart(ctx, store, tr, *cart)
	}
	return nil
}

// checkCart puts one more of the product with code in the cart, checks the
// cart page shows it, and sets it back to what the cart held.
func checkCart(ctx context.Context, store *esselunga.Throttled, tr agent.Trace, code string) error {
	held := func() (int, error) {
		lines, err := store.Cart(ctx, tr)
		if err != nil {
			return 0, err
		}
		show("cart", lines)
		for _, l := range lines {
			if l.Code == code {
				return l.Quantity, nil
			}
		}
		return 0, nil
	}
	before, err := held()
	if err != nil {
		return err
	}
	if err := store.SetCartQuantity(ctx, tr, code, before+1); err != nil {
		return fmt.Errorf("put %d of %s in the cart: %w", before+1, code, err)
	}
	if after, err := held(); err != nil {
		return err
	} else if after != before+1 {
		return fmt.Errorf("the cart page shows %d of %s, want %d", after, code, before+1)
	}
	if err := store.SetCartQuantity(ctx, tr, code, before); err != nil {
		return fmt.Errorf("set %s back to %d in the cart: %w", code, before, err)
	}
	if after, err := held(); err != nil {
		return err
	} else if after != before {
		return fmt.Errorf("the cart page shows %d of %s, want it back to %d", after, code, before)
	}
	return nil
}

func show(what string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Printf("%s:\n%s\n", what, b)
}
