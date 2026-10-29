package recipes

import (
	"context"

	"github.com/simopzz/dandori/internal/domain"
)

// IngredientMatcher decides which known Ingredient a name that matches none
// was meant to be, or that it is a new one. It only suggests: the user
// accepts or ignores what it finds.
type IngredientMatcher interface {
	// Closest is the name in known that name most likely means; ok is false
	// when name is most likely a new Ingredient.
	Closest(ctx context.Context, name string, known []string) (closest string, ok bool, err error)
}

// TrigramMatcher matches names by spelling, so a typo finds the Ingredient
// it misspells.
type TrigramMatcher struct{}

func (TrigramMatcher) Closest(_ context.Context, name string, known []string) (string, bool, error) {
	closest, ok := domain.ClosestIngredient(name, known)
	return closest, ok, nil
}
