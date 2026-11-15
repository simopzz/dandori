-- +goose Up
-- The Profile's exclusions and allergies, the foods the User never eats and those they
-- are allergic to, each normalised and in their Language; and the Planning Brief, their
-- lasting wishes for how their meals are planned. All start empty.
ALTER TABLE profile
    ADD COLUMN exclusions     text[] NOT NULL DEFAULT '{}',
    ADD COLUMN allergies      text[] NOT NULL DEFAULT '{}',
    ADD COLUMN planning_brief text   NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE profile DROP COLUMN planning_brief, DROP COLUMN allergies, DROP COLUMN exclusions;
