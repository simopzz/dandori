package esselunga

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/simopzz/dandori/internal/agent"
)

// ErrNoStore means Dandori was started without a way to reach Esselunga.
var ErrNoStore = errors.New("no Esselunga store is configured: set DANDORI_PLAYWRIGHT_MCP")

// Throttled is the store as Runs use it: one action at a time, across all
// Runs, at least the delay apart and up to half as much again, picked at
// random, so Dandori behaves politely, doesn't tick like a clock, and the
// user's account isn't flagged. Every action and every wait is a Trace
// Event of the Run that asked for it.
type Throttled struct {
	store Store
	delay time.Duration

	mu   sync.Mutex
	last time.Time // when the last action ended
}

// Throttle spaces store's actions delay apart. A nil store fails every
// action with ErrNoStore.
func Throttle(store Store, delay time.Duration) *Throttled {
	return &Throttled{store: store, delay: delay}
}

// do waits for its turn, records action to tr and runs it.
func (t *Throttled) do(ctx context.Context, tr agent.Trace, action string, run func() error) error {
	if t.store == nil {
		return ErrNoStore
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.last.IsZero() {
		gap := t.delay + rand.N(t.delay/2+1)
		if wait := gap - time.Since(t.last); wait > 0 {
			tr.Emit(agent.Event{Kind: agent.EventWait, Summary: fmt.Sprintf("Waiting %s before %s", wait.Round(time.Millisecond), action)})
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	tr.Emit(agent.Event{Kind: agent.EventStoreAction, Summary: "Esselunga: " + action})
	defer func() { t.last = time.Now() }()
	return run()
}

// CheckSession returns ErrLoggedOut unless the session is logged in.
func (t *Throttled) CheckSession(ctx context.Context, tr agent.Trace) error {
	return t.do(ctx, tr, "checking the session", func() error { return t.store.CheckSession(ctx) })
}

// OrderHistory returns the latest orders, newest first, without their
// lines.
func (t *Throttled) OrderHistory(ctx context.Context, tr agent.Trace, orders int) ([]Order, error) {
	var history []Order
	err := t.do(ctx, tr, fmt.Sprintf("reading the last %d orders", orders), func() error {
		var err error
		history, err = t.store.OrderHistory(ctx, orders)
		return err
	})
	return history, err
}

// OrderLines returns what the order with id brought.
func (t *Throttled) OrderLines(ctx context.Context, tr agent.Trace, id string) ([]OrderLine, error) {
	var lines []OrderLine
	err := t.do(ctx, tr, "reading order "+id, func() error {
		var err error
		lines, err = t.store.OrderLines(ctx, id)
		return err
	})
	return lines, err
}

// LookUpProduct returns what the store shows for the product with code.
func (t *Throttled) LookUpProduct(ctx context.Context, tr agent.Trace, code string) (Product, error) {
	var p Product
	err := t.do(ctx, tr, "looking up product "+code, func() error {
		var err error
		p, err = t.store.LookUpProduct(ctx, code)
		return err
	})
	return p, err
}

// SetCartQuantity puts quantity packs of the product with code in the cart.
func (t *Throttled) SetCartQuantity(ctx context.Context, tr agent.Trace, code string, quantity int) error {
	return t.do(ctx, tr, fmt.Sprintf("setting product %s to %d in the cart", code, quantity), func() error {
		return t.store.SetCartQuantity(ctx, code, quantity)
	})
}

// Cart returns what is in the cart.
func (t *Throttled) Cart(ctx context.Context, tr agent.Trace) ([]CartLine, error) {
	var lines []CartLine
	err := t.do(ctx, tr, "reading the cart", func() error {
		var err error
		lines, err = t.store.Cart(ctx)
		return err
	})
	return lines, err
}
