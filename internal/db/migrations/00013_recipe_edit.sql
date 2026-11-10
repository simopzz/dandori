-- +goose Up
-- A User edits and deletes their Recipes; an edit replaces a Recipe's Ingredient rows.
GRANT UPDATE, DELETE ON recipe TO authenticated;
GRANT DELETE ON recipe_ingredient TO authenticated;

-- +goose Down
REVOKE DELETE ON recipe_ingredient FROM authenticated;
REVOKE UPDATE, DELETE ON recipe FROM authenticated;
