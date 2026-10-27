// Package stock is what is in the kitchen: Stock Items and Portions without
// a Slot, each in a Storage Location.
package stock

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/domain"
)

// ErrNotFound means there is no Stock Item with that id.
var ErrNotFound = errors.New("no such stock item")

// ErrUnknownIngredient means a Stock Item names an Ingredient the Recipe
// Library doesn't know.
var ErrUnknownIngredient = errors.New("unknown ingredient")

// InvalidError means a Stock Item breaks the domain rules. Fields is what
// is wrong, keyed by field name as domain.StockItem.Validate gives it.
type InvalidError struct{ Fields map[string]string }

func (e *InvalidError) Error() string { return "invalid stock item" }

func validate(s domain.StockItem) error {
	if invalid := s.Validate(); len(invalid) > 0 {
		return &InvalidError{Fields: invalid}
	}
	return nil
}

// Item is a stored Stock Item.
type Item struct {
	ID int64
	domain.StockItem
}

// Service is the stock's use cases.
type Service struct {
	pool   *pgxpool.Pool
	today  func() time.Time
	arrive func(context.Context) error
}

// New creates the Service; today is the day it is, which decides what the
// Plan has cooked and eaten so far, and arrive takes a Delivery whose
// window has ended as arrived, carrying over what is left of the Cycle it
// closes.
func New(pool *pgxpool.Pool, today func() time.Time, arrive func(context.Context) error) *Service {
	return &Service{pool: pool, today: today, arrive: arrive}
}

// Items loads every Stock Item, by Ingredient name, with its Ingredient's
// name and unit.
func (s *Service) Items(ctx context.Context) ([]Item, error) {
	return loadItems(ctx, s.pool)
}

// Ingredients loads the name and unit of every known Ingredient, by name:
// those a Stock Item can be recorded for.
func (s *Service) Ingredients(ctx context.Context) ([]domain.Ingredient, error) {
	return loadIngredients(ctx, s.pool)
}

// Add records item, a quantity of a known Ingredient.
func (s *Service) Add(ctx context.Context, item domain.StockItem) error {
	if err := validate(item); err != nil {
		return err
	}
	return insertItem(ctx, s.pool, item)
}

// Update adjusts the quantity, Storage Location and expiry date of the
// Stock Item with id to those of item; it keeps its Ingredient.
func (s *Service) Update(ctx context.Context, id int64, item domain.StockItem) error {
	if err := validate(item); err != nil {
		return err
	}
	return updateItem(ctx, s.pool, id, item)
}

// Remove removes the Stock Item with id, if there is one.
func (s *Service) Remove(ctx context.Context, id int64) error {
	return deleteItem(ctx, s.pool, id)
}

// ErrPortionNotFound means there is no Portion with that id.
var ErrPortionNotFound = errors.New("no such portion")

// Portion is a Portion in the kitchen: its Prep Session is cooked, and it
// is not yet eaten.
type Portion struct {
	ID int64
	// Recipe is the name of the Recipe its Prep Session prepared, or empty
	// when the Prep Session set out Ingredients as they are.
	Recipe   string
	Location domain.StorageLocation
	// Slot is where it is to be eaten, or nil for a Portion without a Slot.
	Slot *domain.Slot
}

// Portions loads every Portion in the kitchen, by Recipe. The Plan is taken
// to happen as dated: a Prep Session is cooked from its day on, and a
// Portion is eaten once the day of its Slot is past.
func (s *Service) Portions(ctx context.Context) ([]Portion, error) {
	if err := s.arrive(ctx); err != nil {
		return nil, err
	}
	return loadPortions(ctx, s.pool, s.today())
}

// MovePortion moves the Portion with id to location.
func (s *Service) MovePortion(ctx context.Context, id int64, location domain.StorageLocation) error {
	if msg := location.Validate(); msg != "" {
		return &InvalidError{Fields: map[string]string{"Location": msg}}
	}
	return movePortion(ctx, s.pool, id, location)
}
