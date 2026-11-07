-- +goose Up
-- The User edits and deletes their Ingredients. An Ingredient a Movement refers to can't
-- be deleted: movement's foreign key restricts it, at commit, as it is deferred.
GRANT UPDATE, DELETE ON ingredient TO authenticated;

-- +goose Down
REVOKE UPDATE, DELETE ON ingredient FROM authenticated;
