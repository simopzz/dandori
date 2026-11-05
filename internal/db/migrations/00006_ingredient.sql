-- +goose Up
-- A User's Ingredients, added by hand. name is as the User typed it, and normalised_name
-- the key that identifies the food (domain.NormaliseIngredientName), one per User. The
-- nutritional values are per 100 g, per 100 ml or per piece, as unit says; fibre and sugar
-- are null when unknown, which is never 0.
CREATE TABLE ingredient (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          uuid NOT NULL DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    name             text NOT NULL CHECK (name <> ''),
    normalised_name  text NOT NULL CHECK (normalised_name <> ''),
    unit             text NOT NULL CHECK (unit IN ('g', 'ml', 'piece')),
    kcal             numeric(5, 1) NOT NULL CHECK (kcal >= 0),
    protein          numeric(5, 1) NOT NULL CHECK (protein >= 0),
    carbs            numeric(5, 1) NOT NULL CHECK (carbs >= 0),
    fat              numeric(5, 1) NOT NULL CHECK (fat >= 0),
    fibre            numeric(5, 1) CHECK (fibre >= 0),
    sugar            numeric(5, 1) CHECK (sugar >= 0),
    storage_location text NOT NULL CHECK (storage_location IN ('pantry', 'fridge', 'freezer')),
    staple           boolean NOT NULL,
    UNIQUE (user_id, normalised_name)
);

ALTER TABLE ingredient ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT ON ingredient TO authenticated;

CREATE POLICY own_rows ON ingredient
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE ingredient;
