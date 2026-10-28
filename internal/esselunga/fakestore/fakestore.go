// Package fakestore is an in-memory Esselunga for tests: order history,
// products, availability and a cart, behind the store port. A test can log
// the session out or take a product out of stock while a Run uses it.
package fakestore

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/simopzz/dandori/internal/esselunga"
)

// Store is a fake Esselunga. It is safe to change while a Run uses it.
type Store struct {
	mu        sync.Mutex
	orders    []esselunga.Order // newest first
	products  map[string]esselunga.Product
	cart      map[string]int
	loggedOut bool
	actions   []string
}

// New returns a logged-in fake store selling products, with no orders.
func New(products ...esselunga.Product) *Store {
	s := &Store{products: map[string]esselunga.Product{}, cart: map[string]int{}}
	for _, p := range products {
		s.products[p.Code] = p
	}
	return s
}

// Ordered adds order to the order history as the newest one.
func (s *Store) Ordered(order esselunga.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders = slices.Insert(s.orders, 0, order)
}

// LogOut logs the session out, as when Esselunga's session expires.
func (s *Store) LogOut() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loggedOut = true
}

// SetAvailable makes the product with code available or out of stock.
func (s *Store) SetAvailable(code string, available bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.products[code]
	p.Available = available
	s.products[code] = p
}

// Actions returns the operations performed on the store, in order, such as
// "LookUpProduct 412106".
func (s *Store) Actions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.actions)
}

// act records an action and fails it while the session is logged out.
func (s *Store) act(action string) error {
	s.actions = append(s.actions, action)
	if s.loggedOut {
		return esselunga.ErrLoggedOut
	}
	return nil
}

func (s *Store) CheckSession(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.act("CheckSession")
}

func (s *Store) OrderHistory(_ context.Context, orders int) ([]esselunga.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.act("OrderHistory"); err != nil {
		return nil, err
	}
	return slices.Clone(s.orders[:min(orders, len(s.orders))]), nil
}

func (s *Store) LookUpProduct(_ context.Context, code string) (esselunga.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.act("LookUpProduct " + code); err != nil {
		return esselunga.Product{}, err
	}
	p, ok := s.products[code]
	if !ok {
		return esselunga.Product{}, esselunga.ErrNoSuchProduct
	}
	return p, nil
}

func (s *Store) SetCartQuantity(_ context.Context, code string, quantity int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.act("SetCartQuantity " + code); err != nil {
		return err
	}
	if _, ok := s.products[code]; !ok {
		return esselunga.ErrNoSuchProduct
	}
	if quantity <= 0 {
		delete(s.cart, code)
	} else {
		s.cart[code] = quantity
	}
	return nil
}

func (s *Store) Cart(context.Context) ([]esselunga.CartLine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.act("Cart"); err != nil {
		return nil, err
	}
	var lines []esselunga.CartLine
	for code, q := range s.cart {
		lines = append(lines, esselunga.CartLine{Code: code, Name: s.products[code].Name, Quantity: q})
	}
	slices.SortFunc(lines, func(a, b esselunga.CartLine) int { return cmp.Compare(a.Code, b.Code) })
	return lines, nil
}
