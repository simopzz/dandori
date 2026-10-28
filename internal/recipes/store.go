package recipes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// loadRecipes loads the Recipe with id, or every Recipe when id is nil,
// newest first, with their Ingredients. Call it in a snapshot transaction,
// so both queries see the same Recipes.
func loadRecipes(ctx context.Context, q db.Querier, id *int64) ([]Stored, error) {
	rows, _ := q.Query(ctx, `
		SELECT id, name, portions, prep_minutes, appliances, steps, updated_at FROM recipe
		WHERE $1::bigint IS NULL OR id = $1 ORDER BY id DESC`, id)
	recipes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Stored, error) {
		var r Stored
		err := row.Scan(&r.ID, &r.Name, &r.Portions, &r.PrepMinutes, &r.Appliances, &r.Steps, &r.version)
		return r, err
	})
	if err != nil || len(recipes) == 0 {
		return recipes, err
	}
	byID := map[int64]*Stored{}
	for i := range recipes {
		byID[recipes[i].ID] = &recipes[i]
	}
	rows, _ = q.Query(ctx, `
		SELECT ri.recipe_id, ri.quantity, i.name, i.unit, i.kcal, i.protein_g, i.carbs_g, i.fat_g
		FROM recipe_ingredient ri JOIN ingredient i ON i.id = ri.ingredient_id
		WHERE $1::bigint IS NULL OR ri.recipe_id = $1
		ORDER BY ri.recipe_id, ri.position`, id)
	var recipeID int64
	var ri domain.RecipeIngredient
	n := &ri.Ingredient.Nutrition
	_, err = pgx.ForEachRow(rows, []any{&recipeID, &ri.Quantity, &ri.Ingredient.Name, &ri.Ingredient.Unit, &n.Kcal, &n.ProteinG, &n.CarbsG, &n.FatG}, func() error {
		r := byID[recipeID]
		r.Ingredients = append(r.Ingredients, ri)
		return nil
	})
	return recipes, err
}

// loadIngredients loads every Ingredient, by name.
func loadIngredients(ctx context.Context, q db.Querier) (map[string]domain.Ingredient, error) {
	rows, _ := q.Query(ctx, `SELECT name, unit, kcal, protein_g, carbs_g, fat_g FROM ingredient`)
	known := map[string]domain.Ingredient{}
	var ing domain.Ingredient
	n := &ing.Nutrition
	_, err := pgx.ForEachRow(rows, []any{&ing.Name, &ing.Unit, &n.Kcal, &n.ProteinG, &n.CarbsG, &n.FatG}, func() error {
		known[ing.Name] = ing
		return nil
	})
	return known, err
}

// saveRecipe stores rec as the Recipe with id, or as a new Recipe when id
// is 0, adding its new Ingredients. It returns the Recipe's id.
func saveRecipe(ctx context.Context, q db.Querier, id int64, rec domain.Recipe) (int64, error) {
	var err error
	if id == 0 {
		err = q.QueryRow(ctx, `
			INSERT INTO recipe (name, portions, prep_minutes, appliances, steps)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			rec.Name, rec.Portions, rec.PrepMinutes, rec.Appliances, rec.Steps).Scan(&id)
	} else {
		_, err = q.Exec(ctx, `
			UPDATE recipe SET name = $2, portions = $3, prep_minutes = $4, appliances = $5, steps = $6, updated_at = now()
			WHERE id = $1`,
			id, rec.Name, rec.Portions, rec.PrepMinutes, rec.Appliances, rec.Steps)
		if err == nil {
			_, err = q.Exec(ctx, `DELETE FROM recipe_ingredient WHERE recipe_id = $1`, id)
		}
	}
	if err != nil {
		return 0, err
	}
	for i, ri := range rec.Ingredients {
		ing, n := ri.Ingredient, ri.Ingredient.Nutrition
		// A known Ingredient keeps its values: only new ones are inserted.
		if _, err := q.Exec(ctx, `
			INSERT INTO ingredient (name, unit, kcal, protein_g, carbs_g, fat_g)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			ing.Name, ing.Unit, n.Kcal, n.ProteinG, n.CarbsG, n.FatG); err != nil {
			return 0, err
		}
		if _, err := q.Exec(ctx, `
			INSERT INTO recipe_ingredient (recipe_id, position, ingredient_id, quantity)
			SELECT $1, $2, id, $4 FROM ingredient WHERE name = $3`,
			id, i, ing.Name, ri.Quantity); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// loadEmbedded loads the ids of the Recipes embedded by model as they now
// are.
func loadEmbedded(ctx context.Context, q db.Querier, model string) ([]int64, error) {
	rows, _ := q.Query(ctx, `
		SELECT e.recipe_id FROM recipe_embedding e
		JOIN recipe r ON r.id = e.recipe_id AND r.updated_at = e.recipe_updated_at
		WHERE e.model = $1`, model)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// saveEmbedding stores v, by model, as the embedding of rec as it was
// loaded, unless rec has gone. If rec changed meanwhile, the embedding is
// stale from the start, so it is computed again rather than taken for the
// edited Recipe's.
func saveEmbedding(ctx context.Context, q db.Querier, rec Stored, model string, v []float32) error {
	_, err := q.Exec(ctx, `
		INSERT INTO recipe_embedding (recipe_id, recipe_updated_at, model, embedding)
		SELECT id, $2, $3, $4::vector FROM recipe WHERE id = $1
		ON CONFLICT (recipe_id) DO UPDATE
		SET recipe_updated_at = EXCLUDED.recipe_updated_at, model = EXCLUDED.model, embedding = EXCLUDED.embedding`,
		rec.ID, rec.version, model, vectorText(v))
	return err
}

// vectorText is v as pgvector reads it: [1,2.5,3].
func vectorText(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = strconv.FormatFloat(float64(x), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// rankRecipes returns the ids of the Recipes that pass f's appliance and
// prep time filters: closest to query first when it is set, among those
// embedded by model, else newest first.
func rankRecipes(ctx context.Context, q db.Querier, f Search, model string, query []float32) ([]int64, error) {
	var vector *string
	if query != nil {
		v := vectorText(query)
		vector = &v
	}
	var maxMinutes *int
	if f.MaxMinutes > 0 {
		maxMinutes = &f.MaxMinutes
	}
	rows, _ := q.Query(ctx, `
		SELECT r.id FROM recipe r
		LEFT JOIN recipe_embedding e
		  ON e.recipe_id = r.id AND e.model = $1 AND e.recipe_updated_at = r.updated_at
		WHERE ($2::text = '' OR $2 = ANY (r.appliances))
		  -- A Recipe whose prep time isn't given can't promise to be quick.
		  AND ($3::integer IS NULL OR r.prep_minutes BETWEEN 1 AND $3)
		  AND ($4::vector IS NULL OR e.embedding IS NOT NULL)
		ORDER BY e.embedding <=> $4::vector, r.id DESC`,
		model, f.Appliance, maxMinutes, vector)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// loadAppliances loads every appliance a Recipe needs, alphabetically.
func loadAppliances(ctx context.Context, q db.Querier) ([]string, error) {
	rows, _ := q.Query(ctx, `SELECT DISTINCT unnest(appliances) AS a FROM recipe ORDER BY a`)
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func deleteRecipe(ctx context.Context, q db.Querier, id int64) error {
	_, err := q.Exec(ctx, `DELETE FROM recipe WHERE id = $1`, id)
	// 23503 is a foreign key violation: only a Prep Session refers to a
	// Recipe without cascading.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrHasPortions
	}
	return err
}

// saveDraft stores p as the draft of the parse Run with id, replacing any
// earlier one.
func saveDraft(ctx context.Context, q db.Querier, runID int64, p parsedRecipe) error {
	_, err := q.Exec(ctx, `
		INSERT INTO recipe_draft (run_id, recipe) VALUES ($1, $2)
		ON CONFLICT (run_id) DO UPDATE SET recipe = EXCLUDED.recipe`,
		runID, p)
	return err
}

func loadDraft(ctx context.Context, q db.Querier, runID int64) (parsedRecipe, bool, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT recipe FROM recipe_draft WHERE run_id = $1`, runID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return parsedRecipe{}, false, nil
	}
	if err != nil {
		return parsedRecipe{}, false, err
	}
	var p parsedRecipe
	if err := json.Unmarshal(raw, &p); err != nil {
		return parsedRecipe{}, false, fmt.Errorf("read draft of run %d: %w", runID, err)
	}
	return p, true, nil
}

func deleteDraft(ctx context.Context, q db.Querier, runID int64) error {
	tag, err := q.Exec(ctx, `DELETE FROM recipe_draft WHERE run_id = $1`, runID)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrDraftGone
	}
	return err
}
