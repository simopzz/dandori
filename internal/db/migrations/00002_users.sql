-- +goose Up
-- Users share nothing (ADR-0015), and row-level security enforces it: each
-- User's queries run as Supabase's authenticated role, with the User's id
-- as the sub of request.jwt.claims, the setting Supabase's auth.uid()
-- reads too (db.AsUser). Supabase has the role; a plain Postgres, such as
-- the tests', gets it here.
-- +goose StatementBegin
DO $$
BEGIN
    CREATE ROLE authenticated NOLOGIN NOINHERIT;
EXCEPTION WHEN duplicate_object THEN
    NULL;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO authenticated;

-- current_user_id is the signed-in User, or null outside db.AsUser. Every
-- owned table's policy compares its user_id with it.
CREATE FUNCTION current_user_id() RETURNS uuid
LANGUAGE sql STABLE
AS $$ SELECT (nullif(current_setting('request.jwt.claims', true), '')::jsonb ->> 'sub')::uuid $$;

GRANT EXECUTE ON FUNCTION current_user_id() TO authenticated;

-- A User, created at their first sign-in. id is their Supabase Auth id.
CREATE TABLE "user" (
    id         uuid PRIMARY KEY,
    email      text NOT NULL,
    created_at timestamptz NOT NULL
);

ALTER TABLE "user" ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE ON "user" TO authenticated;

-- An owned table follows this pattern: a user_id defaulting to
-- current_user_id(), and a policy letting each User see and write only
-- their own rows. The "user" table owns itself, by its id.
CREATE POLICY own_rows ON "user"
    USING (id = (SELECT current_user_id()))
    WITH CHECK (id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE "user";
DROP FUNCTION current_user_id();
