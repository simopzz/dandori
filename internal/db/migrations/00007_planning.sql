-- +goose Up
-- The stretch one grocery delivery has to last. Cycles never overlap: a
-- delivery before ends_on closes the Cycle early, on closed_on.
CREATE TABLE cycle (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    delivery_on date NOT NULL UNIQUE,
    ends_on     date NOT NULL CHECK (ends_on > delivery_on),
    closed_on   date CHECK (closed_on > delivery_on AND closed_on <= ends_on)
);

-- A Prep Session of a Cycle's Plan is prepared for an eating occasion of
-- one of its days. The columns are null for Prep Sessions from before
-- planning.
ALTER TABLE prep_session
    ADD COLUMN cycle_id      bigint REFERENCES cycle,
    ADD COLUMN prepared_on   date,
    ADD COLUMN prepared_for  text CHECK (prepared_for IN ('breakfast', 'lunch', 'snack', 'dinner'));

-- What a Prep Session without a Recipe sets out, for all its Portions
-- together; quantity is in the Ingredient's unit.
CREATE TABLE prep_session_ingredient (
    prep_session_id bigint  NOT NULL REFERENCES prep_session ON DELETE CASCADE,
    position        integer NOT NULL,
    ingredient_id   bigint  NOT NULL REFERENCES ingredient,
    quantity        numeric NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (prep_session_id, position)
);

-- An eating occasion of a Cycle, filled by a Portion. A Portion with no
-- Slot is a Portion without a Slot.
CREATE TABLE slot (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cycle_id   bigint NOT NULL REFERENCES cycle ON DELETE CASCADE,
    day        date   NOT NULL,
    kind       text   NOT NULL CHECK (kind IN ('breakfast', 'lunch', 'snack', 'dinner')),
    portion_id bigint NOT NULL UNIQUE REFERENCES portion ON DELETE CASCADE,
    UNIQUE (cycle_id, day, kind)
);

-- The Profile's exclusions and allergies (normalised food terms) the user
-- confirmed an Ingredient falls under: Plan validation matches on these.
CREATE TABLE ingredient_falls_under (
    ingredient_id bigint NOT NULL REFERENCES ingredient ON DELETE CASCADE,
    term          text   NOT NULL,
    PRIMARY KEY (ingredient_id, term)
);

-- A set of changes to a Cycle's Plan awaiting the user's decision. plan is
-- the whole proposed Plan, as the planner submitted it; approving it
-- replaces the Cycle's Plan.
CREATE TABLE plan_proposal (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cycle_id   bigint NOT NULL REFERENCES cycle ON DELETE CASCADE,
    origin     text   NOT NULL CHECK (origin IN ('initial', 'substitution')),
    plan       jsonb  NOT NULL,
    status     text   NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    -- Why it was rejected, if said: the user's note, or that its Cycle
    -- closed early.
    note       text   NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz
);

-- +goose Down
DROP TABLE plan_proposal;
DROP TABLE ingredient_falls_under;
DROP TABLE slot;
DROP TABLE prep_session_ingredient;
ALTER TABLE prep_session DROP COLUMN prepared_for, DROP COLUMN prepared_on, DROP COLUMN cycle_id;
DROP TABLE cycle;
