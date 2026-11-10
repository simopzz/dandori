package recipes

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/domain"
)

// DemoLLM answers parse Runs without asking a real model, so the pages can be
// tried under mise run dev at no cost. It submits one Recipe named after the paste's
// first line, in the Language the system prompt names, each of its Ingredients the User
// knows in the unit they know it in, then ends its turn; a refusal ends it saying why.
type DemoLLM struct{}

// demoUsage is what each of its turns claims to use, so the Trace shows tokens.
var demoUsage = agent.Usage{Input: 1200, Output: 300, CacheRead: 1000}

// Send implements agent.LLM.
func (DemoLLM) Send(_ context.Context, req agent.Request) (agent.Response, error) {
	last := req.Messages[len(req.Messages)-1].Content
	if len(req.Messages) > 1 {
		said := "Read it into a Recipe Draft, without asking a model."
		if result := last[len(last)-1]; result.IsError {
			said = "submit_recipe refused it:\n\n" + result.Text
		}
		return agent.Response{
			StopReason: agent.StopEndTurn,
			Content:    []agent.Block{{Type: agent.BlockText, Text: said}},
			Usage:      demoUsage,
		}, nil
	}

	l := domain.English
	if strings.Contains(req.System, " in Italian.") {
		l = domain.Italian
	}
	prompt := last[0].Text
	p := demoRecipes[l]
	p.Ingredients = append([]parsedIngredient(nil), p.Ingredients...)
	if name := pastedName(prompt); name != "" {
		p.Name = name
	}
	known := knownUnits(l, prompt)
	for i, ing := range p.Ingredients {
		if unit, ok := known[domain.NormaliseIngredientName(l, ing.Name)]; ok {
			p.Ingredients[i].Unit, p.Ingredients[i].New = unit, nil
		}
	}
	input, err := json.Marshal(p)
	if err != nil {
		return agent.Response{}, err
	}
	return agent.Response{
		StopReason: agent.StopToolUse,
		Content: []agent.Block{
			{Type: agent.BlockText, Text: "Reading " + p.Name + "."},
			{Type: agent.BlockToolUse, ToolUseID: "demo", ToolName: "submit_recipe", Input: input},
		},
		Usage: demoUsage,
	}, nil
}

// pastedName is the first line of the recipe text in a parse Run's prompt, at most 80
// characters of it.
func pastedName(prompt string) string {
	_, text, _ := strings.Cut(prompt, "<recipe>\n")
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return string([]rune(line)[:min(len([]rune(line)), 80)])
		}
	}
	return ""
}

// knownUnits are the units of the known Ingredients a parse Run's prompt lists, by
// normalised name, from its lines "- name (unit)".
func knownUnits(l domain.Language, prompt string) map[string]domain.Unit {
	units := map[string]domain.Unit{}
	for line := range strings.SplitSeq(prompt, "\n") {
		item, ok := strings.CutPrefix(line, "- ")
		if !ok {
			continue
		}
		open := strings.LastIndex(item, " (")
		if open < 0 || !strings.HasSuffix(item, ")") {
			continue
		}
		units[domain.NormaliseIngredientName(l, item[:open])] = domain.Unit(item[open+2 : len(item)-1])
	}
	return units
}

func value(v float64) *float64 { return &v }

// demoRecipes are what DemoLLM submits in each Language, before it names one.
var demoRecipes = map[domain.Language]parsedRecipe{
	domain.English: {
		Name: "Pasta and chickpeas", Portions: 2, ActiveMinutes: 10, TotalMinutes: 25,
		SlotKinds: []domain.SlotKind{domain.Lunch, domain.Dinner}, Appliances: []string{"hob"},
		Steps: []string{"Soften the garlic in the oil.", "Add the chickpeas and water, then the pasta.", "Cook until the pasta is done."},
		Ingredients: []parsedIngredient{
			{Name: "dried pasta", Quantity: 160, Unit: domain.Grams, New: &proposedIngredient{
				Kcal: value(355), Protein: value(12.5), Carbs: value(71), Fat: value(1.5), Fibre: value(3), Sugar: value(3.5), Usual: domain.Pantry}},
			{Name: "chickpeas", Quantity: 240, Unit: domain.Grams, New: &proposedIngredient{
				Kcal: value(120), Protein: value(7), Carbs: value(16), Fat: value(2.5), Usual: domain.Pantry}},
			{Name: "olive oil", Quantity: 10, Unit: domain.Millilitres, New: &proposedIngredient{
				Kcal: value(820), Protein: value(0), Carbs: value(0), Fat: value(91), Fibre: value(0), Sugar: value(0), Usual: domain.Pantry, Staple: true}},
			{Name: "garlic", Quantity: 1, Unit: domain.Pieces, New: &proposedIngredient{
				Kcal: value(4), Protein: value(0.2), Carbs: value(1), Fat: value(0), Usual: domain.Pantry}},
		},
	},
	domain.Italian: {
		Name: "Pasta e ceci", Portions: 2, ActiveMinutes: 10, TotalMinutes: 25,
		SlotKinds: []domain.SlotKind{domain.Lunch, domain.Dinner}, Appliances: []string{"fornello"},
		Steps: []string{"Fai appassire l'aglio nell'olio.", "Aggiungi i ceci e l'acqua, poi la pasta.", "Cuoci finché la pasta è pronta."},
		Ingredients: []parsedIngredient{
			{Name: "pasta secca", Quantity: 160, Unit: domain.Grams, New: &proposedIngredient{
				Kcal: value(355), Protein: value(12.5), Carbs: value(71), Fat: value(1.5), Fibre: value(3), Sugar: value(3.5), Usual: domain.Pantry}},
			{Name: "ceci", Quantity: 240, Unit: domain.Grams, New: &proposedIngredient{
				Kcal: value(120), Protein: value(7), Carbs: value(16), Fat: value(2.5), Usual: domain.Pantry}},
			{Name: "olio d'oliva", Quantity: 10, Unit: domain.Millilitres, New: &proposedIngredient{
				Kcal: value(820), Protein: value(0), Carbs: value(0), Fat: value(91), Fibre: value(0), Sugar: value(0), Usual: domain.Pantry, Staple: true}},
			{Name: "aglio", Quantity: 1, Unit: domain.Pieces, New: &proposedIngredient{
				Kcal: value(4), Protein: value(0.2), Carbs: value(1), Fat: value(0), Usual: domain.Pantry}},
		},
	},
}
