package domain

import "strings"

// MealOutFields are the Meal Out estimate's fields, as typed.
var MealOutFields = []string{"kcal", "protein", "carbs", "fat", "fibre", "sugar"}

// ParseMealOut reads the Meal Out estimate as typed, by field, with a problem per field
// refused: each value a whole number from 0, below 10000, and fibre and sugar Unknown
// when left blank.
func ParseMealOut(typed map[string]string) (Nutrition, map[string]Problem) {
	problems := map[string]Problem{}
	read := func(field string) Amount {
		if (field == "fibre" || field == "sugar") && strings.TrimSpace(typed[field]) == "" {
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
		Kcal: read("kcal"), Protein: read("protein"), Carbs: read("carbs"), Fat: read("fat"),
		Fibre: read("fibre"), Sugar: read("sugar"),
	}
	return n, problems
}
