-- +goose Up
-- A quantity of one Ingredient, in the Ingredient's unit, held in a Storage
-- Location.
CREATE TABLE stock_item (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ingredient_id bigint  NOT NULL REFERENCES ingredient,
    quantity      numeric NOT NULL CHECK (quantity > 0),
    location      text    NOT NULL CHECK (location IN ('pantry', 'fridge', 'freezer')),
    expires_on    date
);

-- One preparation of food that yields Portions: of a Recipe, or of
-- Ingredients eaten as they are when recipe_id is null. Deleting a Recipe
-- unlinks its Prep Sessions, so their Portions stay.
CREATE TABLE prep_session (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recipe_id bigint REFERENCES recipe ON DELETE SET NULL
);

-- One ready-to-eat serving a Prep Session yields, kept in a Storage
-- Location.
CREATE TABLE portion (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    prep_session_id bigint NOT NULL REFERENCES prep_session ON DELETE CASCADE,
    location        text   NOT NULL CHECK (location IN ('pantry', 'fridge', 'freezer'))
);

-- +goose Down
DROP TABLE portion;
DROP TABLE prep_session;
DROP TABLE stock_item;
