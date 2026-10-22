-- +goose Up
-- The single Profile of the app: the id column only ever holds true.
CREATE TABLE profile (
    id                  boolean PRIMARY KEY DEFAULT true CHECK (id),

    target_kcal         integer NOT NULL CHECK (target_kcal > 0),
    target_protein_g    integer NOT NULL CHECK (target_protein_g >= 0),
    target_carbs_g      integer NOT NULL CHECK (target_carbs_g >= 0),
    target_fat_g        integer NOT NULL CHECK (target_fat_g >= 0),

    -- Normalised food terms (lowercase, single-spaced) the planner and
    -- Recipe generation check Ingredients against.
    exclusions          text[]  NOT NULL DEFAULT '{}',
    allergies           text[]  NOT NULL DEFAULT '{}',

    meal_out_kcal       integer NOT NULL CHECK (meal_out_kcal >= 0),
    meal_out_protein_g  integer NOT NULL CHECK (meal_out_protein_g >= 0),
    meal_out_carbs_g    integer NOT NULL CHECK (meal_out_carbs_g >= 0),
    meal_out_fat_g      integer NOT NULL CHECK (meal_out_fat_g >= 0),

    untracked_kcal      integer NOT NULL CHECK (untracked_kcal >= 0),
    untracked_protein_g integer NOT NULL CHECK (untracked_protein_g >= 0),
    untracked_carbs_g   integer NOT NULL CHECK (untracked_carbs_g >= 0),
    untracked_fat_g     integer NOT NULL CHECK (untracked_fat_g >= 0),

    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE profile;
