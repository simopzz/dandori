-- +goose Up
-- The Profile's Meal Out estimate: what a Meal Out counts unless its Exception gives its
-- own, calories and grams of the rest as whole numbers from 0. Fibre and sugar are null
-- when unknown, never 0. Every Profile starts from the default.
ALTER TABLE profile
    ADD COLUMN meal_out_kcal    integer NOT NULL DEFAULT 1000 CHECK (meal_out_kcal >= 0),
    ADD COLUMN meal_out_protein integer NOT NULL DEFAULT 35 CHECK (meal_out_protein >= 0),
    ADD COLUMN meal_out_carbs   integer NOT NULL DEFAULT 110 CHECK (meal_out_carbs >= 0),
    ADD COLUMN meal_out_fat     integer NOT NULL DEFAULT 45 CHECK (meal_out_fat >= 0),
    ADD COLUMN meal_out_fibre   integer DEFAULT 6 CHECK (meal_out_fibre >= 0),
    ADD COLUMN meal_out_sugar   integer DEFAULT 12 CHECK (meal_out_sugar >= 0);

-- +goose Down
ALTER TABLE profile
    DROP COLUMN meal_out_sugar, DROP COLUMN meal_out_fibre, DROP COLUMN meal_out_fat,
    DROP COLUMN meal_out_carbs, DROP COLUMN meal_out_protein, DROP COLUMN meal_out_kcal;
