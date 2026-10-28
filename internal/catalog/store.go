package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

const productColumns = `id, code, name, pack_quantity, unit, kcal, protein_g, carbs_g, fat_g, aisle, price, available, availability_seen_at`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	var kcal, protein, carbs, fat *float64
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.PackQuantity, &p.Unit, &kcal, &protein, &carbs, &fat,
		&p.Aisle, &p.Price, &p.Available, &p.AvailabilitySeen)
	if kcal != nil {
		p.Nutrition = &domain.IngredientNutrition{Kcal: *kcal, ProteinG: *protein, CarbsG: *carbs, FatG: *fat}
	}
	return p, err
}

// loadProducts loads every Catalog Product, by aisle and name.
func loadProducts(ctx context.Context, q db.Querier) ([]Product, error) {
	rows, _ := q.Query(ctx, `SELECT `+productColumns+` FROM catalog_product ORDER BY aisle, name`)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Product, error) { return scanProduct(row) })
}

// loadProduct loads the Catalog Product with id, or with code when id is 0.
func loadProduct(ctx context.Context, q db.Querier, id int64, code string) (Product, bool, error) {
	p, err := scanProduct(q.QueryRow(ctx, `SELECT `+productColumns+` FROM catalog_product WHERE id = $1 OR code = $2`, id, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, false, nil
	}
	return p, err == nil, err
}

// saveProduct stores p, updating the Catalog Product with its code if
// there is one, so a seed run again refreshes it and keeps its mappings.
func saveProduct(ctx context.Context, q db.Querier, p domain.CatalogProduct) error {
	var kcal, protein, carbs, fat *float64
	if n := p.Nutrition; n != nil {
		kcal, protein, carbs, fat = &n.Kcal, &n.ProteinG, &n.CarbsG, &n.FatG
	}
	_, err := q.Exec(ctx, `
		INSERT INTO catalog_product (code, name, pack_quantity, unit, kcal, protein_g, carbs_g, fat_g, aisle, price, available, availability_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name, pack_quantity = EXCLUDED.pack_quantity, unit = EXCLUDED.unit,
			kcal = EXCLUDED.kcal, protein_g = EXCLUDED.protein_g, carbs_g = EXCLUDED.carbs_g, fat_g = EXCLUDED.fat_g,
			aisle = EXCLUDED.aisle, price = EXCLUDED.price,
			available = EXCLUDED.available, availability_seen_at = EXCLUDED.availability_seen_at`,
		p.Code, p.Name, p.PackQuantity, p.Unit, kcal, protein, carbs, fat, p.Aisle, p.Price, p.Available, p.AvailabilitySeen)
	return err
}

// loadMappings loads every Ingredient, by name, with its Product Mapping
// if it has one.
func loadMappings(ctx context.Context, q db.Querier) ([]Mapping, error) {
	rows, _ := q.Query(ctx, `
		SELECT i.id, i.name, i.unit, cp.id, cp.name, coalesce(m.proposed, false)
		FROM ingredient i
		LEFT JOIN product_mapping m ON m.ingredient_id = i.id
		LEFT JOIN catalog_product cp ON cp.id = m.catalog_product_id
		ORDER BY i.name`)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Mapping, error) {
		var m Mapping
		var productID *int64
		var productName *string
		err := row.Scan(&m.IngredientID, &m.Ingredient.Name, &m.Ingredient.Unit, &productID, &productName, &m.Proposed)
		if productID != nil {
			m.ProductID, m.ProductName = *productID, *productName
		}
		return m, err
	})
}

// loadIngredient loads the Ingredient with id, or named name when id is 0.
func loadIngredient(ctx context.Context, q db.Querier, id int64, name string) (int64, domain.Ingredient, bool, error) {
	var ing domain.Ingredient
	err := q.QueryRow(ctx, `SELECT id, name, unit FROM ingredient WHERE id = $1 OR name = $2`, id, name).
		Scan(&id, &ing.Name, &ing.Unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.Ingredient{}, false, nil
	}
	return id, ing, err == nil, err
}

// setMapping makes productID the user's Product Mapping for the Ingredient,
// or removes its mapping when productID is 0.
func setMapping(ctx context.Context, q db.Querier, ingredientID, productID int64) error {
	if productID == 0 {
		_, err := q.Exec(ctx, `DELETE FROM product_mapping WHERE ingredient_id = $1`, ingredientID)
		return err
	}
	_, err := q.Exec(ctx, `
		INSERT INTO product_mapping (ingredient_id, catalog_product_id, proposed) VALUES ($1, $2, false)
		ON CONFLICT (ingredient_id) DO UPDATE SET catalog_product_id = EXCLUDED.catalog_product_id, proposed = false`,
		ingredientID, productID)
	return err
}

// proposeMapping stores productID as a proposed Product Mapping for the
// Ingredient unless it already has a mapping, reporting whether it did.
func proposeMapping(ctx context.Context, q db.Querier, ingredientID, productID int64) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO product_mapping (ingredient_id, catalog_product_id, proposed) VALUES ($1, $2, true)
		ON CONFLICT (ingredient_id) DO NOTHING`,
		ingredientID, productID)
	return tag.RowsAffected() == 1, err
}
