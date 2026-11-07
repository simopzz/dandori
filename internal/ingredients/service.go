// Package ingredients is the User's Ingredients: adding them by hand, listing, editing
// and deleting them, in the side sheet any page can open. Each food is one Ingredient,
// identified by its name as domain.NormaliseIngredientName reduces it in the Language of
// its User.
package ingredients

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ledger"
)

// NameTakenError is an Ingredient refused because the User has one of that food already,
// named Existing.
type NameTakenError struct{ Existing string }

func (e *NameTakenError) Error() string {
	return fmt.Sprintf("the User already has the Ingredient %q", e.Existing)
}

// SuggestedError is an Ingredient not added because its name is close to the name of one
// the User has, Closest, which they may have meant.
type SuggestedError struct{ Closest string }

func (e *SuggestedError) Error() string {
	return fmt.Sprintf("the User may have meant the Ingredient %q", e.Closest)
}

// UnitFixedError is an Ingredient's unit not changed because Movements of it are recorded
// in its unit, Unit, which they keep however it is counted afterwards.
type UnitFixedError struct{ Unit domain.Unit }

func (e *UnitFixedError) Error() string {
	return fmt.Sprintf("the Ingredient's Movements are recorded in %s", e.Unit)
}

// ErrHasMovements is an Ingredient not deleted because Movements refer to it: deleting it
// would take part of the Kitchen Ledger's history with it.
var ErrHasMovements = errors.New("movements refer to the Ingredient")

// ErrNoIngredient is an Ingredient the signed-in User hasn't got.
var ErrNoIngredient = errors.New("the User has no such Ingredient")

// Service is the signed-in User's Ingredients.
type Service struct{ pool *pgxpool.Pool }

// New creates the Service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Add adds ing, which has no Problems, for the signed-in User, whose Language is l, and
// is its id. One whose name normalises as another's is a *NameTakenError. One whose name
// is only close to another's is a *SuggestedError, unless confirmed says the User meant
// the name typed.
func (s *Service) Add(ctx context.Context, l domain.Language, ing domain.Ingredient, confirmed bool) (id int64, err error) {
	normalised := domain.NormaliseIngredientName(l, ing.Name)
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		if !confirmed {
			known, err := LoadIngredients(ctx, tx)
			if err != nil {
				return err
			}
			if closest := closeTo(l, ing.Name, known); closest != "" {
				return &SuggestedError{Closest: closest}
			}
		}
		var added bool
		id, added, err = saveIngredient(ctx, tx, ing, normalised)
		if err != nil || added {
			return err
		}
		existing, err := loadNameOf(ctx, tx, normalised)
		if err != nil {
			return err
		}
		return &NameTakenError{Existing: existing}
	})
	return id, err
}

// Edit sets the signed-in User's Ingredient id, whose Language is l, to ing, which has no
// Problems, at at. Marking it a Staple counts its stock to nothing in the same
// transaction, and counted is the Movement of each Stock Item that held some; unmarking
// it starts it from nothing. A name that normalises as another's is a *NameTakenError. A
// new name only close to another's is a *SuggestedError, unless confirmed says the User
// meant the name typed. A unit changed once Movements refer to the Ingredient is a
// *UnitFixedError, and one they haven't got is ErrNoIngredient.
func (s *Service) Edit(ctx context.Context, l domain.Language, at time.Time, id int64, ing domain.Ingredient, confirmed bool) (counted []domain.Movement, err error) {
	normalised := domain.NormaliseIngredientName(l, ing.Name)
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		current, ok, err := LoadIngredient(ctx, tx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNoIngredient
		}
		taken, err := loadOtherNameOf(ctx, tx, id, normalised)
		if err != nil {
			return err
		}
		if taken != "" {
			return &NameTakenError{Existing: taken}
		}
		if !confirmed && domain.NormaliseIngredientName(l, current.Name) != normalised {
			known, err := LoadIngredients(ctx, tx)
			if err != nil {
				return err
			}
			others := slices.DeleteFunc(known, func(k domain.Ingredient) bool { return k.ID == id })
			if closest := closeTo(l, ing.Name, others); closest != "" {
				return &SuggestedError{Closest: closest}
			}
		}
		if ing.Unit != current.Unit {
			moved, err := ledger.HasMovements(ctx, tx, id)
			if err != nil {
				return err
			}
			if moved {
				return &UnitFixedError{Unit: current.Unit}
			}
		}
		if ing.Staple && !current.Staple {
			if counted, err = ledger.CountToNothing(ctx, tx, at, id); err != nil {
				return err
			}
		}
		return updateIngredient(ctx, tx, id, ing, normalised)
	})
	if err != nil {
		return nil, err
	}
	return counted, nil
}

// Delete deletes the signed-in User's Ingredient id, and is its name. One that Movements
// refer to is ErrHasMovements, which the database also holds to, and one they haven't got
// is ErrNoIngredient.
func (s *Service) Delete(ctx context.Context, id int64) (name string, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ing, ok, err := LoadIngredient(ctx, tx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNoIngredient
		}
		moved, err := ledger.HasMovements(ctx, tx, id)
		if err != nil {
			return err
		}
		if moved {
			return ErrHasMovements
		}
		name = ing.Name
		return deleteIngredient(ctx, tx, id)
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == foreignKeyViolation {
		return "", ErrHasMovements
	}
	return name, err
}

// foreignKeyViolation is Postgres's code for a row deleted while another refers to it.
const foreignKeyViolation = "23503"

// Suggest is the name of the signed-in User's Ingredient that name is close to, which
// they may have meant, or "" if there is none.
func (s *Service) Suggest(ctx context.Context, l domain.Language, name string) (string, error) {
	known, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	return closeTo(l, name, known), nil
}

// closeTo is the name of the one of known that name is close to without being the
// same food, or "" if there is none.
func closeTo(l domain.Language, name string, known []domain.Ingredient) string {
	closest, ok := domain.ClosestIngredient(l, name, names(known))
	if !ok || domain.NormaliseIngredientName(l, closest) == domain.NormaliseIngredientName(l, name) {
		return ""
	}
	return closest
}

// List is the signed-in User's Ingredients by name.
func (s *Service) List(ctx context.Context) (ings []domain.Ingredient, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ings, err = LoadIngredients(ctx, tx)
		return err
	})
	return ings, err
}

// Find is the signed-in User's Ingredient id; ok is false if they have none.
func (s *Service) Find(ctx context.Context, id int64) (ing domain.Ingredient, ok bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ing, ok, err = LoadIngredient(ctx, tx, id)
		return err
	})
	return ing, ok, err
}

// Search is the signed-in User's Ingredients a search for query finds by spelling, the
// most alike first, so a typo still finds the Ingredient: every one, by name, for a blank
// query. Every feature that finds an Ingredient as the User types searches here.
func (s *Service) Search(ctx context.Context, query string) ([]domain.Ingredient, error) {
	ings, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return search(query, ings), nil
}

// Listing is what Search finds for query, with the ids of the signed-in User's
// Ingredients some Movement refers to, read together, for the Ingredients' list.
func (s *Service) Listing(ctx context.Context, query string) (found []domain.Ingredient, withMovements map[int64]bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ings, err := LoadIngredients(ctx, tx)
		if err != nil {
			return err
		}
		found = search(query, ings)
		withMovements, err = ledger.WithMovements(ctx, tx)
		return err
	})
	return found, withMovements, err
}

// search is the ones of ings a search for query finds by spelling, the most alike first.
func search(query string, ings []domain.Ingredient) []domain.Ingredient {
	var found []domain.Ingredient
	for _, i := range domain.SearchIngredients(query, names(ings)) {
		found = append(found, ings[i])
	}
	return found
}

// names are ings' names, in their order.
func names(ings []domain.Ingredient) []string {
	ns := make([]string, len(ings))
	for i, ing := range ings {
		ns[i] = ing.Name
	}
	return ns
}
