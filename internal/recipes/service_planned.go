package recipes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
)

// The planner's Recipe Drafts: drafted in a planning Run, stored with the Plan Proposal
// that prepares them, accepted into the Recipe Library when it is approved, and never
// listed with the User's own Recipe Drafts.

// Drafted is a planner's Recipe Draft: its id once stored with its Plan Proposal, and the
// Recipe it drafts, its new Ingredients without an id.
type Drafted struct {
	ID     int64
	Recipe domain.Recipe
	doc    []byte
}

// ClashError is approving a Plan Proposal whose Recipe Draft Name no longer fits the
// Recipe Library: a Recipe of its name, or an Ingredient it can't be saved with, came
// since it was drafted.
type ClashError struct{ Name string }

func (e *ClashError) Error() string {
	return fmt.Sprintf("the Recipe Draft %q no longer fits the Recipe Library", e.Name)
}

// DraftSchema is draft_recipe's input: a Recipe as submit_recipe takes it.
var DraftSchema = submitRecipeSchema

// ReadDraft reads input, a Recipe the planner drafts for a User speaking l, against their
// Ingredients, known, and their Recipe Library, or is why it can't be drafted: what
// submit_recipe says, and a name already in the Recipe Library.
func ReadDraft(l domain.Language, known []domain.Ingredient, library []domain.Recipe, input json.RawMessage) (Drafted, error) {
	var p parsedRecipe
	if err := json.Unmarshal(input, &p); err != nil {
		return Drafted{}, fmt.Errorf("the Recipe does not match the schema: %w", err)
	}
	rec, problems := p.recipe(l, known)
	for _, r := range library {
		if domain.NormaliseRecipeName(r.Name) == domain.NormaliseRecipeName(rec.Name) {
			problems["name"] = fmt.Sprintf("the Recipe Library already has %s, Recipe %d: prepare that one, or draft another name", r.Name, r.ID)
		}
	}
	if err := fixThese(problems); err != nil {
		return Drafted{}, err
	}
	doc, err := json.Marshal(p)
	return Drafted{Recipe: rec, doc: doc}, err
}

// SaveDrafted stores d as a Recipe Draft of the Plan Proposal proposalID, in the caller's
// transaction, and is its id.
func SaveDrafted(ctx context.Context, q db.Querier, proposalID int64, d Drafted) (int64, error) {
	return storePlanned(ctx, q, proposalID, d.doc)
}

// LoadDrafted are the Recipe Drafts of the signed-in User's Plan Proposal proposalID, in
// the order drafted, each Ingredient resolved by name to theirs, for them speaking l.
func LoadDrafted(ctx context.Context, q db.Querier, l domain.Language, proposalID int64) ([]Drafted, error) {
	drafted, err := loadPlanned(ctx, q, proposalID)
	if err != nil || len(drafted) == 0 {
		return drafted, err
	}
	known, err := ingredients.LoadIngredients(ctx, q)
	if err != nil {
		return nil, err
	}
	for i := range drafted {
		if drafted[i].Recipe, _, err = drafted[i].read(l, known); err != nil {
			return nil, err
		}
	}
	return drafted, nil
}

// read is d's Recipe for a User speaking l, resolved against known, and its problems.
func (d Drafted) read(l domain.Language, known []domain.Ingredient) (domain.Recipe, map[string]string, error) {
	var p parsedRecipe
	if err := json.Unmarshal(d.doc, &p); err != nil {
		return domain.Recipe{}, nil, err
	}
	rec, problems := p.recipe(l, known)
	return rec, problems, nil
}

// AcceptDrafted adds each Recipe Draft of the signed-in User's Plan Proposal proposalID,
// who speaks l, to their Recipe Library as drafted, with its new Ingredients, and deletes
// it, in tx; accepted maps each Recipe Draft's id to its Recipe's. One that no longer
// fits the Recipe Library is a *ClashError.
func AcceptDrafted(ctx context.Context, tx pgx.Tx, l domain.Language, proposalID int64) (accepted map[int64]int64, err error) {
	drafted, err := deletePlanned(ctx, tx, proposalID)
	if err != nil {
		return nil, err
	}
	accepted = map[int64]int64{}
	for _, d := range drafted {
		known, err := ingredients.LoadIngredients(ctx, tx) // again: an earlier one may have added an Ingredient this one uses
		if err != nil {
			return nil, err
		}
		rec, problems, err := d.read(l, known)
		if err != nil {
			return nil, err
		}
		if len(problems) > 0 {
			return nil, &ClashError{Name: rec.Name}
		}
		id, err := addIn(ctx, tx, l, rec)
		if _, taken := errors.AsType[*NameTakenError](err); taken {
			return nil, &ClashError{Name: rec.Name}
		}
		if _, taken := errors.AsType[*ingredients.NameTakenError](err); taken {
			return nil, &ClashError{Name: rec.Name}
		}
		if err != nil {
			return nil, err
		}
		accepted[d.ID] = id
	}
	return accepted, nil
}
