// Package recipes is the Recipe Library: the Recipes a User writes by hand and the one
// form every Recipe is written with.
package recipes

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
)

// NameTakenError is a Recipe refused because the User has one of that name already, the
// Recipe ID named Existing.
type NameTakenError struct {
	ID       int64
	Existing string
}

func (e *NameTakenError) Error() string {
	return fmt.Sprintf("the User already has the Recipe %q", e.Existing)
}

// ErrNoRecipe is a Recipe the signed-in User hasn't got.
var ErrNoRecipe = errors.New("the User has no such Recipe")

// Service is the signed-in User's Recipes.
type Service struct {
	pool        *pgxpool.Pool
	ingredients *ingredients.Service
}

// New creates the Service.
func New(pool *pgxpool.Pool, ings *ingredients.Service) *Service {
	return &Service{pool: pool, ingredients: ings}
}

// Add adds rec, which has no Problems, for the signed-in User, whose Language is l, and
// its new Ingredients, all or nothing, and is its id. A name another of theirs has is a
// *NameTakenError. A new Ingredient the User has added since the form was read comes back
// as the *ingredients.NameTakenError it is.
func (s *Service) Add(ctx context.Context, l domain.Language, rec domain.Recipe) (id int64, err error) {
	normalised := domain.NormaliseRecipeName(rec.Name)
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		var added bool
		if id, added, err = saveRecipe(ctx, tx, rec, normalised); err != nil {
			return err
		}
		if !added {
			taken, existing, err := loadTaken(ctx, tx, normalised)
			if err != nil {
				return err
			}
			return &NameTakenError{ID: taken, Existing: existing}
		}
		for i, ri := range rec.Ingredients {
			if ri.Ingredient.ID != 0 {
				continue
			}
			if rec.Ingredients[i].Ingredient.ID, err = ingredients.AddIn(ctx, tx, l, ri.Ingredient); err != nil {
				return err
			}
		}
		return saveRecipeIngredients(ctx, tx, id, rec)
	})
	return id, err
}

// List is the signed-in User's Recipes by name.
func (s *Service) List(ctx context.Context) (recipes []domain.Recipe, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		recipes, err = loadRecipes(ctx, tx, nil)
		return err
	})
	return recipes, err
}

// Find is the signed-in User's Recipe id, or ErrNoRecipe.
func (s *Service) Find(ctx context.Context, id int64) (domain.Recipe, error) {
	var recipes []domain.Recipe
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		recipes, err = loadRecipes(ctx, tx, &id)
		return err
	})
	if err != nil {
		return domain.Recipe{}, err
	}
	if len(recipes) == 0 {
		return domain.Recipe{}, ErrNoRecipe
	}
	return recipes[0], nil
}

// KnownIngredients is the signed-in User's Ingredients, which the form resolves names to.
func (s *Service) KnownIngredients(ctx context.Context) ([]domain.Ingredient, error) {
	return s.ingredients.List(ctx)
}
