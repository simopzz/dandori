-- +goose Up
-- Whether freezer stock of an Ingredient is used without thawing. Leave existing rows
-- null for review; non-freezer Ingredients have no such property.
ALTER TABLE ingredient ADD COLUMN use_from_frozen boolean;

ALTER TABLE ingredient ADD CONSTRAINT ingredient_use_from_frozen_location CHECK (
    use_from_frozen IS NULL OR storage_location = 'freezer'
);

-- +goose Down
ALTER TABLE ingredient DROP CONSTRAINT ingredient_use_from_frozen_location;
ALTER TABLE ingredient DROP COLUMN use_from_frozen;
