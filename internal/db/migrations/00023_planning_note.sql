-- +goose Up
-- What the User tells the planner about one Cycle, newest last: given when rejecting a
-- Plan Proposal (the one it rejected) or asking for changes (none).
CREATE TABLE planning_note (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          uuid NOT NULL DEFAULT current_user_id(),
    cycle_id         bigint NOT NULL,
    note             text NOT NULL CHECK (note <> ''),
    given_at         timestamptz NOT NULL,
    plan_proposal_id bigint,
    FOREIGN KEY (user_id, cycle_id) REFERENCES cycle (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, plan_proposal_id) REFERENCES plan_proposal (user_id, id) ON DELETE CASCADE
);

CREATE INDEX ON planning_note (user_id, cycle_id);

ALTER TABLE planning_note ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT ON planning_note TO authenticated;

-- The User decides on a pending Plan Proposal: only its status and when change.
GRANT UPDATE (status, decided_at) ON plan_proposal TO authenticated;

CREATE POLICY own_rows ON planning_note
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
REVOKE UPDATE (status, decided_at) ON plan_proposal FROM authenticated;
DROP TABLE planning_note;
