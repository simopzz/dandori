-- +goose Up
-- How far each User's kitchen has been brought up to now (ADR-0013): up_to is the moment
-- the step last brought it to, on a request or a beat, and beaten_at the scheduler's last
-- beat for them, which Home shows. Neither moves back, so the step run again for a moment
-- it has reached changes nothing.
CREATE TABLE kitchen (
    user_id   uuid PRIMARY KEY DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    up_to     timestamptz NOT NULL,
    beaten_at timestamptz
);

ALTER TABLE kitchen ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE ON kitchen TO authenticated;

CREATE POLICY own_rows ON kitchen
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE kitchen;
