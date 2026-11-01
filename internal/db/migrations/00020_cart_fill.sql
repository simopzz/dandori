-- +goose Up
-- The Cart Fill Runs of a Cycle: each adds its Grocery List to the
-- Esselunga cart while its Delivery is tentative.
CREATE TABLE cart_fill_run (
    run_id   bigint PRIMARY KEY REFERENCES run ON DELETE CASCADE,
    cycle_id bigint NOT NULL REFERENCES cycle ON DELETE CASCADE
);

-- What Esselunga was out of when Cart Fill started a planning Run to
-- replan around it; empty for any other planning Run. Its Plan Proposal is
-- a substitution.
ALTER TABLE planning_run ADD COLUMN missing text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE planning_run DROP COLUMN missing;
DROP TABLE cart_fill_run;
