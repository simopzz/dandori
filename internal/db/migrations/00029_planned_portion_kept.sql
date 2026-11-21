-- +goose Up
-- A Planned Portion's own id: a later Plan Proposal of its Cycle puts it in a Slot again
-- by it, while the Kitchen Ledger doesn't hold the Portion yet.
ALTER TABLE planned_portion ADD COLUMN id bigint GENERATED ALWAYS AS IDENTITY;
ALTER TABLE planned_portion ADD UNIQUE (user_id, id);

-- A Plan Proposal puts in a Slot either a Portion the Kitchen Ledger holds without one,
-- or a Planned Portion an earlier Prep Session of its Cycle prepared.
ALTER TABLE kept_portion DROP CONSTRAINT kept_portion_pkey;
ALTER TABLE kept_portion ALTER COLUMN portion_id DROP NOT NULL;
ALTER TABLE kept_portion ADD COLUMN planned_portion_id bigint;
ALTER TABLE kept_portion ADD FOREIGN KEY (user_id, planned_portion_id) REFERENCES planned_portion (user_id, id) ON DELETE CASCADE;
ALTER TABLE kept_portion ADD CHECK ((portion_id IS NULL) <> (planned_portion_id IS NULL));
ALTER TABLE kept_portion ADD UNIQUE (plan_proposal_id, portion_id);
ALTER TABLE kept_portion ADD UNIQUE (plan_proposal_id, planned_portion_id);

-- +goose Down
DELETE FROM kept_portion WHERE planned_portion_id IS NOT NULL;
ALTER TABLE kept_portion DROP COLUMN planned_portion_id;
ALTER TABLE kept_portion DROP CONSTRAINT kept_portion_plan_proposal_id_portion_id_key;
ALTER TABLE kept_portion ALTER COLUMN portion_id SET NOT NULL;
ALTER TABLE kept_portion ADD PRIMARY KEY (plan_proposal_id, portion_id);
ALTER TABLE planned_portion DROP COLUMN id;
