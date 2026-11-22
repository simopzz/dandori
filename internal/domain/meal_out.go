package domain

import "strings"

// The Meal Out estimate's fields, as typed.
const (
	MealOutKcal    = "kcal"
	MealOutProtein = "protein"
	MealOutCarbs   = "carbs"
	MealOutFat     = "fat"
	MealOutFibre   = "fibre"
	MealOutSugar   = "sugar"
)

// MealOutRequired is whether the Meal Out estimate's `field` must be given: all but fibre
// and sugar, which may be left Unknown.
func MealOutRequired(field string) bool { return field != MealOutFibre && field != MealOutSugar }

// ParseMealOut reads the Meal Out estimate as `typed`, by field, with a problem per field
// refused: each value a whole number from 0, below 10000, and fibre and sugar Unknown
// when left blank.
func ParseMealOut(typed map[string]string) (Nutrition, map[string]Problem) {
	problems := map[string]Problem{}
	read := func(field string) Amount {
		if !MealOutRequired(field) && strings.TrimSpace(typed[field]) == "" {
			return Unknown
		}
		v, problem := parseWhole(typed[field])
		switch {
		case problem != "":
		case v < 0:
			problem = Negative
		case v >= maxAmount:
			problem = TooLarge
		}
		if problem != "" {
			problems[field] = problem
			return Unknown
		}
		return Known(float64(v))
	}
	n := Nutrition{
		Kcal: read(MealOutKcal), Protein: read(MealOutProtein), Carbs: read(MealOutCarbs), Fat: read(MealOutFat),
		Fibre: read(MealOutFibre), Sugar: read(MealOutSugar),
	}
	return n, problems
}
