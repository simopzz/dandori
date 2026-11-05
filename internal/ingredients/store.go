package ingredients

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// CountIngredients is how many Ingredients the signed-in User has, for the Profile's Food
// Terms. It has no WHERE on purpose: run in db.AsUser, row-level security leaves only
// their rows.
func CountIngredients(ctx context.Context, q db.Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM ingredient`).Scan(&n)
	return n, err
}

// saveIngredient adds ing for the signed-in User under its normalised name, unless they
// have an Ingredient of that name; added says whether it did.
func saveIngredient(ctx context.Context, q db.Querier, ing domain.Ingredient, normalised string) (added bool, err error) {
	n := ing.Nutrition
	tag, err := q.Exec(ctx, `
		INSERT INTO ingredient (name, normalised_name, unit, kcal, protein, carbs, fat, fibre, sugar, storage_location, staple)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (user_id, normalised_name) DO NOTHING`,
		ing.Name, normalised, string(ing.Unit), n.Kcal.Value, n.Protein.Value, n.Carbs.Value, n.Fat.Value,
		nullable(n.Fibre), nullable(n.Sugar), string(ing.Usual), ing.Staple)
	return tag.RowsAffected() == 1, err
}

// loadNameOf is the name of the signed-in User's Ingredient under the key normalised.
func loadNameOf(ctx context.Context, q db.Querier, normalised string) (string, error) {
	var name string
	err := q.QueryRow(ctx, `SELECT name FROM ingredient WHERE normalised_name = $1`, normalised).Scan(&name)
	return name, err
}

// loadIngredients is the signed-in User's Ingredients by name. It has no WHERE on
// purpose: run in db.AsUser, row-level security leaves only their rows.
func loadIngredients(ctx context.Context, q db.Querier) ([]domain.Ingredient, error) {
	rows, err := q.Query(ctx, `
		SELECT name, unit, kcal, protein, carbs, fat, fibre, sugar, storage_location, staple
		FROM ingredient ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Ingredient, error) {
		var (
			ing                       domain.Ingredient
			unit, usual               string
			kcal, protein, carbs, fat float64
			fibre, sugar              *float64
		)
		err := row.Scan(&ing.Name, &unit, &kcal, &protein, &carbs, &fat, &fibre, &sugar, &usual, &ing.Staple)
		ing.Unit, ing.Usual = domain.Unit(unit), domain.StorageLocation(usual)
		ing.Nutrition = domain.Nutrition{
			Kcal: domain.Known(kcal), Protein: domain.Known(protein), Carbs: domain.Known(carbs), Fat: domain.Known(fat),
			Fibre: amount(fibre), Sugar: amount(sugar),
		}
		return ing, err
	})
}

// nullable is a as stored: null when Unknown.
func nullable(a domain.Amount) *float64 {
	if !a.Known {
		return nil
	}
	return &a.Value
}

// amount is a stored value, null being Unknown.
func amount(v *float64) domain.Amount {
	if v == nil {
		return domain.Unknown
	}
	return domain.Known(*v)
}
