-- +goose Up
-- A Plan Proposal is replaced by something that happened, rather than
-- rejected by the user. cause is what replaced it, or that its Cycle
-- closed; never a Planning Note, which note alone holds.
ALTER TABLE plan_proposal DROP CONSTRAINT plan_proposal_status_check,
    ADD CONSTRAINT plan_proposal_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'replaced')),
    ADD COLUMN cause text NOT NULL DEFAULT '';
-- Why Dandori rejected them used to be written as their note.
UPDATE plan_proposal SET cause = note, note = ''
WHERE status = 'rejected' AND note LIKE 'Its Cycle closed: %';
UPDATE plan_proposal SET status = 'replaced', cause = note, note = ''
WHERE status = 'rejected' AND (note LIKE 'An Exception changed: %' OR note LIKE 'Its Delivery moved to %');

-- Why a Run was cancelled, when it wasn't the user: whatever replaced the
-- Plan Proposal a planning Run was making, or that its Cycle was dropped.
ALTER TABLE run ADD COLUMN cancelled_because text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE run DROP COLUMN cancelled_because;
UPDATE plan_proposal SET status = 'rejected', note = cause WHERE status = 'replaced';
UPDATE plan_proposal SET note = cause WHERE cause <> '' AND note = '';
ALTER TABLE plan_proposal DROP COLUMN cause,
    DROP CONSTRAINT plan_proposal_status_check,
    ADD CONSTRAINT plan_proposal_status_check CHECK (status IN ('pending', 'approved', 'rejected'));
