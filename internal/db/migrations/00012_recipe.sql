-- +goose Up
-- A User's Recipes. name is as the User typed it, and normalised_name the key no two of
-- theirs share (domain.NormaliseRecipeName). A Recipe's Ingredients are quantities of known
-- Ingredients in each one's unit, each once. An Ingredient a Recipe lists can't be deleted.
CREATE TABLE recipe (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         uuid NOT NULL DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    name            text NOT NULL CHECK (name <> ''),
    normalised_name text NOT NULL CHECK (normalised_name <> ''),
    portions        integer NOT NULL CHECK (portions >= 1),
    active_minutes  integer NOT NULL CHECK (active_minutes >= 0),
    total_minutes   integer NOT NULL CHECK (total_minutes > 0),
    slot_kinds      text[] NOT NULL CHECK (cardinality(slot_kinds) > 0 AND slot_kinds <@ ARRAY['breakfast', 'lunch', 'snack', 'dinner']),
    appliances      text[] NOT NULL,
    steps           text[] NOT NULL,
    CHECK (active_minutes <= total_minutes),
    UNIQUE (user_id, normalised_name)
);

CREATE TABLE recipe_ingredient (
    recipe_id     bigint NOT NULL REFERENCES recipe (id) ON DELETE CASCADE,
    position      integer NOT NULL,
    user_id       uuid NOT NULL DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    ingredient_id bigint NOT NULL,
    quantity      numeric(8, 1) NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (recipe_id, position),
    UNIQUE (recipe_id, ingredient_id),
    -- Deferred, so deleting a User, which cascades in no set order, can finish.
    CONSTRAINT recipe_ingredient_ingredient_fkey FOREIGN KEY (user_id, ingredient_id)
        REFERENCES ingredient (user_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX ON recipe_ingredient (ingredient_id);

ALTER TABLE recipe ENABLE ROW LEVEL SECURITY;
ALTER TABLE recipe_ingredient ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT ON recipe, recipe_ingredient TO authenticated;

CREATE POLICY own_rows ON recipe
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

CREATE POLICY own_rows ON recipe_ingredient
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE recipe_ingredient;
DROP TABLE recipe;
