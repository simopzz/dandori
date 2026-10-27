package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// proposedPlan is a Plan as a Plan Proposal stores it, the shape the
// planner submits: Recipes by id, Ingredients by name, dates as
// YYYY-MM-DD.
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
}

// start is the first Slot of c the Plan replaces.
func (p proposedPlan) start(c Cycle) (domain.Slot, error) {
	if p.from == nil {
		return c.firstSlot(), nil
	}
	if !c.inCycle(p.from.On) {
		return domain.Slot{}, unreadable("it starts at %s, outside its Cycle", *p.from)
	}
	return *p.from, nil
}

type proposedPrepSession struct {
	On       string          `json:"on"`
	Kind     domain.SlotKind `json:"kind"`
	RecipeID *int64          `json:"recipe_id"`
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
// a bad date, an unknown Recipe or Ingredient.
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
// occasions and Storage Locations.
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

// resolve turns p into a domain.Plan, with its Recipes and Ingredients.
func (p proposedPlan) resolve(ctx context.Context, q db.Querier) (domain.Plan, error) {
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
