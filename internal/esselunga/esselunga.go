// Package esselunga is the store port: the high-level operations Dandori
// performs on the user's logged-in Esselunga session, and the throttle every
// one of them goes through. The Playwright adapter and the fake store for
// tests implement it.
//
// There is no checkout or payment operation, by design (ADR-0005): an agent
// reaches the store only through these operations, so it cannot check out.
package esselunga

import (
	"context"
	"errors"
	"time"
)

// Store is the port to the user's Esselunga session.
type Store interface {
	// CheckSession returns ErrLoggedOut unless the session is logged in.
	CheckSession(ctx context.Context) error
	// OrderHistory returns the user's latest orders, newest first, at most
	// orders of them.
	OrderHistory(ctx context.Context, orders int) ([]Order, error)
	// LookUpProduct returns what the store shows for the product with
	// code, or ErrNoSuchProduct.
	LookUpProduct(ctx context.Context, code string) (Product, error)
	// SetCartQuantity puts quantity packs of the product with code in the
	// cart, whatever was there: 0 takes it out. Setting rather than adding
	// makes a repeated Cart Fill idempotent.
	SetCartQuantity(ctx context.Context, code string, quantity int) error
	// Cart returns what is in the cart.
	Cart(ctx context.Context) ([]CartLine, error)
}

// ErrLoggedOut means the Esselunga session is not logged in. It says what
// to do, since a Run stops on it.
var ErrLoggedOut = errors.New("the Esselunga session is logged out: log in to Esselunga in Dandori's browser, then run this again")

// ErrNoSuchProduct means the store has no product with that code.
var ErrNoSuchProduct = errors.New("Esselunga has no such product") //nolint:staticcheck // product name shown to the user

// Order is one past Esselunga order.
type Order struct {
	ID          string
	DeliveredOn time.Time
	Lines       []OrderLine
}

// OrderLine is one product of an order.
type OrderLine struct {
	Code     string
	Name     string
	Quantity int
	// Price is what one pack cost, in euros.
	Price float64
	// Weight is the approximate weight of one of a product sold by weight,
	// such as "850 g" for a bunch of bananas; empty otherwise.
	Weight string
}

// Product is what the store shows for one product, as it shows it: the
// agent turns it into a Catalog Product.
type Product struct {
	Code  string
	Name  string
	Brand string
	// Quantity is the pack's quantity as the label states it, such as
	// "400g · Peso sgocciolato: 240g"; empty when the store gives none.
	Quantity string
	// SoldByWeight is true for a product priced by the kilogram, such as
	// loose fruit: its order lines give the weight of one.
	SoldByWeight bool
	// Aisle is the store's category path, such as "Dispensa › Legumi e
	// cereali"; empty when it gives none.
	Aisle string
	// Price is one pack's current price, in euros; per kilogram when
	// SoldByWeight.
	Price     float64
	Available bool
	// NutritionPer is what the nutrition label is given for, such as
	// "Per 100g"; Nutrition is its rows, label to value as printed
	// ("Proteine" → "5,0 g"). Both are empty without a label.
	NutritionPer string
	Nutrition    []NutritionRow
}

// NutritionRow is one row of a nutrition label.
type NutritionRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// CartLine is one product in the cart.
type CartLine struct {
	Code     string
	Name     string
	Quantity int
}
