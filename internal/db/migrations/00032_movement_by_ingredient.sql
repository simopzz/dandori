-- +goose Up
-- A Count, a move and a Stock Item page read one Ingredient's Movements.
CREATE INDEX ON movement (user_id, ingredient_id, happened_at, id);

-- +goose Down
DROP INDEX movement_user_id_ingredient_id_happened_at_id_idx;
