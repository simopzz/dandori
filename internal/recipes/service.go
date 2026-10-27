// Package recipes is the Recipe Library: Recipes the user keeps, edits and
// deletes, and Recipe Drafts parsed by the agent from pasted text, which
// become Recipes only when the user accepts them.
package recipes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/runs"
)

// ErrNotFound means there is no Recipe with that id.
var ErrNotFound = errors.New("no such recipe")

// ErrHasPortions means a Recipe can't be deleted because Portions of it
// are still kept.
var ErrHasPortions = errors.New("recipe has portions")

// ErrDraftGone means the draft was already accepted or discarded.
var ErrDraftGone = errors.New("no such draft")

// InvalidError means a Recipe breaks the domain rules. Fields is what is
// wrong, keyed by domain field path as domain.Recipe.Validate gives it.
type InvalidError struct{ Fields map[string]string }

func (e *InvalidError) Error() string { return "invalid recipe" }

func validate(rec domain.Recipe) error {
	if invalid := rec.Validate(); len(invalid) > 0 {
		return &InvalidError{Fields: invalid}
	}
	return nil
}

// Stored is a Recipe in the Recipe Library.
type Stored struct {
	ID int64
	domain.Recipe
}

// Service is the Recipe Library's use cases.
type Service struct {
	pool     *pgxpool.Pool
	runs     *runs.Engine
	language domain.ContentLanguage
}

// New creates the Service; parse Runs go through engine and write food
// terms in language.
func New(pool *pgxpool.Pool, engine *runs.Engine, language domain.ContentLanguage) *Service {
	return &Service{pool: pool, runs: engine, language: language}
}

// snapshot reads several queries as of one moment, so a Recipe saved or
// deleted meanwhile is either wholly there or not at all.
var snapshot = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// List loads every Recipe, newest first, with its Ingredients.
func (s *Service) List(ctx context.Context) ([]Stored, error) {
	var recipes []Stored
	err := pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		var err error
		recipes, err = loadRecipes(ctx, tx, nil)
		return err
	})
	return recipes, err
}

// Get loads the Recipe with id, with its Ingredients.
func (s *Service) Get(ctx context.Context, id int64) (Stored, error) {
	var recipes []Stored
	err := pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		var err error
		recipes, err = loadRecipes(ctx, tx, &id)
		return err
	})
	if err != nil {
		return Stored{}, err
	}
	if len(recipes) == 0 {
		return Stored{}, ErrNotFound
	}
	return recipes[0], nil
}

// KnownIngredients loads every Ingredient in the Recipe Library, by name.
func (s *Service) KnownIngredients(ctx context.Context) (map[string]domain.Ingredient, error) {
	return loadIngredients(ctx, s.pool)
}

// Update replaces the Recipe with id by rec, adding its new Ingredients.
func (s *Service) Update(ctx context.Context, id int64, rec domain.Recipe) error {
	if err := validate(rec); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := saveRecipe(ctx, tx, id, rec)
		return err
	})
}

// Delete deletes the Recipe with id, if there is one. It returns
// ErrHasPortions while a Prep Session of it has Portions.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return deleteRecipe(ctx, s.pool, id)
}

// StartParse starts a Run parsing the pasted text into a Recipe Draft. The
// draft takes the Run's id.
func (s *Service) StartParse(ctx context.Context, text string) (int64, error) {
	known, err := loadIngredients(ctx, s.pool)
	if err != nil {
		return 0, err
	}
	return s.runs.Start(ctx, recipeParseKind, s.parseJob(text, known))
}

// ParseRun loads the parse Run with id and its Trace, for while its draft
// is pending. found is false when there is no such parse Run.
func (s *Service) ParseRun(ctx context.Context, id int64) (run runs.Run, found bool, err error) {
	run, found, err = s.runs.Get(ctx, id)
	if err != nil || !found || run.Kind != recipeParseKind {
		return runs.Run{}, false, err
	}
	run.Events, err = s.runs.Trace(ctx, id, 0)
	if err != nil {
		return runs.Run{}, false, err
	}
	return run, true, nil
}

// Draft is the Recipe Draft of the parse Run with id, its Ingredients
// matched to the known ones. found is false while the Run goes on, since
// the model may still change the draft, and when there is no draft.
func (s *Service) Draft(ctx context.Context, runID int64) (rec domain.Recipe, found bool, err error) {
	if s.runs.Live(runID) {
		return domain.Recipe{}, false, nil
	}
	p, found, err := loadDraft(ctx, s.pool, runID)
	if err != nil || !found {
		return domain.Recipe{}, false, err
	}
	known, err := loadIngredients(ctx, s.pool)
	if err != nil {
		return domain.Recipe{}, false, err
	}
	return p.recipe(known), true, nil
}

// HasDraft reports whether the Recipe Draft of the Run with id is still
// waiting to be accepted or discarded.
func (s *Service) HasDraft(ctx context.Context, runID int64) (bool, error) {
	_, found, err := loadDraft(ctx, s.pool, runID)
	return found, err
}

// AcceptDraft adds rec, the draft as the user corrected it, to the Recipe
// Library, and drops the draft. It returns the new Recipe's id.
func (s *Service) AcceptDraft(ctx context.Context, runID int64, rec domain.Recipe) (int64, error) {
	if err := validate(rec); err != nil {
		return 0, err
	}
	var id int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Deleting the draft first locks it, so it is accepted only once.
		if err := deleteDraft(ctx, tx, runID); err != nil {
			return err
		}
		var err error
		id, err = saveRecipe(ctx, tx, 0, rec)
		return err
	})
	return id, err
}

// Discard drops the Recipe Draft of the Run with id.
func (s *Service) Discard(ctx context.Context, runID int64) error {
	return deleteDraft(ctx, s.pool, runID)
}
