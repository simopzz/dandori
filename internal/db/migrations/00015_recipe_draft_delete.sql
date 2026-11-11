-- +goose Up
-- Accepting or discarding a Recipe Draft deletes it.
GRANT DELETE ON recipe_draft TO authenticated;

-- +goose Down
REVOKE DELETE ON recipe_draft FROM authenticated;
