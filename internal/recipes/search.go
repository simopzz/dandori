package recipes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/embedding"
)

// searchLimit is how many Recipes a search by meaning returns: the closest,
// since every Recipe is some distance away.
const searchLimit = 10

// Search is what the Recipe Library is searched for. Every part is
// optional; with none, every Recipe is found.
type Search struct {
	// Text is what the user is after, in natural language.
	Text string
	// Appliance, when set, is one the Recipes must need.
	Appliance string
	// MaxMinutes, when above 0, is the longest prep time; Recipes whose
	// prep time isn't given are left out.
	MaxMinutes int
	// InStock keeps only Recipes whose Ingredients stock covers.
	InStock bool
}

// normalised is f with its text trimmed and its appliance written as
// Recipes store it.
func (f Search) normalised() Search {
	f.Text = strings.TrimSpace(f.Text)
	f.Appliance = domain.FoodTerm(f.Appliance)
	return f
}

// Any reports whether f searches for anything, rather than listing every
// Recipe.
func (f Search) Any() bool { return f.normalised() != Search{} }

// Found is a Recipe a search found.
type Found struct {
	Stored
	// Missing names the Ingredients stock doesn't cover, in the Recipe's
	// order.
	Missing []string
}

// Results is what a search found: when it had Text, the closest in meaning
// first, else newest first.
type Results struct {
	Recipes []Found
	// Unranked, when set, is why Text couldn't be searched by meaning, so
	// Recipes are only filtered.
	Unranked string
	// Failed means Unranked is a failure of the embedding provider, rather
	// than there being none.
	Failed bool
}

// Search finds the Recipes matching f.
func (s *Service) Search(ctx context.Context, f Search) (Results, error) {
	f = f.normalised()
	var res Results
	var query []float32
	if f.Text != "" {
		query, res.Unranked, res.Failed = s.embedQuery(ctx, f.Text)
	}
	stock, err := s.stock.Items(ctx)
	if err != nil {
		return Results{}, err
	}
	var items []domain.StockItem
	for _, it := range stock {
		items = append(items, it.StockItem)
	}

	var ids []int64
	var recipes []Stored
	err = pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		var err error
		if ids, err = rankRecipes(ctx, tx, f, s.model(), query); err != nil {
			return err
		}
		recipes, err = loadRecipes(ctx, tx, nil)
		return err
	})
	if err != nil {
		return Results{}, err
	}
	today := s.today()
	for _, id := range ids {
		i := slices.IndexFunc(recipes, func(r Stored) bool { return r.ID == id })
		if i < 0 {
			continue
		}
		found := Found{Stored: recipes[i], Missing: recipes[i].MissingFrom(items, today)}
		if f.InStock && len(found.Missing) > 0 {
			continue
		}
		res.Recipes = append(res.Recipes, found)
		if query != nil && len(res.Recipes) == searchLimit {
			break
		}
	}
	return res, nil
}

// embedQuery embeds text to search by, first embedding the Recipes that
// aren't yet. Without a vector, unranked says why, and failed whether the
// embedding provider failed.
func (s *Service) embedQuery(ctx context.Context, text string) (query []float32, unranked string, failed bool) {
	if s.embedder == nil {
		return nil, "Searching by meaning needs an embedding provider, and none is set up", false
	}
	err := s.embedRecipes(ctx)
	var vectors [][]float32
	if err == nil {
		vectors, err = s.embedder.Embed(ctx, embedding.Query, []string{text})
	}
	if err != nil {
		return nil, "The embedding provider failed: " + err.Error(), true
	}
	return vectors[0], "", false
}

// model is the embedding model, "" without an embedder.
func (s *Service) model() string {
	if s.embedder == nil {
		return ""
	}
	return s.embedder.Model()
}

// embedRecipes embeds every Recipe that has no embedding by the configured
// model: changed or new since, made before there was an embedder, or
// embedded by another model.
func (s *Service) embedRecipes(ctx context.Context) error {
	if s.embedder == nil {
		return nil
	}
	model := s.embedder.Model()
	var pending []Stored
	err := pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		recipes, err := loadRecipes(ctx, tx, nil)
		if err != nil {
			return err
		}
		embedded, err := loadEmbedded(ctx, tx, model)
		for _, r := range recipes {
			if !slices.Contains(embedded, r.ID) {
				pending = append(pending, r)
			}
		}
		return err
	})
	if err != nil || len(pending) == 0 {
		return err
	}
	texts := make([]string, len(pending))
	for i, r := range pending {
		texts[i] = embeddingText(r.Recipe)
	}
	vectors, err := s.embedder.Embed(ctx, embedding.Document, texts)
	if err != nil {
		return err
	}
	if len(vectors) != len(pending) {
		return fmt.Errorf("embedded %d Recipes, got %d vectors", len(pending), len(vectors))
	}
	for i, r := range pending {
		if err := saveEmbedding(ctx, s.pool, r, model, vectors[i]); err != nil {
			return err
		}
	}
	return nil
}

// embeddingText is what a Recipe is searched by: its name, Ingredients,
// appliances and steps.
func embeddingText(r domain.Recipe) string {
	names := make([]string, len(r.Ingredients))
	for i, ri := range r.Ingredients {
		names[i] = ri.Ingredient.Name
	}
	return fmt.Sprintf("%s\nIngredients: %s\nAppliances: %s\n%s",
		r.Name, strings.Join(names, ", "), strings.Join(r.Appliances, ", "), strings.Join(r.Steps, "\n"))
}

// Appliances lists every appliance a Recipe in the Recipe Library needs,
// alphabetically.
func (s *Service) Appliances(ctx context.Context) ([]string, error) {
	return loadAppliances(ctx, s.pool)
}

var searchRecipesSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "What you are after, in natural language, in the Content Language. Leave out to filter only."},
    "appliance": {"type": "string", "description": "An appliance the Recipes must need."},
    "max_prep_minutes": {"type": "integer", "minimum": 1, "description": "The longest prep time; Recipes whose prep time isn't given are left out."},
    "in_stock": {"type": "boolean", "description": "Only Recipes whose Ingredients current Stock Items cover, unexpired."}
  }
}`)

// SearchTool is the search as a tool the planner agent can call.
func (s *Service) SearchTool() agent.Tool {
	return agent.Tool{
		Name: "search_recipes",
		Description: fmt.Sprintf("Search the Recipe Library: with a query, the %d Recipes closest to it in meaning, closest first; without one, every Recipe that passes the filters, newest first. "+
			"Each Recipe comes with its Portions, prep time, appliances, nutrition per Portion, Ingredients and the ones stock doesn't cover.", searchLimit),
		InputSchema: searchRecipesSchema,
		Run: func(ctx context.Context, _ agent.Trace, input json.RawMessage) (string, error) {
			var in struct {
				Query          string `json:"query"`
				Appliance      string `json:"appliance"`
				MaxPrepMinutes int    `json:"max_prep_minutes"`
				InStock        bool   `json:"in_stock"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return "", fmt.Errorf("the search does not match the schema: %w", err)
			}
			if in.MaxPrepMinutes < 0 {
				return "", errors.New("max_prep_minutes must be at least 1, or left out")
			}
			res, err := s.Search(ctx, Search{Text: in.Query, Appliance: in.Appliance, MaxMinutes: in.MaxPrepMinutes, InStock: in.InStock})
			if err != nil {
				return "", err
			}
			return describeResults(res), nil
		},
	}
}

// describeResults is res as the model reads it.
func describeResults(res Results) string {
	var b strings.Builder
	if res.Unranked != "" {
		fmt.Fprintf(&b, "%s; these are every Recipe that passes the filters, newest first.\n", res.Unranked)
	}
	if len(res.Recipes) == 0 {
		b.WriteString("No Recipes match.")
	}
	for i, r := range res.Recipes {
		n := r.NutritionPerPortion()
		fmt.Fprintf(&b, "%d. %s (Recipe %d): %d Portions, %s, needs %s; per Portion %s. Ingredients: ",
			i+1, r.Name, r.ID, r.Portions, formatMinutes(r.PrepMinutes), orNone(r.Appliances), n.Describe())
		for j, ri := range r.Ingredients {
			if j > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s %s %s", formatAmount(ri.Quantity), ri.Ingredient.Unit, ri.Ingredient.Name)
		}
		fmt.Fprintf(&b, ". %s.\n", coverage(r.Missing))
	}
	return strings.TrimSpace(b.String())
}

func orNone(appliances []string) string {
	if len(appliances) == 0 {
		return "no appliance"
	}
	return strings.Join(appliances, ", ")
}
