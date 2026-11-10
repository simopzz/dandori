package recipes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
)

// CountRecipes is how many Recipes the signed-in User has, for the Profile's Food Terms.
// It has no WHERE on purpose: run in db.AsUser, row-level security leaves their rows.
func CountRecipes(ctx context.Context, q db.Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM recipe`).Scan(&n)
	return n, err
}

// RecipesUsing is the first by name of the signed-in User's Recipes that list the
// Ingredient id, or "" if none does, and how many others do, which ingredients is given
// by app since this feature depends on it.
func RecipesUsing(ctx context.Context, q db.Querier, id int64) (first string, others int, err error) {
	err = q.QueryRow(ctx, `
		SELECT r.name, count(*) OVER () - 1
		FROM recipe_ingredient ri JOIN recipe r ON r.id = ri.recipe_id
		WHERE ri.ingredient_id = $1
		ORDER BY r.normalised_name LIMIT 1`, id).Scan(&first, &others)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, nil
	}
	return first, others, err
}

// slotKindsOf is kinds as the database stores them.
func slotKindsOf(kinds []domain.SlotKind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// saveRecipe adds rec for the signed-in User under its normalised name, unless they have
// a Recipe of that name, and is its id; added says whether it did.
func saveRecipe(ctx context.Context, q db.Querier, rec domain.Recipe, normalised string) (id int64, added bool, err error) {
	err = q.QueryRow(ctx, `
		INSERT INTO recipe (name, normalised_name, portions, active_minutes, total_minutes, slot_kinds, appliances, steps)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, normalised_name) DO NOTHING
		RETURNING id`,
		rec.Name, normalised, rec.Portions, rec.ActiveMinutes, rec.TotalMinutes, slotKindsOf(rec.Kinds), rec.Appliances, rec.Steps).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// updateRecipe sets the signed-in User's Recipe id to rec under its normalised name, or
// is ErrNoRecipe.
func updateRecipe(ctx context.Context, q db.Querier, id int64, rec domain.Recipe, normalised string) error {
	tag, err := q.Exec(ctx, `
		UPDATE recipe SET name = $2, normalised_name = $3, portions = $4, active_minutes = $5, total_minutes = $6,
			slot_kinds = $7, appliances = $8, steps = $9
		WHERE id = $1`,
		id, rec.Name, normalised, rec.Portions, rec.ActiveMinutes, rec.TotalMinutes, slotKindsOf(rec.Kinds), rec.Appliances, rec.Steps)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNoRecipe
	}
	return err
}

// deleteRecipeIngredients takes every Ingredient off the Recipe id.
func deleteRecipeIngredients(ctx context.Context, q db.Querier, id int64) error {
	_, err := q.Exec(ctx, `DELETE FROM recipe_ingredient WHERE recipe_id = $1`, id)
	return err
}

// deleteRecipe deletes the signed-in User's Recipe id and is its name, or ErrNoRecipe.
func deleteRecipe(ctx context.Context, q db.Querier, id int64) (name string, err error) {
	err = q.QueryRow(ctx, `DELETE FROM recipe WHERE id = $1 RETURNING name`, id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoRecipe
	}
	return name, err
}

// saveRecipeIngredients lists rec's Ingredients, every one saved, on the Recipe id.
func saveRecipeIngredients(ctx context.Context, q db.Querier, id int64, rec domain.Recipe) error {
	for i, ri := range rec.Ingredients {
		if _, err := q.Exec(ctx, `
			INSERT INTO recipe_ingredient (recipe_id, position, ingredient_id, quantity) VALUES ($1, $2, $3, $4)`,
			id, i, ri.Ingredient.ID, ri.Quantity.Number()); err != nil {
			return err
		}
	}
	return nil
}

// loadTaken is the id and name of the signed-in User's Recipe under the key normalised,
// other than the one except, or 0 if there is none.
func loadTaken(ctx context.Context, q db.Querier, normalised string, except int64) (id int64, name string, err error) {
	err = q.QueryRow(ctx, `SELECT id, name FROM recipe WHERE normalised_name = $1 AND id <> $2`, normalised, except).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil
	}
	return id, name, err
}

// loadRecipes is the signed-in User's Recipes by name with their Ingredients, or only the
// one with the id only, if that isn't nil.
func loadRecipes(ctx context.Context, q db.Querier, only *int64) ([]domain.Recipe, error) {
	rows, err := q.Query(ctx, `
		SELECT id, name, portions, active_minutes, total_minutes, slot_kinds, appliances, steps FROM recipe
		WHERE $1::bigint IS NULL OR id = $1
		ORDER BY normalised_name`, only)
	if err != nil {
		return nil, err
	}
	recipes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Recipe, error) {
		var r domain.Recipe
		var kinds []string
		err := row.Scan(&r.ID, &r.Name, &r.Portions, &r.ActiveMinutes, &r.TotalMinutes, &kinds, &r.Appliances, &r.Steps)
		for _, k := range kinds {
			r.Kinds = append(r.Kinds, domain.SlotKind(k))
		}
		return r, err
	})
	if err != nil || len(recipes) == 0 {
		return recipes, err
	}
	known, err := ingredients.LoadIngredients(ctx, q)
	if err != nil {
		return nil, err
	}
	byID := map[int64]domain.Ingredient{}
	for _, ing := range known {
		byID[ing.ID] = ing
	}
	at := map[int64]int{}
	for i, r := range recipes {
		at[r.ID] = i
	}
	rows, err = q.Query(ctx, `
		SELECT recipe_id, ingredient_id, quantity FROM recipe_ingredient
		WHERE $1::bigint IS NULL OR recipe_id = $1
		ORDER BY recipe_id, position`, only)
	if err != nil {
		return nil, err
	}
	var recipeID, ingredientID int64
	var quantity float64
	_, err = pgx.ForEachRow(rows, []any{&recipeID, &ingredientID, &quantity}, func() error {
		if i, ok := at[recipeID]; ok {
			recipes[i].Ingredients = append(recipes[i].Ingredients, domain.RecipeIngredient{
				Ingredient: byID[ingredientID], Quantity: domain.QuantityOf(quantity),
			})
		}
		return nil
	})
	return recipes, err
}
