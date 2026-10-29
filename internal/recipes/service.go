// Package recipes is the Recipe Library: Recipes the user keeps, edits and
// deletes, and Recipe Drafts parsed by the agent from pasted text, which
// become Recipes only when the user accepts them.
package recipes

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/embedding"
	"github.com/simopzz/dandori/internal/runs"
	"github.com/simopzz/dandori/internal/stock"
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
	// version is when the Recipe last changed, so an embedding computed
	// from an older version isn't stored.
	version time.Time
}

// Service is the Recipe Library's use cases.
type Service struct {
	pool     *pgxpool.Pool
	runs     *runs.Engine
	language domain.ContentLanguage
	embedder embedding.Embedder
	stock    *stock.Service
	today    func() time.Time
	matcher  IngredientMatcher
}

// Config is what the Recipe Library needs besides its database.
type Config struct {
	// Runs is where parse Runs go through.
	Runs *runs.Engine
	// Language is the Content Language food terms are written in.
	Language domain.ContentLanguage
	// Embedder embeds Recipes and searches; without one, searches only
	// filter.
	Embedder embedding.Embedder
	// Stock is the Stock Items that decide whether stock covers a Recipe.
	Stock *stock.Service
	// Today is the day it is, before which a Stock Item has expired.
	Today func() time.Time
	// Matcher suggests the known Ingredient a new one's name was meant to
	// be; TrigramMatcher when nil.
	Matcher IngredientMatcher
}

// New creates the Service.
func New(pool *pgxpool.Pool, cfg Config) *Service {
	matcher := cfg.Matcher
	if matcher == nil {
		matcher = TrigramMatcher{}
	}
	return &Service{pool: pool, runs: cfg.Runs, language: cfg.Language, embedder: cfg.Embedder, stock: cfg.Stock, today: cfg.Today, matcher: matcher}
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

// ClosestIngredient is the known Ingredient name, of those in known, that
// name, matching none, was most likely meant to be; ok is false when it is
// most likely a new Ingredient.
func (s *Service) ClosestIngredient(ctx context.Context, name string, known []string) (closest string, ok bool, err error) {
	return s.matcher.Closest(ctx, name, known)
}

// Update replaces the Recipe with id by rec, adding its new Ingredients.
func (s *Service) Update(ctx context.Context, id int64, rec domain.Recipe) error {
	if err := validate(rec); err != nil {
		return err
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := saveRecipe(ctx, tx, id, rec)
		return err
	})
	if err == nil {
		s.embedSaved(ctx)
	}
	return err
}

// embedSaveWait bounds how long saving a Recipe waits to embed it.
const embedSaveWait = 5 * time.Second

// embedSaved embeds a Recipe just saved. A failure leaves it without an
// embedding, which the next search computes: saving never fails, nor waits
// long, because the embedding provider does.
func (s *Service) embedSaved(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, embedSaveWait)
	defer cancel()
	_ = s.embedRecipes(ctx)
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
	id, found, err := draftOfRun(ctx, s.pool, runID)
	if err != nil || !found {
		return domain.Recipe{}, false, err
	}
	return s.DraftByID(ctx, id)
}

// DraftByID is the pending Recipe Draft with id, its Ingredients matched
// to the known ones. found is false when it was accepted or discarded.
func (s *Service) DraftByID(ctx context.Context, id int64) (rec domain.Recipe, found bool, err error) {
	p, found, err := loadDraft(ctx, s.pool, id)
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
	_, found, err := draftOfRun(ctx, s.pool, runID)
	return found, err
}

// AcceptDraft adds rec, the draft of the parse Run with runID as the user
// corrected it, to the Recipe Library. It returns the new Recipe's id.
func (s *Service) AcceptDraft(ctx context.Context, runID int64, rec domain.Recipe) (int64, error) {
	id, err := s.pendingDraftOf(ctx, runID)
	if err != nil {
		return 0, err
	}
	return s.AcceptDraftByID(ctx, id, rec)
}

// AcceptDraftByID adds rec, the Recipe Draft with id as the user corrected
// it, to the Recipe Library. It returns the new Recipe's id.
func (s *Service) AcceptDraftByID(ctx context.Context, id int64, rec domain.Recipe) (int64, error) {
	if err := validate(rec); err != nil {
		return 0, err
	}
	var recipeID int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		recipeID, err = acceptDraft(ctx, tx, id, rec)
		return err
	})
	if err == nil {
		s.embedSaved(ctx)
	}
	return recipeID, err
}

// acceptDraft adds rec to the Recipe Library as what the pending draft
// with id became.
func acceptDraft(ctx context.Context, q db.Querier, id int64, rec domain.Recipe) (int64, error) {
	if err := lockDraft(ctx, q, id); err != nil {
		return 0, err
	}
	recipeID, err := saveRecipe(ctx, q, 0, rec)
	if err != nil {
		return 0, err
	}
	return recipeID, markAccepted(ctx, q, id, recipeID)
}

// Discard drops the Recipe Draft of the parse Run with id.
func (s *Service) Discard(ctx context.Context, runID int64) error {
	id, err := s.pendingDraftOf(ctx, runID)
	if err != nil {
		return err
	}
	return s.DiscardByID(ctx, id)
}

// pendingDraftOf is the id of the parse Run with runID's draft, or
// ErrDraftGone once it was accepted or discarded.
func (s *Service) pendingDraftOf(ctx context.Context, runID int64) (int64, error) {
	id, found, err := draftOfRun(ctx, s.pool, runID)
	if err == nil && !found {
		err = ErrDraftGone
	}
	return id, err
}

// DiscardByID drops the pending Recipe Draft with id.
func (s *Service) DiscardByID(ctx context.Context, id int64) error {
	return deleteDraft(ctx, s.pool, id)
}

// Drafted is a Recipe Draft the planner drafted, as a Plan Proposal using
// it reads it.
type Drafted struct {
	ID int64
	// RunID is the Run that drafted it.
	RunID int64
	// Recipe is the draft, its Ingredients matched to the known ones.
	Recipe domain.Recipe
	// RecipeID is the Recipe it became once accepted; 0 while it is pending.
	RecipeID int64
}

// DraftsByID loads the Recipe Drafts with ids, by id, through q, so it reads
// them in the caller's transaction. A discarded one is missing.
func (s *Service) DraftsByID(ctx context.Context, q db.Querier, ids []int64) (map[int64]Drafted, error) {
	drafts, err := loadDrafts(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	known, err := loadIngredients(ctx, q)
	if err != nil {
		return nil, err
	}
	drafted := map[int64]Drafted{}
	for id, d := range drafts {
		drafted[id] = Drafted{ID: id, RunID: d.runID, Recipe: d.recipe.recipe(known), RecipeID: d.recipeID}
	}
	return drafted, nil
}

// AcceptDrafted adds the Recipe Drafts with ids still pending to the
// Recipe Library as drafted, through q, so it is part of the caller's
// transaction. It returns the Recipe each draft became, by draft id.
func (s *Service) AcceptDrafted(ctx context.Context, q db.Querier, ids []int64) (map[int64]int64, error) {
	drafted, err := s.DraftsByID(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	recipes := map[int64]int64{}
	for _, id := range ids {
		d, ok := drafted[id]
		switch {
		case !ok:
			return nil, ErrDraftGone
		case d.RecipeID == 0:
			if err := validate(d.Recipe); err != nil {
				return nil, err
			}
			if d.RecipeID, err = acceptDraft(ctx, q, id, d.Recipe); err != nil {
				return nil, err
			}
		}
		recipes[id] = d.RecipeID
	}
	return recipes, nil
}
