// Package kitchen is the Kitchen Ledger (ADR-0013): the Movements of each User's kitchen,
// what they add up to, and the step that brings a kitchen up to now, run first on every
// request and by the scheduler's beat. That step will write the assumed Movements whose
// time has passed; until a Plan gives it any, it only records how far it has reached.
package kitchen

import (
	"context"
	"errors"
	"log/slog"
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
		items := domain.AddUp(movements, now)
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

// Believed is what the signed-in User's Kitchen Ledger believes each Storage Location
// holds of the Ingredient id at now.
func (s *Service) Believed(ctx context.Context, now time.Time, id int64) (believed map[domain.StorageLocation]domain.Quantity, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		believed, err = loadBelieved(ctx, tx, now, id)
		return err
	})
	return believed, err
}

// StartCount is what a Count of the Ingredient id starts from: the Ingredient, and what
// Dandori believes each Storage Location holds of it at now, read together. ok is false
// when it can't be counted: the signed-in User hasn't got it, or it is a Staple.
func (s *Service) StartCount(ctx context.Context, now time.Time, id int64) (ing domain.Ingredient, believed map[domain.StorageLocation]domain.Quantity, ok bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ing, ok, err = ingredients.LoadIngredient(ctx, tx, id)
		if err != nil || !ok {
			return err
		}
		if ing.Staple {
			ok = false
			return nil
		}
		believed, err = loadBelieved(ctx, tx, now, id)
		return err
	})
	return ing, believed, ok, err
}

// loadBelieved adds up what each Storage Location holds of the Ingredient id at now.
func loadBelieved(ctx context.Context, q db.Querier, now time.Time, id int64) (map[domain.StorageLocation]domain.Quantity, error) {
	movements, err := loadMovements(ctx, q)
	if err != nil {
		return nil, err
	}
	items := domain.AddUp(movements, now)
	believed := make(map[domain.StorageLocation]domain.Quantity, len(domain.StorageLocations))
	for _, location := range domain.StorageLocations {
		believed[location] = items[domain.StockKey{Ingredient: id, Location: location}].Held
	}
	return believed, nil
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

// FindCount is the Count id as it was saved, or nil if the signed-in User has none.
func (s *Service) FindCount(ctx context.Context, id int64) (*Counted, error) {
	var counted *Counted
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ingredient, location, stated, ok, err := loadCount(ctx, tx, id)
		if err != nil || !ok {
			return err
		}
		ing, ok, err := ingredients.LoadIngredient(ctx, tx, ingredient)
		if err != nil || !ok {
			return err
		}
		counted = &Counted{Ingredient: ing, Location: location, Stated: stated}
		return nil
	})
	return counted, err
}

// Count records at the Count of the Ingredient id in location, and is that Count's id:
// stated is what the User says is there, and what Dandori believed is read here, once
// the kitchen is held, in the transaction that writes it, so the difference is from the
// Kitchen Ledger as it stands and not as a form showed it. A Count that changes nothing
// is recorded like any other. Counting a Staple is ErrStaple.
func (s *Service) Count(ctx context.Context, at time.Time, id int64, location domain.StorageLocation, stated domain.Quantity, note string) (int64, error) {
	var counted int64
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockKitchen(ctx, tx); err != nil {
			return err
		}
		ing, ok, err := ingredients.LoadIngredient(ctx, tx, id)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("count: the User has no such Ingredient")
		}
		if ing.Staple {
			return ErrStaple
		}
		movements, err := loadMovements(ctx, tx)
		if err != nil {
			return err
		}
		// A Count that overlapped this one may have taken the lock first with a later
		// time: what it wrote is believed all the same.
		asOf := at
		if n := len(movements); n > 0 && movements[n-1].At.After(asOf) {
			asOf = movements[n-1].At
		}
		believed := domain.AddUp(movements, asOf)[domain.StockKey{Ingredient: id, Location: location}].Held
		counted, err = saveCount(ctx, tx, at, id, location, believed, stated, strings.TrimSpace(note))
		return err
	})
	return counted, err
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
