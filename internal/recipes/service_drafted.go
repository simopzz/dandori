package recipes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
)

// The planner's Recipe Drafts: drafted in a planning Run and stored by planning with the
// Plan Proposal that prepares them, accepted into the Recipe Library when it is approved,
// and never listed with the User's own Recipe Drafts.

// Drafted is a planner's Recipe Draft: its id once stored with its Plan Proposal, and the
// Recipe it drafts, its new Ingredients without an id.
type Drafted struct {
	ID     int64
	Recipe domain.Recipe
	doc    []byte
}

// ClashError is approving a Plan Proposal whose Recipe Draft Name no longer fits the
// Recipe Library or the Ingredients, changed since it was drafted: a Recipe of its name
// added, or an Ingredient it reuses deleted or one it adds taken.
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
	for i, ri := range rec.Ingredients {
		if !ri.Ingredient.IsNew() {
			p.Ingredients[i].New = nil // values proposed for a known Ingredient: dropped, so approving won't read them as a clash
		}
	}
	doc, err := json.Marshal(p)
	return Drafted{Recipe: rec, doc: doc}, err
}

// Document is d as stored, for ReadStored to read back.
func (d Drafted) Document() []byte { return d.doc }

// ReadStored is the Recipe Draft id stored as doc, its Recipe read for a User speaking l,
// each Ingredient resolved by name to theirs, known.
func ReadStored(l domain.Language, known []domain.Ingredient, id int64, doc []byte) (Drafted, error) {
	d := Drafted{ID: id, doc: doc}
	var p parsedRecipe
	err := json.Unmarshal(doc, &p)
	d.Recipe, _ = p.recipe(l, known)
	return d, err
}

// AcceptDrafted adds each of drafted, a Plan Proposal's Recipe Drafts for the signed-in
// User, who speaks l, to their Recipe Library as drafted, with its new Ingredients, in
// the caller's transaction; accepted maps each Recipe Draft's id to its Recipe's. One
// that no longer fits the Recipe Library is a *ClashError.
func AcceptDrafted(ctx context.Context, q db.Querier, l domain.Language, drafted []Drafted) (accepted map[int64]int64, err error) {
	accepted = map[int64]int64{}
	added := map[string]bool{} // the Ingredients an earlier one added, by normalised name
	for _, d := range drafted {
		known, err := ingredients.LoadIngredients(ctx, q) // again: an earlier one may have added an Ingredient this one uses
		if err != nil {
			return nil, err
		}
		var p parsedRecipe
		if err := json.Unmarshal(d.doc, &p); err != nil {
			return nil, err
		}
		rec, problems := p.recipe(l, known)
		if len(problems) > 0 {
			return nil, &ClashError{Name: rec.Name}
		}
		for i, pi := range p.Ingredients {
			name := domain.NormaliseIngredientName(l, pi.Name)
			if pi.New != nil && !rec.Ingredients[i].Ingredient.IsNew() && !added[name] {
				return nil, &ClashError{Name: rec.Name} // the User has added it since, with values of their own
			}
		}
		for _, ri := range rec.Ingredients { // before addIn, which gives them their ids
			if ri.Ingredient.IsNew() {
				added[domain.NormaliseIngredientName(l, ri.Ingredient.Name)] = true
			}
		}
		id, err := addIn(ctx, q, l, rec)
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
