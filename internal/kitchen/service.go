// Package kitchen is the Kitchen page: the Stock the Kitchen Ledger adds up to, a Stock
// Item's Movements, and Counts and moves, each a transaction over the Ingredients and
// the Ledger.
package kitchen

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/ledger"
)

// ErrStaple is a Count refused for a Staple, which no Stock Item holds.
var ErrStaple = errors.New("a Staple is never counted")

// Service is the signed-in User's Kitchen page.
type Service struct {
	pool        *pgxpool.Pool
	ingredients *ingredients.Service
}

// New creates the Service.
func New(pool *pgxpool.Pool, ings *ingredients.Service) *Service {
	return &Service{pool: pool, ingredients: ings}
}

// Held is one Stock Item holding more than nothing, for the Kitchen page.
type Held struct {
	Ingredient domain.Ingredient
	Location   domain.StorageLocation
	Item       domain.StockItem
}

// Stock is what the signed-in User's Kitchen Ledger adds up to at now: every Stock Item
// holding more than nothing, by Storage Location, then by name.
func (s *Service) Stock(ctx context.Context, now time.Time) ([]Held, error) {
	var held []Held
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ings, err := ingredients.LoadIngredients(ctx, tx)
		if err != nil {
			return err
		}
		movements, err := ledger.LoadMovements(ctx, tx)
		if err != nil {
			return err
		}
		items := domain.NewLedger(movements).At(now)
		for _, location := range domain.StorageLocations {
			for _, ing := range ings {
				item := items[domain.StockKey{Ingredient: ing.ID, Location: location}]
				if item.Held > 0 {
					held = append(held, Held{Ingredient: ing, Location: location, Item: item})
				}
			}
		}
		return nil
	})
	return held, err
}

// StockItemMovements is one Stock Item with its Movements, newest first.
type StockItemMovements struct {
	Ingredient domain.Ingredient
	Location   domain.StorageLocation
	Movements  []domain.StockMovement
}

// StockItemMovements is the Stock Item of the Ingredient id in location with its
// Movements, none if nothing has moved there, or nil if the signed-in User has no such
// Ingredient or location is no Storage Location. One counted to nothing, or whose
// Ingredient is now a Staple, keeps them.
func (s *Service) StockItemMovements(ctx context.Context, id int64, location domain.StorageLocation) (*StockItemMovements, error) {
	if !slices.Contains(domain.StorageLocations, location) {
		return nil, nil
	}
	var item *StockItemMovements
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ing, ok, err := ingredients.LoadIngredient(ctx, tx, id)
		if err != nil || !ok {
			return err
		}
		movements, err := ledger.LoadMovements(ctx, tx)
		if err != nil {
			return err
		}
		touched := domain.NewLedger(movements).Movements(domain.StockKey{Ingredient: id, Location: location})
		slices.Reverse(touched)
		item = &StockItemMovements{Ingredient: ing, Location: location, Movements: touched}
		return nil
	})
	return item, err
}

// Holding is an Ingredient and what each Storage Location holds of it, read together. A
// Storage Location no Movement of it has reached has no entry.
type Holding struct {
	Ingredient domain.Ingredient
	Held       map[domain.StorageLocation]domain.Quantity
}

// Holding is the signed-in User's Ingredient id and what each Storage Location holds of
// it at now, or nil if they have no such Ingredient.
func (s *Service) Holding(ctx context.Context, now time.Time, id int64) (holding *Holding, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		movements, err := ledger.LoadMovements(ctx, tx)
		if err != nil {
			return err
		}
		holding, err = loadHolding(ctx, tx, domain.NewLedger(movements), now, id)
		return err
	})
	return holding, err
}

// loadHolding is the Holding of the Ingredient id, or nil if there is none, as a Movement
// written at now would find it in l.
func loadHolding(ctx context.Context, q db.Querier, l domain.Ledger, now time.Time, id int64) (*Holding, error) {
	ing, ok, err := ingredients.LoadIngredient(ctx, q, id)
	if err != nil || !ok {
		return nil, err
	}
	return &Holding{Ingredient: ing, Held: l.HeldAt(l.WriteAt(now), id)}, nil
}

// Countable is the signed-in User's Ingredients a search for query finds, Staples left
// out: no Stock Item holds one. When query names a Staple and nothing else is found,
// staple is its name, speaking l.
func (s *Service) Countable(ctx context.Context, l domain.Language, query string) (found []domain.Ingredient, staple string, err error) {
	all, err := s.ingredients.Search(ctx, query)
	if err != nil {
		return nil, "", err
	}
	for _, ing := range all {
		switch {
		case !ing.Staple:
			found = append(found, ing)
		case domain.NormaliseIngredientName(l, ing.Name) == domain.NormaliseIngredientName(l, query):
			staple = ing.Name
		}
	}
	if len(found) > 0 {
		staple = ""
	}
	return found, staple, nil
}

// Recorded is what a Count or a move read once the kitchen was held, and what it wrote:
// the Holding, the Movement recorded, if any, and why each field the User typed was
// refused, by field, if any was, which records nothing.
type Recorded struct {
	Holding
	Movement domain.Movement
	Problems map[string]domain.Problem
}

// Count records at now the Count of the Ingredient id in location, stating what the User
// typed is there, as domain.ParseCount reads it. All is read once the kitchen is held, in
// the transaction that writes it, so the Count is of the Kitchen Ledger as it stands and
// not as a form showed it. It is nil if the signed-in User has no such Ingredient.
// Counting a Staple is ErrStaple, with what was read.
func (s *Service) Count(ctx context.Context, now time.Time, id int64, location domain.StorageLocation, stated, note string) (rec *Recorded, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		return ledger.Write(ctx, tx, func(l domain.Ledger) ([]domain.Movement, error) {
			holding, err := loadHolding(ctx, tx, l, now, id)
			if err != nil || holding == nil {
				return nil, err
			}
			rec = &Recorded{Holding: *holding}
			q, problems := domain.ParseCount(holding.Ingredient.Unit, location, stated)
			rec.Problems = problems
			switch {
			case len(problems) > 0:
				return nil, nil
			case holding.Ingredient.Staple:
				return nil, ErrStaple
			}
			rec.Movement = l.Count(now, domain.StockKey{Ingredient: id, Location: location}, q, strings.TrimSpace(note))
			return []domain.Movement{rec.Movement}, nil
		})
	})
	return rec, err
}

// Move records at now a move of the Ingredient id from one Storage Location to another,
// of what the User typed, as domain.ParseMove reads it. All is read once the kitchen is
// held, so a move is checked against the Kitchen Ledger as it stands and not as a form
// showed it; one domain.CheckMove refuses is a problem of its quantity. It is nil if the
// signed-in User has no such Ingredient.
func (s *Service) Move(ctx context.Context, now time.Time, id int64, from, to domain.StorageLocation, moved, note string) (rec *Recorded, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		return ledger.Write(ctx, tx, func(l domain.Ledger) ([]domain.Movement, error) {
			holding, err := loadHolding(ctx, tx, l, now, id)
			if err != nil || holding == nil {
				return nil, err
			}
			rec = &Recorded{Holding: *holding}
			q, problems := domain.ParseMove(holding.Ingredient.Unit, from, to, moved)
			if rec.Problems = problems; len(problems) > 0 {
				return nil, nil
			}
			var problem domain.Problem
			if rec.Movement, problem = l.Move(now, domain.StockKey{Ingredient: id, Location: from}, to, q, strings.TrimSpace(note)); problem != "" {
				rec.Problems["quantity"] = problem
				return nil, nil
			}
			return []domain.Movement{rec.Movement}, nil
		})
	})
	return rec, err
}
