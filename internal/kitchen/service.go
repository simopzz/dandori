// Package kitchen is the Kitchen Ledger (ADR-0013): the Movements of each User's kitchen,
// what they add up to, and the step that brings a kitchen up to now, run first on every
// request and by the scheduler's beat. That step will write the assumed Movements whose
// time has passed; until a Plan gives it any, it only records how far it has reached.
package kitchen

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
)

// BeatEvery is how often the scheduler beats, on the clock.
const BeatEvery = 5 * time.Minute

// ErrStaple is a Count refused for a Staple, which no Stock Item holds.
var ErrStaple = errors.New("a Staple is never counted")

// Service is the signed-in User's Kitchen Ledger, and the step that brings a kitchen
// up to now.
type Service struct {
	pool        *pgxpool.Pool
	log         *slog.Logger
	ingredients *ingredients.Service
}

// New creates the Service.
func New(pool *pgxpool.Pool, log *slog.Logger, ings *ingredients.Service) *Service {
	return &Service{pool: pool, log: log, ingredients: ings}
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
		movements, err := loadMovements(ctx, tx)
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
		movements, err := loadMovements(ctx, tx)
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
		holding, _, err = loadHolding(ctx, tx, now, id)
		return err
	})
	return holding, err
}

// loadHolding is the Holding of the Ingredient id, or nil if there is none, as a Movement
// written at now would find it, with the Ledger that adds it up.
func loadHolding(ctx context.Context, q db.Querier, now time.Time, id int64) (*Holding, domain.Ledger, error) {
	ing, ok, err := ingredients.LoadIngredient(ctx, q, id)
	if err != nil || !ok {
		return nil, domain.Ledger{}, err
	}
	movements, err := loadMovements(ctx, q)
	if err != nil {
		return nil, domain.Ledger{}, err
	}
	l := domain.NewLedger(movements)
	return &Holding{Ingredient: ing, Held: l.HeldAt(l.WriteAt(now), id)}, l, nil
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
		if err := lockKitchen(ctx, tx); err != nil {
			return err
		}
		holding, l, err := loadHolding(ctx, tx, now, id)
		if err != nil || holding == nil {
			return err
		}
		rec = &Recorded{Holding: *holding}
		q, problems := domain.ParseCount(holding.Ingredient.Unit, location, stated)
		rec.Problems = problems
		switch {
		case len(problems) > 0:
			return nil
		case holding.Ingredient.Staple:
			return ErrStaple
		}
		rec.Movement = l.Count(now, domain.StockKey{Ingredient: id, Location: location}, q, strings.TrimSpace(note))
		return saveMovement(ctx, tx, rec.Movement)
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
		if err := lockKitchen(ctx, tx); err != nil {
			return err
		}
		holding, l, err := loadHolding(ctx, tx, now, id)
		if err != nil || holding == nil {
			return err
		}
		rec = &Recorded{Holding: *holding}
		q, problems := domain.ParseMove(holding.Ingredient.Unit, from, to, moved)
		if rec.Problems = problems; len(problems) > 0 {
			return nil
		}
		var problem domain.Problem
		if rec.Movement, problem = l.Move(now, domain.StockKey{Ingredient: id, Location: from}, to, q, strings.TrimSpace(note)); problem != "" {
			rec.Problems["quantity"] = problem
			return nil
		}
		return saveMovement(ctx, tx, rec.Movement)
	})
	return rec, err
}

// CountToNothing counts each Stock Item of the signed-in User's Ingredient id holding
// some of it to nothing at now, as marking it a Staple does, in q's transaction, and is
// what each held, in Storage Location order. It holds the kitchen first, as a Count
// does, so no Count or move of it slips in before q's transaction ends. The Ingredients
// call it, and app wires it, as this feature depends on theirs.
func CountToNothing(ctx context.Context, q db.Querier, now time.Time, id int64) ([]ingredients.CountedToNothing, error) {
	if err := lockKitchen(ctx, q); err != nil {
		return nil, err
	}
	movements, err := loadMovements(ctx, q)
	if err != nil {
		return nil, err
	}
	var counted []ingredients.CountedToNothing
	for _, m := range domain.NewLedger(movements).CountToNothing(now, id) {
		if err := saveMovement(ctx, q, m); err != nil {
			return nil, err
		}
		counted = append(counted, ingredients.CountedToNothing{Location: m.Key.Location, Believed: m.Believed})
	}
	return counted, nil
}

// BringUpToNow brings the signed-in User's kitchen up to now. Run again for a moment it
// has reached, it changes nothing.
func (s *Service) BringUpToNow(ctx context.Context, now time.Time) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveUpTo(ctx, tx, now) })
}

// Beat brings every User's kitchen up to at, the scheduler's beat, and records the beat
// for each. One User's failure is logged and doesn't stop the others.
func (s *Service) Beat(ctx context.Context, at time.Time) {
	users, err := loadUserIDs(ctx, s.pool)
	if err != nil {
		s.log.ErrorContext(ctx, "kitchen beat: list the Users", "err", err)
		return
	}
	for _, id := range users {
		err := db.AsUser(db.WithUser(ctx, id), s.pool, func(tx pgx.Tx) error {
			if err := saveUpTo(ctx, tx, at); err != nil {
				return err
			}
			return saveBeat(ctx, tx, at)
		})
		if err != nil {
			s.log.ErrorContext(ctx, "kitchen beat", "user", id, "err", err)
		}
	}
}

// LastBeat is when the scheduler last brought the signed-in User's kitchen up to now; ok
// is false until it first has.
func (s *Service) LastBeat(ctx context.Context) (at time.Time, ok bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		at, ok, err = loadBeat(ctx, tx)
		return err
	})
	return at, ok, err
}
