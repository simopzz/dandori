-- +goose Up
-- Which Cycle a planning Run plans, from which Slot: a day and a kind, never a time, so a
-- changed Slot time moves it. It commits with its Run (runs.Engine.Start's link), so no
-- planning Run is attached to no Cycle.
CREATE TABLE planning_run (
    run_id     bigint PRIMARY KEY,
    user_id    uuid NOT NULL DEFAULT current_user_id(),
    cycle_id   bigint NOT NULL,
    first_on   date NOT NULL,
    first_kind text NOT NULL CHECK (first_kind IN ('breakfast', 'lunch', 'snack', 'dinner')),
    UNIQUE (user_id, run_id),
    FOREIGN KEY (user_id, run_id) REFERENCES run (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, cycle_id) REFERENCES cycle (user_id, id) ON DELETE CASCADE
);

CREATE INDEX ON planning_run (user_id, cycle_id);

-- The Plan Proposal a planning Run stored, with the planner's summary. At most one of a
-- User's is pending.
CREATE TABLE plan_proposal (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid NOT NULL DEFAULT current_user_id(),
    run_id     bigint NOT NULL UNIQUE,
    status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'replaced')),
    summary    text NOT NULL,
    decided_at timestamptz,
    UNIQUE (user_id, id),
    FOREIGN KEY (user_id, run_id) REFERENCES planning_run (user_id, run_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX plan_proposal_one_pending ON plan_proposal (user_id) WHERE status = 'pending';

ALTER TABLE recipe ADD UNIQUE (user_id, id);

-- A Plan Proposal's Prep Sessions, each at a Slot: of a Recipe, its Portions all of one
-- size, or of an Ingredient set out as it is, quantity of it for each Portion.
CREATE TABLE prep_session (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          uuid NOT NULL DEFAULT current_user_id(),
    plan_proposal_id bigint NOT NULL,
    prepared_on      date NOT NULL,
    prepared_kind    text NOT NULL CHECK (prepared_kind IN ('breakfast', 'lunch', 'snack', 'dinner')),
    recipe_id        bigint,
    size             numeric CHECK (size BETWEEN 0.75 AND 1.5),
    ingredient_id    bigint,
    quantity         numeric(8, 1) CHECK (quantity > 0),
    CHECK ((recipe_id IS NULL) = (size IS NULL)),
    CHECK ((ingredient_id IS NULL) = (quantity IS NULL)),
    CHECK ((recipe_id IS NULL) <> (ingredient_id IS NULL)),
    UNIQUE (user_id, id),
    FOREIGN KEY (user_id, plan_proposal_id) REFERENCES plan_proposal (user_id, id) ON DELETE CASCADE,
    -- Deferred, so deleting a User, which cascades in no set order, can finish.
    FOREIGN KEY (user_id, recipe_id) REFERENCES recipe (user_id, id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (user_id, ingredient_id) REFERENCES ingredient (user_id, id) DEFERRABLE INITIALLY DEFERRED
);

-- The Portions a Prep Session yields, in order: where each is kept, and its Slot, if any.
CREATE TABLE planned_portion (
    prep_session_id bigint NOT NULL,
    position        integer NOT NULL,
    user_id         uuid NOT NULL DEFAULT current_user_id(),
    location        text NOT NULL CHECK (location IN ('pantry', 'fridge', 'freezer')),
    eat_on          date,
    eat_kind        text CHECK (eat_kind IN ('breakfast', 'lunch', 'snack', 'dinner')),
    CHECK ((eat_on IS NULL) = (eat_kind IS NULL)),
    PRIMARY KEY (prep_session_id, position),
    FOREIGN KEY (user_id, prep_session_id) REFERENCES prep_session (user_id, id) ON DELETE CASCADE
);

-- The Portions without a Slot a Plan Proposal puts in Slots.
CREATE TABLE kept_portion (
    plan_proposal_id bigint NOT NULL,
    portion_id       bigint NOT NULL,
    user_id          uuid NOT NULL DEFAULT current_user_id(),
    eat_on           date NOT NULL,
    eat_kind         text NOT NULL CHECK (eat_kind IN ('breakfast', 'lunch', 'snack', 'dinner')),
    PRIMARY KEY (plan_proposal_id, portion_id),
    FOREIGN KEY (user_id, plan_proposal_id) REFERENCES plan_proposal (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, portion_id) REFERENCES portion (user_id, id) DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE planning_run ENABLE ROW LEVEL SECURITY;
ALTER TABLE plan_proposal ENABLE ROW LEVEL SECURITY;
ALTER TABLE prep_session ENABLE ROW LEVEL SECURITY;
ALTER TABLE planned_portion ENABLE ROW LEVEL SECURITY;
ALTER TABLE kept_portion ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT ON planning_run, plan_proposal, prep_session, planned_portion, kept_portion TO authenticated;

CREATE POLICY own_rows ON planning_run
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

CREATE POLICY own_rows ON plan_proposal
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

CREATE POLICY own_rows ON prep_session
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

CREATE POLICY own_rows ON planned_portion
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

CREATE POLICY own_rows ON kept_portion
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE kept_portion;
DROP TABLE planned_portion;
DROP TABLE prep_session;
DROP TABLE plan_proposal;
ALTER TABLE recipe DROP CONSTRAINT recipe_user_id_id_key;
DROP TABLE planning_run;
