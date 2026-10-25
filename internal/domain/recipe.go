package domain

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Unit is the one unit an Ingredient is measured in, in Recipes and in its
// nutritional values.
type Unit string

const (
	Grams       Unit = "g"
	Millilitres Unit = "ml"
	Pieces      Unit = "piece"
)

// Units lists every Unit.
var Units = []Unit{Grams, Millilitres, Pieces}

// per is the quantity an Ingredient's nutritional values are given for:
// 100 g, 100 ml or one piece.
func (u Unit) per() float64 {
	if u == Pieces {
		return 1
	}
	return 100
}

// Ingredient is a food as such, with its nutritional values.
type Ingredient struct {
	// Name is normalised by IngredientName; it identifies the Ingredient.
	Name string
	Unit Unit
	// Nutrition is per 100 g, per 100 ml or per piece, as Unit says.
	Nutrition IngredientNutrition
}

// IngredientNutrition is the calories and macros in 100 g, 100 ml or one
// piece of an Ingredient. Unlike a day's Nutrition it keeps decimals, since
// small amounts add up across a Recipe.
type IngredientNutrition struct {
	Kcal     float64
	ProteinG float64
	CarbsG   float64
	FatG     float64
}

// IngredientName normalises an Ingredient name as typed or parsed, so the
// same food is matched however it is written.
func IngredientName(s string) string { return normaliseFood(s) }

// RecipeIngredient is a quantity of an Ingredient in a Recipe, in the
// Ingredient's Unit.
type RecipeIngredient struct {
	Ingredient Ingredient
	Quantity   float64
}

// Recipe is a reusable description of a dish. Portions is how many Portions
// its Ingredients make.
type Recipe struct {
	Name        string
	Portions    int
	PrepMinutes int
	Appliances  []string
	Steps       []string
	Ingredients []RecipeIngredient
}

// Validate returns what is wrong with the Recipe, keyed by field path such
// as "Portions" or "Ingredients.2.Quantity". A valid Recipe returns none.
func (r Recipe) Validate() map[string]string {
	problems := map[string]string{}
	if strings.TrimSpace(r.Name) == "" {
		problems["Name"] = "Required"
	}
	if r.Portions < 1 {
		problems["Portions"] = "Must be at least 1"
	}
	if r.PrepMinutes < 0 {
		problems["PrepMinutes"] = "Can't be negative"
	}
	if len(r.Ingredients) == 0 {
		problems["Ingredients"] = "A Recipe needs at least one Ingredient"
	}
	seen := map[string]bool{}
	for i, ri := range r.Ingredients {
		key := fmt.Sprintf("Ingredients.%d.", i)
		name := IngredientName(ri.Ingredient.Name)
		switch {
		case name == "":
			problems[key+"Name"] = "Required"
		case seen[name]:
			problems[key+"Name"] = "Listed twice"
		}
		seen[name] = true
		if !slices.Contains(Units, ri.Ingredient.Unit) {
			problems[key+"Unit"] = "Must be g, ml or piece"
		}
		if ri.Quantity <= 0 {
			problems[key+"Quantity"] = "Must be more than 0"
		}
		n := ri.Ingredient.Nutrition
		if n.Kcal < 0 || n.ProteinG < 0 || n.CarbsG < 0 || n.FatG < 0 {
			problems[key+"Nutrition"] = "Can't be negative"
		}
	}
	return problems
}

// NutritionPerPortion is the Recipe's Ingredients' nutritional values
// divided over its Portions, rounded to whole kcal and grams.
func (r Recipe) NutritionPerPortion() Nutrition {
	if r.Portions <= 0 {
		return Nutrition{}
	}
	var total IngredientNutrition
	for _, ri := range r.Ingredients {
		f := ri.Quantity / ri.Ingredient.Unit.per()
		n := ri.Ingredient.Nutrition
		total.Kcal += n.Kcal * f
		total.ProteinG += n.ProteinG * f
		total.CarbsG += n.CarbsG * f
		total.FatG += n.FatG * f
	}
	portions := float64(r.Portions)
	return Nutrition{
		Kcal:     int(math.Round(total.Kcal / portions)),
		ProteinG: int(math.Round(total.ProteinG / portions)),
		CarbsG:   int(math.Round(total.CarbsG / portions)),
		FatG:     int(math.Round(total.FatG / portions)),
	}
}
