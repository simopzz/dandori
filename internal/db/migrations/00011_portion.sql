-- +goose Up
-- A User's Portions, each one ready-to-eat serving with its values per Portion; fibre and
-- sugar are null when unknown, which is never 0. A Portion has no Storage Location or
-- status: where it is and whether the kitchen still holds it are what its Movements add
-- up to (ADR-0013).
CREATE TABLE portion (
    id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id uuid NOT NULL DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    name    text NOT NULL CHECK (name <> ''),
    kcal    numeric(5, 1) NOT NULL CHECK (kcal >= 0),
    protein numeric(5, 1) NOT NULL CHECK (protein >= 0),
    carbs   numeric(5, 1) NOT NULL CHECK (carbs >= 0),
    fat     numeric(5, 1) NOT NULL CHECK (fat >= 0),
    fibre   numeric(5, 1) CHECK (fibre >= 0),
    sugar   numeric(5, 1) CHECK (sugar >= 0),
    UNIQUE (user_id, id)
);

ALTER TABLE portion ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT ON portion TO authenticated;

CREATE POLICY own_rows ON portion
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- A Movement is about one Ingredient or a set of Portions: one act is one Movement, with
-- change the whole number of Portions and movement_portion listing them, append-only as
-- Movements are.
ALTER TABLE movement
    ALTER COLUMN ingredient_id DROP NOT NULL,
    ADD UNIQUE (user_id, id),
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted', 'moved', 'staple', 'added')),
    ADD CONSTRAINT movement_added_check CHECK (
        cause_kind <> 'added'
        OR (ingredient_id IS NULL AND change > 0 AND believed IS NULL AND stated IS NULL)
    ),
    ADD CONSTRAINT movement_portions_whole_check CHECK (ingredient_id IS NOT NULL OR change = trunc(change));

-- Deferred, as movement's reference to its Ingredient is, so deleting a User, which
-- cascades to every table in no set order, can finish.
CREATE TABLE movement_portion (
    user_id     uuid NOT NULL DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    movement_id bigint NOT NULL,
    portion_id  bigint NOT NULL,
    PRIMARY KEY (movement_id, portion_id),
    FOREIGN KEY (user_id, movement_id) REFERENCES movement (user_id, id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (user_id, portion_id) REFERENCES portion (user_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX ON movement_portion (user_id, portion_id);

ALTER TABLE movement_portion ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT ON movement_portion TO authenticated;

CREATE POLICY own_rows ON movement_portion
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
-- The Ledger forgets Portions: nothing without an Ingredient can stay.
DROP TABLE movement_portion;
DELETE FROM movement WHERE ingredient_id IS NULL;
ALTER TABLE movement
    DROP CONSTRAINT movement_portions_whole_check,
    DROP CONSTRAINT movement_added_check,
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted', 'moved', 'staple')),
    DROP CONSTRAINT movement_user_id_id_key,
    ALTER COLUMN ingredient_id SET NOT NULL;
DROP TABLE portion;
