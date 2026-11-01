-- +goose Up
-- What the user tells the planner about a Cycle, when rejecting a Plan
-- Proposal or asking for changes to its Plan. Every planning Run of the
-- Cycle is told them all, newest first: in the order they were given.
CREATE TABLE planning_note (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cycle_id bigint NOT NULL REFERENCES cycle ON DELETE CASCADE,
    note     text   NOT NULL CHECK (note <> '')
);
INSERT INTO planning_note (cycle_id, note)
SELECT cycle_id, note FROM plan_proposal
WHERE status = 'rejected' AND note <> ''
ORDER BY decided_at, id;

-- A Plan Proposal for a Cycle that already has a Plan, asked for by its
-- Planning Notes.
ALTER TABLE plan_proposal DROP CONSTRAINT plan_proposal_origin_check,
    ADD CONSTRAINT plan_proposal_origin_check CHECK (origin IN ('initial', 'substitution', 'changes'));

-- +goose Down
UPDATE plan_proposal SET origin = 'initial' WHERE origin = 'changes';
ALTER TABLE plan_proposal DROP CONSTRAINT plan_proposal_origin_check,
    ADD CONSTRAINT plan_proposal_origin_check CHECK (origin IN ('initial', 'substitution'));
DROP TABLE planning_note;
