-- +goose Up
-- The Profile's daily Targets: calories, and grams of protein and fibre as floors and of
-- sugar as a ceiling. They are null until the User sets them, all four at once, and
-- planning needs them. The calorie tolerance, a percent either side, starts at 5.
ALTER TABLE profile
    ADD COLUMN target_kcal    integer CHECK (target_kcal > 0),
    ADD COLUMN target_protein integer CHECK (target_protein > 0),
    ADD COLUMN target_fibre   integer CHECK (target_fibre > 0),
    ADD COLUMN target_sugar   integer CHECK (target_sugar > 0),
    ADD COLUMN kcal_tolerance integer NOT NULL DEFAULT 5 CHECK (kcal_tolerance BETWEEN 1 AND 20),
    ADD CHECK (num_nulls(target_kcal, target_protein, target_fibre, target_sugar) IN (0, 4));

-- +goose Down
ALTER TABLE profile
    DROP COLUMN kcal_tolerance, DROP COLUMN target_sugar, DROP COLUMN target_fibre,
    DROP COLUMN target_protein, DROP COLUMN target_kcal;
