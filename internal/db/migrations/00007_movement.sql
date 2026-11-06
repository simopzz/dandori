-- +goose Up
-- The Kitchen Ledger (ADR-0013): every change to what the kitchen holds, append-only, so
-- the User's role may insert and read Movements but never update or delete one. Stock Items
-- are not stored: they are what the Movements add up to, per Ingredient and Storage
-- Location. change is the signed quantity in the Ingredient's unit. A cause is a kind plus
-- a reference to what caused it, which no kind has yet. A Count keeps what Dandori believed
-- and what the User stated, whose difference is its change.
ALTER TABLE ingredient ADD UNIQUE (user_id, id);

CREATE TABLE movement (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          uuid NOT NULL DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    happened_at      timestamptz NOT NULL,
    ingredient_id    bigint NOT NULL,
    storage_location text NOT NULL CHECK (storage_location IN ('pantry', 'fridge', 'freezer')),
    change           numeric(8, 1) NOT NULL,
    cause_kind       text NOT NULL CHECK (cause_kind IN ('counted')),
    cause_ref        bigint,
    note             text CHECK (note <> ''),
    believed         numeric(8, 1) CHECK (believed >= 0),
    stated           numeric(8, 1) CHECK (stated >= 0),
    CHECK (cause_kind <> 'counted' OR (believed IS NOT NULL AND stated IS NOT NULL AND change = stated - believed)),
    -- Deferred, so deleting a User, which cascades to both tables in no set order, can
    -- finish; while Movements remain at commit, their Ingredient can't be deleted.
    FOREIGN KEY (user_id, ingredient_id) REFERENCES ingredient (user_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX ON movement (user_id, happened_at, id);

ALTER TABLE movement ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT ON movement TO authenticated;

CREATE POLICY own_rows ON movement
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE movement;
ALTER TABLE ingredient DROP CONSTRAINT ingredient_user_id_id_key;
