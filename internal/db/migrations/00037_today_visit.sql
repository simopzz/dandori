-- +goose Up
-- When each User last opened Today, which lists what Dandori assumed since the visit
-- before, and at least all it assumed that day.
CREATE TABLE today_visit (
    user_id    uuid PRIMARY KEY DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    visited_at timestamptz NOT NULL
);

ALTER TABLE today_visit ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE ON today_visit TO authenticated;

CREATE POLICY own_rows ON today_visit
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE today_visit;
