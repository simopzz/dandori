-- +goose Up
-- A User's Profile, the first table a User owns. It is created when they
-- choose their Language at their first sign-in, so a User without one has
-- yet to choose (ADR-0015).
CREATE TABLE profile (
    user_id  uuid PRIMARY KEY DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    language text NOT NULL CHECK (language IN ('en', 'it'))
);

ALTER TABLE profile ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE ON profile TO authenticated;

CREATE POLICY own_rows ON profile
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE profile;
