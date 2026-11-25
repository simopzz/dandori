// Package recipes is the Recipe Library: the Recipes a User writes by hand or pastes, the
// Recipe Drafts parse Runs read pasted text into, and the one form every Recipe is
// written with.
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
	"github.com/simopzz/dandori/internal/runs"
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

// ErrInPlan is deleting a Recipe a Plan or a Plan Proposal prepares.
var ErrInPlan = errors.New("a Plan or a Plan Proposal prepares the Recipe")

// Prepares reports whether a Plan or a Plan Proposal of the signed-in User's prepares the
// Recipe id. The planning feature, which depends on this one, reads it, and app wires it.
type Prepares func(ctx context.Context, q db.Querier, id int64) (bool, error)

// Service is the signed-in User's Recipes.
type Service struct {
	pool        *pgxpool.Pool
	ingredients *ingredients.Service
	runs        *runs.Engine
	prepares    Prepares
}

// New creates the Service, its parse Runs going through `engine`.
func New(pool *pgxpool.Pool, ings *ingredients.Service, engine *runs.Engine, prepares Prepares) *Service {
	return &Service{pool: pool, ingredients: ings, runs: engine, prepares: prepares}
}

// Add adds `rec`, which has no Problems, for the signed-in User, whose Language is `l`,
// and its new Ingredients, all or nothing, and is its `id`. A name another of theirs has
// is a *NameTakenError. A new Ingredient the User has added since the form was read comes
// back as the *ingredients.NameTakenError it is.
func (s *Service) Add(ctx context.Context, l domain.Language, rec domain.Recipe) (id int64, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		id, err = addIn(ctx, tx, l, rec)
		return err
	})
	return id, err
}

// addIn is Add in the caller's transaction.
func addIn(ctx context.Context, q db.Querier, l domain.Language, rec domain.Recipe) (int64, error) {
	normalised := domain.NormaliseRecipeName(rec.Name)
	id, added, err := saveRecipe(ctx, q, rec, normalised)
	if err != nil {
		return 0, err
	}
	if !added {
		taken, existing, err := loadTaken(ctx, q, normalised, 0)
		if err != nil {
			return 0, err
		}
		return 0, &NameTakenError{ID: taken, Existing: existing}
	}
	return id, saveIngredients(ctx, q, l, id, rec)
}

// Edit sets the signed-in User's Recipe `id`, whose Language is `l`, to `rec`, which has
// no Problems, adding its new Ingredients, all or nothing. Only the Recipe changes: what
// is already recorded keeps its own name and values. A name another of theirs has is a
// *NameTakenError, a new Ingredient added since the form was read an
// *ingredients.NameTakenError, and one they haven't got is ErrNoRecipe.
func (s *Service) Edit(ctx context.Context, l domain.Language, id int64, rec domain.Recipe) error {
	normalised := domain.NormaliseRecipeName(rec.Name)
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		taken, existing, err := loadTaken(ctx, tx, normalised, id)
		if err != nil {
			return err
		}
		if taken != 0 {
			return &NameTakenError{ID: taken, Existing: existing}
		}
		if err := updateRecipe(ctx, tx, id, rec, normalised); err != nil {
			return err
		}
		if err := deleteRecipeIngredients(ctx, tx, id); err != nil {
			return err
		}
		return saveIngredients(ctx, tx, l, id, rec)
	})
}

// Delete deletes the signed-in User's Recipe `id` with its own rows, freeing its
// Ingredients, and is its `name`, or ErrNoRecipe. One a Plan or a Plan Proposal prepares
// is ErrInPlan, with its `name`.
func (s *Service) Delete(ctx context.Context, id int64) (name string, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		if name, err = deleteRecipe(ctx, tx, id); err != nil {
			return err
		}
		prepared, err := s.prepares(ctx, tx, id)
		if err == nil && prepared {
			return ErrInPlan // rolls the delete back
		}
		return err
	})
	return name, err
}

// saveIngredients adds `rec`'s new Ingredients for a User speaking `l`, in the caller's
// transaction, and lists all of them on the Recipe `id`.
func saveIngredients(ctx context.Context, q db.Querier, l domain.Language, id int64, rec domain.Recipe) (err error) {
	for i, ri := range rec.Ingredients {
		if !ri.Ingredient.IsNew() {
			continue
		}
		if rec.Ingredients[i].Ingredient.ID, err = ingredients.AddIn(ctx, q, l, ri.Ingredient); err != nil {
			return err
		}
	}
	return saveRecipeIngredients(ctx, q, id, rec)
}

// Search is the signed-in User's Recipes whose names a search for `query` finds, most
// alike first, or all of them by name for a blank `query`.
func (s *Service) Search(ctx context.Context, query string) ([]domain.Recipe, error) {
	var recipes []domain.Recipe
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		recipes, err = loadRecipes(ctx, tx, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	names := make([]string, len(recipes))
	for i, rec := range recipes {
		names[i] = rec.Name
	}
	found := []domain.Recipe{}
	for _, i := range domain.SearchNames(query, names) {
		found = append(found, recipes[i])
	}
	return found, nil
}

// Find is the signed-in User's Recipe `id`, or ErrNoRecipe.
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
