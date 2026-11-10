package recipes

import (
	"context"
	"encoding/json"
	"errors"

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
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO recipe_draft (run_id, document) VALUES ($1, $2)
			ON CONFLICT (run_id) DO UPDATE SET document = excluded.document`, runID, doc)
		return err
	})
}

// DraftOf is the id of the Recipe Draft the signed-in User's parse Run runID left. It is
// ErrNoDraft if the Run left none.
func (s *Service) DraftOf(ctx context.Context, runID int64) (int64, error) {
	var id int64
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM recipe_draft WHERE run_id = $1`, runID).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoDraft
	}
	return id, err
}

// FindDraft is the signed-in User's Recipe Draft id, for them speaking l, each
// Ingredient resolved by name to theirs. It is ErrNoDraft while its Run is going, and
// when there is no such Recipe Draft.
func (s *Service) FindDraft(ctx context.Context, l domain.Language, id int64) (domain.Recipe, error) {
	var doc []byte
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		// Not while its Run goes, as the model may still change it.
		return tx.QueryRow(ctx, `
			SELECT d.document FROM recipe_draft d JOIN run r ON r.id = d.run_id
			WHERE d.id = $1 AND r.status <> 'going'`, id).Scan(&doc)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Recipe{}, ErrNoDraft
	}
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
