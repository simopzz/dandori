package stock

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// itemsQuery selects every Stock Item by Ingredient name, then in the order
// they were recorded.
const itemsQuery = `
	SELECT s.id, s.quantity, s.location, i.name, i.unit
	FROM stock_item s JOIN ingredient i ON i.id = s.ingredient_id
	ORDER BY i.name, s.id`

func loadItems(ctx context.Context, q db.Querier) ([]Item, error) {
	return queryItems(ctx, q, itemsQuery)
}

// lockItems loads every Stock Item as loadItems does, locking them until
// q's transaction ends, so a change the user makes meanwhile waits for it.
func lockItems(ctx context.Context, q db.Querier) ([]Item, error) {
	return queryItems(ctx, q, itemsQuery+` FOR UPDATE OF s`)
}

func queryItems(ctx context.Context, q db.Querier, sql string) ([]Item, error) {
	rows, _ := q.Query(ctx, sql)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Item, error) {
		var it Item
		err := row.Scan(&it.ID, &it.Quantity, &it.Location, &it.Ingredient.Name, &it.Ingredient.Unit)
		return it, err
	})
}

// insertItem stores item, returning ErrUnknownIngredient when its
// Ingredient isn't known.
func insertItem(ctx context.Context, q db.Querier, item domain.StockItem) error {
	tag, err := q.Exec(ctx, `
		INSERT INTO stock_item (ingredient_id, quantity, location)
		SELECT id, $2, $3 FROM ingredient WHERE name = $1`,
		item.Ingredient.Name, item.Quantity, item.Location)
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
		UPDATE stock_item SET quantity = $2, location = $3 WHERE id = $1`,
		id, item.Quantity, item.Location)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// setQuantity sets what the Stock Item with id holds to quantity, removing
// it when that is 0: domain.UseUp leaves exactly 0 of a Stock Item it uses
// up, however close to it float arithmetic came.
func setQuantity(ctx context.Context, q db.Querier, id int64, quantity float64) error {
	if quantity == 0 {
		return deleteItem(ctx, q, id)
	}
	_, err := q.Exec(ctx, `UPDATE stock_item SET quantity = $2 WHERE id = $1`, id, quantity)
	return err
}

// insertTaken records that the Prep Session with prepSessionID took t of
// the Stock Item with stockItemID.
func insertTaken(ctx context.Context, q db.Querier, prepSessionID, stockItemID int64, t domain.Taking) error {
	_, err := q.Exec(ctx, `
		INSERT INTO stock_taken (prep_session_id, stock_item_id, ingredient_id, quantity, location)
		SELECT $1, $2, id, $4, $5 FROM ingredient WHERE name = $3`,
		prepSessionID, stockItemID, t.Ingredient.Name, t.Quantity, t.Location)
	return err
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
