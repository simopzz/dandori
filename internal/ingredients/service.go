// Package ingredients is the User's Ingredients: adding them by hand and listing them, in
// the side sheet any page can open. Each food is one Ingredient, identified by its name
// as domain.NormaliseIngredientName reduces it in the User's Language.
package ingredients

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// NameTakenError is an Ingredient refused because the User has one of that food already,
// named Existing.
type NameTakenError struct{ Existing string }

func (e *NameTakenError) Error() string {
	return fmt.Sprintf("the User already has the Ingredient %q", e.Existing)
}

// Service is the signed-in User's Ingredients.
type Service struct {
	pool *pgxpool.Pool
}

// New creates the Service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Add adds ing, which has no Problems, for the signed-in User, whose Language is l. One
// whose name normalises as another's is a *NameTakenError.
func (s *Service) Add(ctx context.Context, l domain.Language, ing domain.Ingredient) error {
	normalised := domain.NormaliseIngredientName(l, ing.Name)
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		added, err := saveIngredient(ctx, tx, ing, normalised)
		if err != nil || added {
			return err
		}
		existing, err := loadNameOf(ctx, tx, normalised)
		if err != nil {
			return err
		}
		return &NameTakenError{Existing: existing}
	})
}

// List is the signed-in User's Ingredients by name.
func (s *Service) List(ctx context.Context) (ings []domain.Ingredient, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		ings, err = loadIngredients(ctx, tx)
		return err
	})
	return ings, err
}
