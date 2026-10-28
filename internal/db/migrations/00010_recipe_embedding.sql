-- +goose Up
-- A Recipe's embedding, for searching the Recipe Library by meaning, made
-- from the Recipe as it was at recipe_updated_at. One made from an older
-- version of its Recipe, or by another model than the configured one, is
-- stale: it is computed again before the next search, as is a missing one.
-- The vector has no fixed size, so the model can change.
CREATE TABLE recipe_embedding (
    recipe_id         bigint      PRIMARY KEY REFERENCES recipe ON DELETE CASCADE,
    recipe_updated_at timestamptz NOT NULL,
    model             text        NOT NULL,
    embedding         vector      NOT NULL
);

-- +goose Down
DROP TABLE recipe_embedding;
