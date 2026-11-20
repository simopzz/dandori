-- +goose Up
-- The days a Plan Proposal was judged with, beside its misses: the seven days before its
-- first Slot and the days it plans, as domain.Day. Its pending page shows them as judged,
-- whatever its Recipes become. Not null with no default: a Plan Proposal stored before
-- has no Verdict to fill them, so a database holding any must be cleared first.
ALTER TABLE plan_proposal ADD COLUMN days_before jsonb NOT NULL, ADD COLUMN days jsonb NOT NULL;

-- +goose Down
ALTER TABLE plan_proposal DROP COLUMN days, DROP COLUMN days_before;
