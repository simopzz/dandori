-- +goose Up
-- The single Planning Brief of the app: the id column only ever holds true.
-- It is kept apart from the profile row so it can be saved before the
-- Profile is.
CREATE TABLE planning_brief (
    id         boolean PRIMARY KEY DEFAULT true CHECK (id),
    brief      text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE planning_brief;
