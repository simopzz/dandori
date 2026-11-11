package recipes

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// ErrNoDraft is a Recipe Draft the signed-in User hasn't got.
var ErrNoDraft = errors.New("the User has no such Recipe Draft")

// StartParse starts a parse Run reading text, pasted by the signed-in User speaking l,
// into a Recipe Draft, and is the Run's id.
func (s *Service) StartParse(ctx context.Context, l domain.Language, text string) (int64, error) {
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return 0, err
	}
	return s.runs.Start(ctx, ParseKind, text, s.parseBrief(l, text, known))
}

// saveDraft stores p as the Recipe Draft of the Run runID, replacing any earlier one.
func (s *Service) saveDraft(ctx context.Context, runID int64, p parsedRecipe) error {
	doc, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return storeDraft(ctx, tx, runID, doc) })
}

// DraftOf is the id of the Recipe Draft the signed-in User's parse Run runID left. It is
// ErrNoDraft if the Run left none.
func (s *Service) DraftOf(ctx context.Context, runID int64) (int64, error) {
	var id int64
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		id, err = loadDraftID(ctx, tx, runID)
		return err
	})
	return id, err
}

// FindDraft is the signed-in User's Recipe Draft id, for them speaking l, each
// Ingredient resolved by name to theirs. It is ErrNoDraft unless its Run has succeeded,
// and when there is no such Recipe Draft.
func (s *Service) FindDraft(ctx context.Context, l domain.Language, id int64) (domain.Recipe, error) {
	var doc []byte
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		doc, err = loadDraft(ctx, tx, id)
		return err
	})
	if err != nil {
		return domain.Recipe{}, err
	}
	var p parsedRecipe
	if err := json.Unmarshal(doc, &p); err != nil {
		return domain.Recipe{}, err
	}
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return domain.Recipe{}, err
	}
	rec, _ := p.recipe(l, known)
	return rec, nil
}

// Draft is a Recipe Draft awaiting the User's check: its name, and when its Run started.
type Draft struct {
	ID        int64
	Name      string
	StartedAt time.Time
}

// Drafts are the signed-in User's Recipe Drafts whose Runs have succeeded, newest first.
func (s *Service) Drafts(ctx context.Context) ([]Draft, error) {
	var drafts []Draft
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		drafts, err = loadDrafts(ctx, tx)
		return err
	})
	return drafts, err
}

// Accept adds rec, which has no Problems and is what the User made of their Recipe Draft
// id, as Add does, and deletes the Recipe Draft, all or nothing. A Recipe Draft they
// haven't got, or whose Run is going, is ErrNoDraft.
func (s *Service) Accept(ctx context.Context, l domain.Language, id int64, rec domain.Recipe) (recipeID int64, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := deleteDraft(ctx, tx, id); err != nil {
			return err
		}
		recipeID, err = addIn(ctx, tx, l, rec)
		return err
	})
	return recipeID, err
}

// Discard deletes the signed-in User's Recipe Draft id and is its name, or ErrNoDraft.
func (s *Service) Discard(ctx context.Context, id int64) (name string, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		name, err = deleteDraft(ctx, tx, id)
		return err
	})
	return name, err
}
