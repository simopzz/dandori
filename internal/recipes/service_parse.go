package recipes

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/runs"
)

// ParseKind is the kind of Run that reads pasted text into a Recipe Draft.
const ParseKind = "parse"

// parseEffort is how hard the model thinks reading a recipe.
const parseEffort = agent.EffortMedium

// parsedRecipe is a Recipe as the model submits it, and as its Recipe Draft is stored:
// its Ingredients by name, with values proposed for those that aren't known.
type parsedRecipe struct {
	Name          string             `json:"name"`
	Portions      int                `json:"portions"`
	ActiveMinutes int                `json:"active_minutes"`
	TotalMinutes  int                `json:"total_minutes"`
	SlotKinds     []domain.SlotKind  `json:"slot_kinds"`
	Appliances    []string           `json:"appliances"`
	Steps         []string           `json:"steps"`
	Ingredients   []parsedIngredient `json:"ingredients"`
}

type parsedIngredient struct {
	Name     string      `json:"name"`
	Quantity float64     `json:"quantity"`
	Unit     domain.Unit `json:"unit"`
	// New is what the model proposes for an Ingredient that isn't known.
	New *proposedIngredient `json:"new,omitempty"`
}

// proposedIngredient is a new Ingredient's values; nil is unknown, never 0.
type proposedIngredient struct {
	Kcal    *float64               `json:"kcal"`
	Protein *float64               `json:"protein_g"`
	Carbs   *float64               `json:"carbs_g"`
	Fat     *float64               `json:"fat_g"`
	Fibre   *float64               `json:"fibre_g"`
	Sugar   *float64               `json:"sugar_g"`
	Usual   domain.StorageLocation `json:"usual"`
	Staple  bool                   `json:"staple"`
}

// recipe is p for a User speaking l, each Ingredient resolved by name to the one of known
// it is, if any, and otherwise new with the values proposed. problems are what p has
// that a Recipe can't, by field path, as the model is told.
func (p parsedRecipe) recipe(l domain.Language, known []domain.Ingredient) (rec domain.Recipe, problems map[string]string) {
	problems = map[string]string{}
	rec = domain.Recipe{
		Name: strings.TrimSpace(p.Name), Portions: p.Portions,
		ActiveMinutes: p.ActiveMinutes, TotalMinutes: p.TotalMinutes,
		Kinds: p.SlotKinds, Appliances: nonBlank(p.Appliances), Steps: nonBlank(p.Steps),
	}
	for _, k := range p.SlotKinds {
		if !slices.Contains(domain.SlotKinds, k) {
			problems["slot_kinds"] = fmt.Sprintf("%q is not a kind of Slot", k)
		}
	}
	unresolved := map[int]bool{} // rows whose own problems would only repeat why
	byName := map[string]domain.Ingredient{}
	for _, ing := range known {
		byName[domain.NormaliseIngredientName(l, ing.Name)] = ing
	}
	for i, pi := range p.Ingredients {
		at := fmt.Sprintf("ingredients.%d", i)
		ing, isKnown := byName[domain.NormaliseIngredientName(l, pi.Name)]
		switch {
		case isKnown && pi.Unit != ing.Unit:
			problems[at+".unit"] = fmt.Sprintf("%s is a known Ingredient measured in %s; give its quantity in %s", ing.Name, ing.Unit, ing.Unit)
			unresolved[i] = true
			rec.Ingredients = append(rec.Ingredients, domain.RecipeIngredient{Ingredient: ing}) // no quantity: never converted
			continue
		case !isKnown && pi.New == nil: // no values proposed, as for a known Ingredient deleted since: new, its values unknown
			problems[at+".new"] = fmt.Sprintf("%s is not a known Ingredient; propose its values in new", pi.Name)
			unresolved[i] = true
			ing = domain.Ingredient{Name: strings.TrimSpace(pi.Name), Unit: pi.Unit}
		case !isKnown:
			ing = domain.Ingredient{Name: strings.TrimSpace(pi.Name), Unit: pi.Unit, Usual: pi.New.Usual, Staple: pi.New.Staple}
			ing.Nutrition = domain.Nutrition{
				Kcal: amount(problems, at+".new.kcal", pi.New.Kcal), Protein: amount(problems, at+".new.protein_g", pi.New.Protein),
				Carbs: amount(problems, at+".new.carbs_g", pi.New.Carbs), Fat: amount(problems, at+".new.fat_g", pi.New.Fat),
				Fibre: amount(problems, at+".new.fibre_g", pi.New.Fibre), Sugar: amount(problems, at+".new.sugar_g", pi.New.Sugar),
			}
		}
		q, problem := domain.ParseQuantity(ing.Unit, strconv.FormatFloat(pi.Quantity, 'f', -1, 64))
		if problem != "" && ing.Unit != "" {
			problems[at+".quantity"] = string(problem)
		}
		rec.Ingredients = append(rec.Ingredients, domain.RecipeIngredient{Ingredient: ing, Quantity: q})
	}
	fields, rows := rec.Problems(l)
	for field, problem := range fields {
		problems[cmp.Or(schemaFields[field], field)] = string(problem)
	}
	for i, row := range rows {
		if unresolved[i] {
			continue
		}
		for field, problem := range row {
			key := fmt.Sprintf("ingredients.%d.%s", i, cmp.Or(schemaFields["ingredient."+field], field))
			if _, ok := problems[key]; !ok {
				problems[key] = string(problem)
			}
		}
	}
	return rec, problems
}

// schemaFields are submit_recipe's names for the fields domain.Recipe.Problems names
// otherwise, a row's prefixed "ingredient.".
var schemaFields = map[string]string{
	"active": "active_minutes", "total": "total_minutes", "slots": "slot_kinds",
	"ingredient.kcal": "new.kcal", "ingredient.protein": "new.protein_g", "ingredient.carbs": "new.carbs_g",
	"ingredient.fat": "new.fat_g", "ingredient.usual": "new.usual",
}

// amount is a proposed value v, nil being unknown, or why it can't be, kept in problems
// at field.
func amount(problems map[string]string, field string, v *float64) domain.Amount {
	if v == nil {
		return domain.Unknown
	}
	a, problem := domain.ParseAmount(strconv.FormatFloat(*v, 'f', -1, 64))
	if problem != "" {
		problems[field] = string(problem)
	}
	return a
}

// languageNames name each Language for the model.
var languageNames = map[domain.Language]string{domain.English: "English", domain.Italian: "Italian"}

// parseSystem is the parse Run's system prompt, the same for every User and paste but
// for %[1]s, the name of the User's Language, so it is cached.
const parseSystem = `You turn recipe text the user pasted into a structured Recipe for their Recipe Library.

Call submit_recipe with the whole Recipe. Then end your turn with one short sentence.

- Write the Recipe name, Ingredient names, steps and appliances in %[1]s. If the recipe text is in another language, translate it into %[1]s; for an Ingredient with no word-for-word %[1]s name, use the food a %[1]s-speaking cook would buy for it.
- Name each Ingredient as a generic food: lowercase, singular, no amounts or brands ("chicken breast", not "2 organic chicken breasts").
- When an Ingredient is one of the known Ingredients, use exactly its name and unit, and leave out new.
- Give every quantity in the Ingredient's unit: g for solids, ml for liquids, piece for things that are counted (eggs, lemons). Convert cups and spoons to g or ml. Give food packed in liquid drained: "1 tin of chickpeas" is 240 g.
- For an Ingredient that is not known, propose in new its nutritional values per 100 g, per 100 ml or per piece, matching its unit, where it is usually kept (pantry, fridge or freezer), and whether it is a staple: always in the kitchen and too fiddly to weigh, such as oil, salt and spices. Give fibre_g and sugar_g as null when you can't tell; never guess 0.
- portions is how many Portions the recipe makes as written; estimate it if the text doesn't say. active_minutes is the time spent working at it, total_minutes the whole time including unattended oven or resting time.
- slot_kinds are when its Portions may be eaten: breakfast, lunch, snack or dinner, any number of them.
- appliances are the kitchen appliances the recipe needs (oven, hob, air fryer, blender, …). steps are the method, one instruction each.

If submit_recipe reports problems, fix them and call it again.`

var submitRecipeSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "portions": {"type": "integer", "minimum": 1},
    "active_minutes": {"type": "integer", "minimum": 0},
    "total_minutes": {"type": "integer", "minimum": 1},
    "slot_kinds": {"type": "array", "items": {"type": "string", "enum": ["breakfast", "lunch", "snack", "dinner"]}, "minItems": 1},
    "appliances": {"type": "array", "items": {"type": "string"}},
    "steps": {"type": "array", "items": {"type": "string"}},
    "ingredients": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "quantity": {"type": "number", "exclusiveMinimum": 0, "exclusiveMaximum": 1000000, "description": "A whole number in g or ml; at most one decimal place in piece."},
          "unit": {"type": "string", "enum": ["g", "ml", "piece"]},
          "new": {
            "type": "object",
            "description": "Only for an Ingredient that is not known. Values per 100 g, per 100 ml or per piece.",
            "properties": {
              "kcal": {"type": "number", "minimum": 0, "exclusiveMaximum": 10000, "description": "At most one decimal place."},
              "protein_g": {"type": "number", "minimum": 0, "exclusiveMaximum": 10000, "description": "At most one decimal place."},
              "carbs_g": {"type": "number", "minimum": 0, "exclusiveMaximum": 10000, "description": "At most one decimal place."},
              "fat_g": {"type": "number", "minimum": 0, "exclusiveMaximum": 10000, "description": "At most one decimal place."},
              "fibre_g": {"type": ["number", "null"], "minimum": 0, "exclusiveMaximum": 10000, "description": "At most one decimal place."},
              "sugar_g": {"type": ["number", "null"], "minimum": 0, "exclusiveMaximum": 10000, "description": "At most one decimal place."},
              "usual": {"type": "string", "enum": ["pantry", "fridge", "freezer"]},
              "staple": {"type": "boolean"}
            },
            "required": ["kcal", "protein_g", "carbs_g", "fat_g", "fibre_g", "sugar_g", "usual", "staple"]
          }
        },
        "required": ["name", "quantity", "unit"]
      }
    }
  },
  "required": ["name", "portions", "active_minutes", "total_minutes", "slot_kinds", "appliances", "steps", "ingredients"]
}`)

// parseBrief reads text into a Recipe Draft for a User speaking l, stored with the Run.
// known are their Ingredients, which the model is told to reuse.
func (s *Service) parseBrief(l domain.Language, text string, known []domain.Ingredient) agent.Brief {
	var prompt strings.Builder
	prompt.WriteString("Known Ingredients:\n")
	if len(known) == 0 {
		prompt.WriteString("(none yet)\n")
	}
	for _, ing := range known {
		fmt.Fprintf(&prompt, "- %s (%s)\n", ing.Name, ing.Unit)
	}
	fmt.Fprintf(&prompt, "\nRecipe text:\n<recipe>\n%s\n</recipe>", text)

	return agent.Brief{
		System: fmt.Sprintf(parseSystem, languageNames[l]),
		Prompt: prompt.String(),
		Effort: parseEffort,
		Ended: func(ctx context.Context, said string) error {
			_, err := s.DraftOf(ctx, runs.IDFrom(ctx))
			if errors.Is(err, ErrNoDraft) {
				return &agent.Failure{Reason: agent.NoResult, Detail: said}
			}
			return err
		},
		Tools: []agent.Tool{{
			Name:        "submit_recipe",
			Description: "Submit the parsed Recipe as a Recipe Draft for the user to check. Replaces any earlier submission.",
			InputSchema: submitRecipeSchema,
			Run: func(ctx context.Context, input json.RawMessage) (string, error) {
				var p parsedRecipe
				if err := json.Unmarshal(input, &p); err != nil {
					return "", fmt.Errorf("the Recipe does not match the schema: %w", err)
				}
				if _, problems := p.recipe(l, known); len(problems) > 0 {
					var lines []string
					for _, field := range slices.Sorted(maps.Keys(problems)) {
						lines = append(lines, field+": "+problems[field])
					}
					return "", errors.New("fix these and submit again:\n" + strings.Join(lines, "\n"))
				}
				if err := s.saveDraft(ctx, runs.IDFrom(ctx), p); err != nil {
					return "", fmt.Errorf("store the Recipe Draft: %w", err)
				}
				return "The Recipe Draft is saved for the user to check.", nil
			},
		}},
	}
}
