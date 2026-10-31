// Package stock is what is in the kitchen: Stock Items and Portions without
// a Slot, each in a Storage Location.
package stock

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
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

// Service is the stock's use cases. What they show is as stored: the app
// has a Delivery whose window has ended, and the Prep Sessions whose day
// has come, change the stock before any of them runs (ADR-0010).
type Service struct {
	pool  *pgxpool.Pool
	today func() time.Time
}

// New creates the Service; today is the day it is, which decides what the
// Plan has cooked and eaten so far.
func New(pool *pgxpool.Pool, today func() time.Time) *Service {
	return &Service{pool: pool, today: today}
}

// Items loads every Stock Item, by Ingredient name, with its Ingredient's
// name and unit.
func (s *Service) Items(ctx context.Context) ([]Item, error) {
	return loadItems(ctx, s.pool)
}

// ItemsIn loads every Stock Item through q, so a caller's transaction can
// read them with what else it reads.
func (s *Service) ItemsIn(ctx context.Context, q db.Querier) ([]domain.StockItem, error) {
	items, err := loadItems(ctx, q)
	stock := make([]domain.StockItem, len(items))
	for i, it := range items {
		stock[i] = it.StockItem
	}
	return stock, err
}

// Receive records items, what a Delivery brought, through q, in a caller's
// transaction.
func (s *Service) Receive(ctx context.Context, q db.Querier, items []domain.StockItem) error {
	for _, item := range items {
		if err := validate(item); err != nil {
			return err
		}
		if err := insertItem(ctx, q, item); err != nil {
			return err
		}
	}
	return nil
}

// UseUp has sessions, Prep Sessions whose day has come, in the order of
// their days, use up the Stock Items of their Ingredients through q, in a
// caller's transaction, recording what each took. The caller sees to it
// that each Prep Session uses up its stock only once.
func (s *Service) UseUp(ctx context.Context, q db.Querier, sessions []domain.PrepSession) error {
	items, err := lockItems(ctx, q)
	if err != nil {
		return err
	}
	stock := make([]domain.StockItem, len(items))
	for i, it := range items {
		stock[i] = it.StockItem
	}
	_, uses := domain.UseUp(sessions, stock)
	for i, use := range uses {
		for _, t := range use.Taken {
			id := items[t.From].ID
			if err := insertTaken(ctx, q, sessions[i].ID, id, t); err != nil {
				return err
			}
			if err := setQuantity(ctx, q, id, t.Leaves); err != nil {
				return err
			}
		}
	}
	return nil
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
	return loadPortions(ctx, s.pool, s.today())
}

// MovePortion moves the Portion with id to location.
func (s *Service) MovePortion(ctx context.Context, id int64, location domain.StorageLocation) error {
	if msg := location.Validate(); msg != "" {
		return &InvalidError{Fields: map[string]string{"Location": msg}}
	}
	return movePortion(ctx, s.pool, id, location)
}
