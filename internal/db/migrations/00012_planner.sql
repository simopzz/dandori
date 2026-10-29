-- +goose Up
-- A planning Run drafts any number of Recipes, so a Recipe Draft has an id
-- of its own; a parse Run still drafts one. Accepting a draft keeps it,
-- with the Recipe it became, so a Plan Proposal using it can read that
-- Recipe.
ALTER TABLE recipe_draft DROP CONSTRAINT recipe_draft_pkey,
    ADD COLUMN id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ADD COLUMN recipe_id bigint REFERENCES recipe ON DELETE CASCADE;
CREATE INDEX ON recipe_draft (run_id);
-- What used to be accepted was deleted, so every draft left is pending.

-- The planning Runs of a Cycle. after_proposal is the Plan Proposal whose
-- rejection with a note started it, if one did.
CREATE TABLE planning_run (
    run_id         bigint PRIMARY KEY REFERENCES run ON DELETE CASCADE,
    cycle_id       bigint NOT NULL REFERENCES cycle ON DELETE CASCADE,
    after_proposal bigint REFERENCES plan_proposal ON DELETE SET NULL
);

-- The planning Run that submitted a Plan Proposal, so its Trace can be
-- read from it.
ALTER TABLE plan_proposal ADD COLUMN run_id bigint REFERENCES run ON DELETE SET NULL;

-- +goose Down
ALTER TABLE plan_proposal DROP COLUMN run_id;
-- A Run drafted at most one pending Recipe before.
DELETE FROM recipe_draft WHERE recipe_id IS NOT NULL OR run_id IN (SELECT run_id FROM planning_run);
DROP TABLE planning_run;
DROP INDEX recipe_draft_run_id_idx;
ALTER TABLE recipe_draft DROP COLUMN recipe_id, DROP COLUMN id, ADD PRIMARY KEY (run_id);
