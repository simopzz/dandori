package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/recipes"
)

// proposedPlan is a Plan as a Plan Proposal stores it, the shape the
// planner submits: Recipes by id, Recipe Drafts by draft id, Ingredients
// by name, dates as YYYY-MM-DD.
type proposedPlan struct {
	// From is the first Slot the Plan Proposal replaces; the Cycle's first
	// when it is absent, as for the initial Plan.
	From *struct {
		On   string          `json:"on"`
		Kind domain.SlotKind `json:"kind"`
	} `json:"from"`
	PrepSessions []proposedPrepSession `json:"prep_sessions"`
	Kept         []proposedKept        `json:"kept"`

	from *domain.Slot
	// run is the planning Run that submitted it, whose Recipe Drafts are
	// the only ones it may prepare; 0 when no Run did.
	run int64
}

// start is the first Slot of c the Plan replaces.
func (p proposedPlan) start(c Cycle) (domain.Slot, error) {
	if p.from == nil {
		return c.Start, nil
	}
	if !c.Contains(*p.from) {
		return domain.Slot{}, unreadable("it starts at %s, outside its Cycle", *p.from)
	}
	return *p.from, nil
}

type proposedPrepSession struct {
	On       string          `json:"on"`
	Kind     domain.SlotKind `json:"kind"`
	RecipeID *int64          `json:"recipe_id"`
	// DraftID is the planner's Recipe Draft it prepares, when it isn't yet
	// in the Recipe Library.
	DraftID *int64 `json:"draft_id"`
	// Ingredients are set out as they are, when there is no Recipe.
	Ingredients []struct {
		Name     string  `json:"name"`
		Quantity float64 `json:"quantity"`
	} `json:"ingredients"`
	Portions []proposedPortion `json:"portions"`

	at domain.Slot
}

type proposedPortion struct {
	Location domain.StorageLocation `json:"location"`
	// EatOn and EatKind are the planned eating; empty when there is none.
	EatOn   string          `json:"eat_on"`
	EatKind domain.SlotKind `json:"eat_kind"`

	eatAt *domain.Slot
}

type proposedKept struct {
	PortionID int64           `json:"portion_id"`
	On        string          `json:"on"`
	Kind      domain.SlotKind `json:"kind"`

	slot domain.Slot
}

// UnreadableError means a Plan Proposal's Plan isn't one Dandori can read:
// a bad date, an unknown Recipe, Recipe Draft or Ingredient.
type UnreadableError struct{ Reason string }

func (e *UnreadableError) Error() string { return "unreadable plan proposal: " + e.Reason }

func unreadable(format string, args ...any) error {
	return &UnreadableError{Reason: fmt.Sprintf(format, args...)}
}

func readSlot(on string, kind domain.SlotKind) (domain.Slot, error) {
	d, err := time.Parse(time.DateOnly, on)
	if err != nil {
		return domain.Slot{}, unreadable("%q is not a date", on)
	}
	if !slices.Contains(domain.SlotKinds, kind) {
		return domain.Slot{}, unreadable("%q is not breakfast, lunch, snack or dinner", kind)
	}
	return domain.Slot{On: d, Kind: kind}, nil
}

// readPlan reads a stored Plan Proposal's Plan, checking its dates, eating
// occasions, Storage Locations and quantities, and normalising its
// Ingredient names.
func readPlan(raw []byte) (proposedPlan, error) {
	var p proposedPlan
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, unreadable("%v", err)
	}
	if p.From != nil {
		from, err := readSlot(p.From.On, p.From.Kind)
		if err != nil {
			return p, err
		}
		p.from = &from
	}
	var err error
	for i := range p.PrepSessions {
		ps := &p.PrepSessions[i]
		if ps.at, err = readSlot(ps.On, ps.Kind); err != nil {
			return p, err
		}
		prepares := 0
		for _, given := range []bool{ps.RecipeID != nil, ps.DraftID != nil, len(ps.Ingredients) > 0} {
			if given {
				prepares++
			}
		}
		if prepares > 1 {
			return p, unreadable("the Prep Session at %s has more than one of a Recipe, a Recipe Draft and Ingredients set out as they are", ps.at)
		}
		for j := range ps.Ingredients {
			ing := &ps.Ingredients[j]
			if ing.Name = domain.IngredientName(ing.Name); ing.Quantity <= 0 {
				return p, unreadable("%q is set out in a quantity of %g", ing.Name, ing.Quantity)
			}
		}
		for j := range ps.Portions {
			portion := &ps.Portions[j]
			if msg := portion.Location.Validate(); msg != "" {
				return p, unreadable("%q: %s", portion.Location, msg)
			}
			if portion.EatOn == "" {
				continue
			}
			eat, err := readSlot(portion.EatOn, portion.EatKind)
			if err != nil {
				return p, err
			}
			portion.eatAt = &eat
		}
	}
	for i := range p.Kept {
		if p.Kept[i].slot, err = readSlot(p.Kept[i].On, p.Kept[i].Kind); err != nil {
			return p, err
		}
	}
	return p, nil
}

// draftIDs are the Recipe Drafts p prepares, in order.
func (p proposedPlan) draftIDs() []int64 {
	var ids []int64
	for _, ps := range p.PrepSessions {
		if ps.DraftID != nil && !slices.Contains(ids, *ps.DraftID) {
			ids = append(ids, *ps.DraftID)
		}
	}
	return ids
}

// ownDrafts loads the Recipe Drafts p prepares, by id, leaving out those
// discarded and those another Run drafted: a Plan Proposal prepares only
// the drafts of the Run that submitted it.
func (s *Service) ownDrafts(ctx context.Context, q db.Querier, p proposedPlan) (map[int64]recipes.Drafted, error) {
	drafts, err := s.recipes.DraftsByID(ctx, q, p.draftIDs())
	maps.DeleteFunc(drafts, func(_ int64, d recipes.Drafted) bool { return p.run == 0 || d.RunID != p.run })
	return drafts, err
}

// resolve turns p into a domain.Plan, with its Recipes and Ingredients. A
// Recipe Draft prepares the Recipe it became once accepted, and itself
// while it is pending.
func (s *Service) resolve(ctx context.Context, q db.Querier, p proposedPlan) (domain.Plan, error) {
	var recipeIDs []int64
	var names []string
	for _, ps := range p.PrepSessions {
		if ps.RecipeID != nil {
			recipeIDs = append(recipeIDs, *ps.RecipeID)
		}
		for _, ing := range ps.Ingredients {
			names = append(names, ing.Name)
		}
	}
	drafts, err := s.ownDrafts(ctx, q, p)
	if err != nil {
		return domain.Plan{}, err
	}
	for _, d := range drafts {
		if d.RecipeID != 0 {
			recipeIDs = append(recipeIDs, d.RecipeID)
		}
	}
	recipes, err := loadRecipes(ctx, q, recipeIDs)
	if err != nil {
		return domain.Plan{}, err
	}
	ingredients, err := loadIngredients(ctx, q, names)
	if err != nil {
		return domain.Plan{}, err
	}

	var plan domain.Plan
	for _, ps := range p.PrepSessions {
		session := domain.PrepSession{At: ps.at}
		if ps.RecipeID != nil {
			if session.Recipe = recipes[*ps.RecipeID]; session.Recipe == nil {
				return plan, unreadable("there is no Recipe %d", *ps.RecipeID)
			}
		}
		if ps.DraftID != nil {
			d, ok := drafts[*ps.DraftID]
			switch {
			case !ok:
				return plan, unreadable("there is no Recipe Draft %d of the planning Run that proposed it: it was discarded, or never drafted there", *ps.DraftID)
			case d.RecipeID != 0:
				if session.Recipe = recipes[d.RecipeID]; session.Recipe == nil {
					return plan, unreadable("there is no Recipe %d", d.RecipeID)
				}
			default:
				session.Recipe = &d.Recipe
			}
		}
		for _, ing := range ps.Ingredients {
			known, ok := ingredients[ing.Name]
			if !ok {
				return plan, unreadable("there is no Ingredient %q", ing.Name)
			}
			session.Ingredients = append(session.Ingredients, domain.RecipeIngredient{Ingredient: known, Quantity: ing.Quantity})
		}
		for _, portion := range ps.Portions {
			session.Portions = append(session.Portions, domain.PlannedPortion{Location: portion.Location, EatAt: portion.eatAt})
		}
		plan.PrepSessions = append(plan.PrepSessions, session)
	}
	for _, k := range p.Kept {
		plan.Kept = append(plan.Kept, domain.KeptAssignment{PortionID: k.PortionID, Slot: k.slot})
	}
	return plan, nil
}
