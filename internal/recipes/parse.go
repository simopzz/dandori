package recipes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/runs"
)

// recipeParseKind is the kind of Run that parses pasted recipe text into a
// draft Recipe.
const recipeParseKind = "recipe parse"

// parsedRecipe is a Recipe as the model submits it, and as its draft is
// stored until the user accepts or discards it.
type parsedRecipe struct {
	Name        string             `json:"name"`
	Portions    int                `json:"portions"`
	PrepMinutes int                `json:"prep_minutes"`
	Appliances  []string           `json:"appliances"`
	Steps       []string           `json:"steps"`
	Ingredients []parsedIngredient `json:"ingredients"`
}

type parsedIngredient struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	// Nutrition is the model's estimate, for an Ingredient it doesn't know.
	Nutrition *parsedNutrition `json:"nutrition,omitempty"`
}

type parsedNutrition struct {
	Kcal     float64 `json:"kcal"`
	ProteinG float64 `json:"protein_g"`
	CarbsG   float64 `json:"carbs_g"`
	FatG     float64 `json:"fat_g"`
}

// UnmarshalJSON requires every value, so one the model left out is not
// taken as 0.
func (n *parsedNutrition) UnmarshalJSON(data []byte) error {
	var values map[string]*float64
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	for _, key := range []string{"kcal", "protein_g", "carbs_g", "fat_g"} {
		if values[key] == nil {
			return fmt.Errorf("nutrition needs kcal, protein_g, carbs_g and fat_g; %s is missing", key)
		}
	}
	*n = parsedNutrition{*values["kcal"], *values["protein_g"], *values["carbs_g"], *values["fat_g"]}
	return nil
}

// recipe is the parsed Recipe with its Ingredients matched by name to the
// known ones, whose unit and nutritional values it takes.
func (p parsedRecipe) recipe(known map[string]domain.Ingredient) domain.Recipe {
	r := domain.Recipe{
		Name:        strings.TrimSpace(p.Name),
		Portions:    p.Portions,
		PrepMinutes: p.PrepMinutes,
		Appliances:  domain.FoodTerms(strings.Join(p.Appliances, "\n")),
		Steps:       nonBlank(p.Steps),
	}
	for _, pi := range p.Ingredients {
		ing := domain.Ingredient{Name: domain.IngredientName(pi.Name), Unit: domain.Unit(pi.Unit)}
		if k, ok := known[ing.Name]; ok {
			// Its known values, but the unit the quantity is in, so a unit
			// that differs from the known one is caught when it is read back.
			ing.Nutrition = k.Nutrition
		} else if pi.Nutrition != nil {
			ing.Nutrition = domain.IngredientNutrition(*pi.Nutrition)
		}
		r.Ingredients = append(r.Ingredients, domain.RecipeIngredient{Ingredient: ing, Quantity: pi.Quantity})
	}
	return r
}

// problems is what the model must fix before its Recipe can be a draft.
func (p parsedRecipe) problems(known map[string]domain.Ingredient) []string {
	var problems []string
	invalid := p.recipe(known).Validate()
	for _, k := range slices.Sorted(maps.Keys(invalid)) {
		problems = append(problems, k+": "+invalid[k])
	}
	for i, pi := range p.Ingredients {
		name := domain.IngredientName(pi.Name)
		if k, ok := known[name]; ok {
			if domain.Unit(pi.Unit) != k.Unit {
				problems = append(problems, fmt.Sprintf("Ingredients.%d: %s is a known Ingredient measured in %s", i, name, k.Unit))
			}
		} else if pi.Nutrition == nil {
			problems = append(problems, fmt.Sprintf("Ingredients.%d: %s is a new Ingredient; estimate its nutrition", i, name))
		}
	}
	return problems
}

// recipeParseSystem is the parse Run's instructions; %[1]s is the name of the
// Content Language.
const recipeParseSystem = `You turn recipe text the user pasted into a structured Recipe for their Recipe Library.

Call submit_recipe with the whole Recipe. Then end your turn with one short sentence.

- Write the Recipe name, Ingredient names, steps and appliances in %[1]s. If the recipe text is in another language, translate it into %[1]s; for an Ingredient with no word-for-word %[1]s name, use the food a %[1]s-speaking cook would buy for it.
- Name each Ingredient as a generic food: lowercase, singular, no amounts or brands ("chicken breast", not "2 organic chicken breasts").
- When an Ingredient is one of the known Ingredients, use exactly its name and unit.
- Give every quantity in the Ingredient's unit: g for solids, ml for liquids, piece for things that are counted (eggs, lemons). Convert cups and spoons to g or ml.
- For an Ingredient that is not known, estimate its nutritional values per 100 g, per 100 ml or per piece, matching its unit.
- portions is how many servings the recipe makes; estimate it if the text doesn't say. prep_minutes is the total time, 0 if you can't tell.
- appliances are the kitchen appliances the recipe needs (oven, hob, air fryer, blender, …). steps are the method, one instruction each.

If submit_recipe reports problems, fix them and call it again.`

var submitRecipeSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "portions": {"type": "integer", "minimum": 1},
    "prep_minutes": {"type": "integer", "minimum": 0},
    "appliances": {"type": "array", "items": {"type": "string"}},
    "steps": {"type": "array", "items": {"type": "string"}},
    "ingredients": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "quantity": {"type": "number", "exclusiveMinimum": 0},
          "unit": {"type": "string", "enum": ["g", "ml", "piece"]},
          "nutrition": {
            "type": "object",
            "description": "Per 100 g, per 100 ml or per piece. Only for Ingredients that are not known.",
            "properties": {
              "kcal": {"type": "number"},
              "protein_g": {"type": "number"},
              "carbs_g": {"type": "number"},
              "fat_g": {"type": "number"}
            },
            "required": ["kcal", "protein_g", "carbs_g", "fat_g"]
          }
        },
        "required": ["name", "quantity", "unit"]
      }
    }
  },
  "required": ["name", "portions", "prep_minutes", "appliances", "steps", "ingredients"]
}`)

// parseJob parses text into a draft Recipe, stored with the Run. known are
// the Ingredients already in the Recipe Library, which the model is told
// to reuse.
func (s *Service) parseJob(text string, known map[string]domain.Ingredient) agent.Job {
	var prompt strings.Builder
	prompt.WriteString("Known Ingredients:\n")
	if len(known) == 0 {
		prompt.WriteString("(none yet)\n")
	}
	for _, name := range slices.Sorted(maps.Keys(known)) {
		fmt.Fprintf(&prompt, "- %s (%s)\n", name, known[name].Unit)
	}
	fmt.Fprintf(&prompt, "\nRecipe text:\n<recipe>\n%s\n</recipe>", text)

	return agent.Job{
		System: fmt.Sprintf(recipeParseSystem, s.language.Name()),
		Prompt: prompt.String(),
		Tools: []agent.Tool{{
			Name:        "submit_recipe",
			Description: "Submit the parsed Recipe as a draft for the user to check. Replaces any earlier submission.",
			InputSchema: submitRecipeSchema,
			Run: func(ctx context.Context, _ agent.Trace, input json.RawMessage) (string, error) {
				var p parsedRecipe
				if err := json.Unmarshal(input, &p); err != nil {
					return "", fmt.Errorf("the Recipe does not match the schema: %w", err)
				}
				if problems := p.problems(known); len(problems) > 0 {
					return "", errors.New("fix these and submit again:\n" + strings.Join(problems, "\n"))
				}
				if err := saveDraft(ctx, s.pool, runs.IDFrom(ctx), p); err != nil {
					return "", fmt.Errorf("store the draft: %w", err)
				}
				return "The draft is saved for the user to check.", nil
			},
		}},
	}
}
