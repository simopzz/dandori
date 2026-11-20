-- +goose Up
-- What cancelled a Run when the User didn't, as a document its kind's page tells: the
-- Delivery that closed a planning Run's Cycle, or the Cycle dropped. Null when the User
-- cancelled it, and while it isn't cancelled.
ALTER TABLE run ADD COLUMN cause jsonb;

-- What ended a Plan Proposal when the User didn't decide on it: the Delivery that closed
-- its Cycle. Null for one pending or decided by the User.
ALTER TABLE plan_proposal ADD COLUMN ended_by jsonb;
GRANT UPDATE (ended_by) ON plan_proposal TO authenticated;

-- +goose Down
REVOKE UPDATE (ended_by) ON plan_proposal FROM authenticated;
ALTER TABLE plan_proposal DROP COLUMN ended_by;
ALTER TABLE run DROP COLUMN cause;
