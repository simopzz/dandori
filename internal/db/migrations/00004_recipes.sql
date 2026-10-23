-- +goose Up
-- An Ingredient is measured in one unit; its nutritional values are per
-- 100 g, per 100 ml or per piece accordingly.
CREATE TABLE ingredient (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Normalised (lowercase, single-spaced); how Recipes are matched to it.
    name      text    NOT NULL UNIQUE,
    unit      text    NOT NULL CHECK (unit IN ('g', 'ml', 'piece')),
    kcal      numeric NOT NULL CHECK (kcal >= 0),
    protein_g numeric NOT NULL CHECK (protein_g >= 0),
    carbs_g   numeric NOT NULL CHECK (carbs_g >= 0),
    fat_g     numeric NOT NULL CHECK (fat_g >= 0)
);

-- A Recipe in the Recipe Library.
CREATE TABLE recipe (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         text    NOT NULL,
    portions     integer NOT NULL CHECK (portions >= 1),
    prep_minutes integer NOT NULL CHECK (prep_minutes >= 0),
    appliances   text[]  NOT NULL DEFAULT '{}',
    steps        text[]  NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- A Recipe's Ingredients, in the order the Recipe lists them; quantity is
-- in the Ingredient's unit.
CREATE TABLE recipe_ingredient (
    recipe_id     bigint  NOT NULL REFERENCES recipe ON DELETE CASCADE,
    position      integer NOT NULL,
    ingredient_id bigint  NOT NULL REFERENCES ingredient,
    quantity      numeric NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (recipe_id, position),
    UNIQUE (recipe_id, ingredient_id)
);

-- The Recipe a parse Run drafted, awaiting the user's edits and acceptance.
-- Accepting or discarding it deletes it.
CREATE TABLE recipe_draft (
    run_id bigint PRIMARY KEY REFERENCES run ON DELETE CASCADE,
    recipe jsonb  NOT NULL
);

-- +goose Down
DROP TABLE recipe_draft;
DROP TABLE recipe_ingredient;
DROP TABLE recipe;
DROP TABLE ingredient;
