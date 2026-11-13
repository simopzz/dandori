package domain

import (
	"math"
	"strings"
)

// A Recipe's own reasons for refusing a value, beside those of a typed one.
const (
	NotAboveZero Problem = "not above zero"
	ListedTwice  Problem = "listed twice"
	AboveTotal   Problem = "above the total minutes"
)

// maxMinutes bounds a Recipe's minutes: a week covers even a long ferment.
const maxMinutes = 7 * 24 * 60

// RecipeIngredient is a quantity of an Ingredient in a Recipe, in the Ingredient's Unit.
type RecipeIngredient struct {
	Ingredient Ingredient
	Quantity   Quantity
}

// Recipe is a reusable description of a dish. Portions is how many its Ingredients make
// as written. An Ingredient with no ID is new, to be created with the Recipe.
type Recipe struct {
	ID                          int64
	Name                        string
	Portions                    int
	ActiveMinutes, TotalMinutes int
	Kinds                       []SlotKind
	Appliances, Steps           []string
	Ingredients                 []RecipeIngredient
}

// PortionNutrition is one Portion's nutritional values as a Recipe is written. FibreFloor
// and SugarFloor say the value is at least that much, since some of its Ingredients don't
// know it; when none does, it is Unknown.
type PortionNutrition struct {
	Nutrition
	FibreFloor, SugarFloor bool
}

// ParseMinutes reads minutes as the User typed them: a whole number of at least 0, and
// no more than maxMinutes.
func ParseMinutes(typed string) (int, Problem) {
	n, problem := parseWhole(typed)
	switch {
	case problem != "":
		return 0, problem
	case n < 0:
		return 0, NotWhole
	case n > maxMinutes:
		return 0, TooLarge
	}
	return n, ""
}

// NormaliseRecipeName is the key no two of a User's Recipes share: their name, ignoring
// case and surrounding space.
func NormaliseRecipeName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// Problems is what is wrong with r for a User speaking l, by field, and with each of its
// Ingredients, by field, in their order. A valid Recipe has none.
func (r Recipe) Problems(l Language) (map[string]Problem, []map[string]Problem) {
	problems := map[string]Problem{}
	if strings.TrimSpace(r.Name) == "" {
		problems["name"] = Required
	}
	if r.Portions < 1 {
		problems["portions"] = NotWhole
	}
	if r.TotalMinutes <= 0 {
		problems["total"] = NotAboveZero
	} else if r.ActiveMinutes > r.TotalMinutes {
		problems["active"] = AboveTotal
	}
	if len(r.Kinds) == 0 {
		problems["slots"] = Required
	}
	if len(r.Ingredients) == 0 {
		problems["ingredients"] = Required
	}
	rows := make([]map[string]Problem, len(r.Ingredients))
	seen := map[string]bool{}
	for i, ri := range r.Ingredients {
		row := map[string]Problem{}
		if ri.Ingredient.ID == 0 {
			row = ri.Ingredient.Problems()
		}
		key := NormaliseIngredientName(l, ri.Ingredient.Name)
		if key != "" && seen[key] {
			row["name"] = ListedTwice
		}
		seen[key] = true
		if ri.Quantity <= 0 {
			row["quantity"] = NotAboveZero
		}
		rows[i] = row
	}
	return problems, rows
}

// NutritionPerPortion is r's Ingredients' nutritional values divided over its Portions,
// each rounded to a whole number, a floor rounded down.
func (r Recipe) NutritionPerPortion() PortionNutrition {
	if r.Portions < 1 {
		return PortionNutrition{}
	}
	perPortion := func(value func(Nutrition) Amount) (Amount, bool) {
		var total float64
		var known, unknown bool
		for _, ri := range r.Ingredients {
			a := value(ri.Ingredient.Nutrition)
			if !a.Known {
				unknown = true
				continue
			}
			known = true
			total += a.Value * ri.Quantity.Number() / ri.Ingredient.Unit.per()
		}
		if !known {
			return Unknown, false
		}
		if unknown {
			return Known(math.Floor(total / float64(r.Portions))), true // rounded up, it would claim more than is known
		}
		return Known(math.Round(total / float64(r.Portions))), false
	}
	var n PortionNutrition
	n.Kcal, _ = perPortion(func(n Nutrition) Amount { return n.Kcal })
	n.Protein, _ = perPortion(func(n Nutrition) Amount { return n.Protein })
	n.Carbs, _ = perPortion(func(n Nutrition) Amount { return n.Carbs })
	n.Fat, _ = perPortion(func(n Nutrition) Amount { return n.Fat })
	n.Fibre, n.FibreFloor = perPortion(func(n Nutrition) Amount { return n.Fibre })
	n.Sugar, n.SugarFloor = perPortion(func(n Nutrition) Amount { return n.Sugar })
	return n
}

// per is the quantity an Ingredient's nutritional values are given for: 100 g, 100 ml or
// one piece.
func (u Unit) per() float64 {
	if u == Pieces {
		return 1
	}
	return 100
}
