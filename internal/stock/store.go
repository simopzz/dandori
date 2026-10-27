package stock

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

func loadItems(ctx context.Context, q db.Querier) ([]Item, error) {
	rows, _ := q.Query(ctx, `
		SELECT s.id, s.quantity, s.location, s.expires_on, i.name, i.unit
		FROM stock_item s JOIN ingredient i ON i.id = s.ingredient_id
		ORDER BY i.name, s.id`)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Item, error) {
		var it Item
		err := row.Scan(&it.ID, &it.Quantity, &it.Location, &it.ExpiresOn, &it.Ingredient.Name, &it.Ingredient.Unit)
		return it, err
	})
}

func loadIngredients(ctx context.Context, q db.Querier) ([]domain.Ingredient, error) {
	rows, _ := q.Query(ctx, `SELECT name, unit FROM ingredient ORDER BY name`)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Ingredient, error) {
		var ing domain.Ingredient
		err := row.Scan(&ing.Name, &ing.Unit)
		return ing, err
	})
}

// insertItem stores item, returning ErrUnknownIngredient when its
// Ingredient isn't known.
func insertItem(ctx context.Context, q db.Querier, item domain.StockItem) error {
	tag, err := q.Exec(ctx, `
		INSERT INTO stock_item (ingredient_id, quantity, location, expires_on)
		SELECT id, $2, $3, $4 FROM ingredient WHERE name = $1`,
		item.Ingredient.Name, item.Quantity, item.Location, item.ExpiresOn)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnknownIngredient
	}
	return nil
}

func updateItem(ctx context.Context, q db.Querier, id int64, item domain.StockItem) error {
	tag, err := q.Exec(ctx, `
		UPDATE stock_item SET quantity = $2, location = $3, expires_on = $4 WHERE id = $1`,
		id, item.Quantity, item.Location, item.ExpiresOn)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func deleteItem(ctx context.Context, q db.Querier, id int64) error {
	_, err := q.Exec(ctx, `DELETE FROM stock_item WHERE id = $1`, id)
	return err
}

// loadPortions loads every Portion cooked by today and not eaten before
// it. A Prep Session from before planning has no day: it is cooked.
func loadPortions(ctx context.Context, q db.Querier, today time.Time) ([]Portion, error) {
	rows, _ := q.Query(ctx, `
		SELECT p.id, coalesce(r.name, ''), p.location, s.day, s.kind
		FROM portion p
		JOIN prep_session ps ON ps.id = p.prep_session_id
		LEFT JOIN recipe r ON r.id = ps.recipe_id
		LEFT JOIN slot s ON s.portion_id = p.id
		WHERE (ps.prepared_on IS NULL OR ps.prepared_on <= $1)
		  AND (s.day IS NULL OR s.day >= $1)
		ORDER BY r.name, s.day, p.id`, today)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Portion, error) {
		var p Portion
		var day *time.Time
		var kind *domain.SlotKind
		err := row.Scan(&p.ID, &p.Recipe, &p.Location, &day, &kind)
		if day != nil {
			p.Slot = &domain.Slot{On: *day, Kind: *kind}
		}
		return p, err
	})
}

func movePortion(ctx context.Context, q db.Querier, id int64, location domain.StorageLocation) error {
	tag, err := q.Exec(ctx, `UPDATE portion SET location = $2 WHERE id = $1`, id, location)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPortionNotFound
	}
	return nil
}
