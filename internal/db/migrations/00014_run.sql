-- +goose Up
-- A User's Runs, each with its Trace. asked is what the Run was asked, such as the
-- pasted recipe text. A Trace Event is a kind and its data, never fixed prose: pages tell
-- each kind in the User's Language, and keep what the model and its tools said verbatim.
CREATE TABLE run (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid NOT NULL DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    kind       text NOT NULL,
    status     text NOT NULL DEFAULT 'going' CHECK (status IN ('going', 'succeeded', 'failed', 'cancelled')),
    asked      text NOT NULL,
    started_at timestamptz NOT NULL,
    ended_at   timestamptz,
    UNIQUE (user_id, id)
);

CREATE TABLE trace_event (
    run_id  bigint NOT NULL,
    seq     integer NOT NULL,
    user_id uuid NOT NULL DEFAULT current_user_id(),
    at      timestamptz NOT NULL,
    kind    text NOT NULL,
    data    jsonb NOT NULL,
    PRIMARY KEY (run_id, seq),
    FOREIGN KEY (user_id, run_id) REFERENCES run (user_id, id) ON DELETE CASCADE
);

-- A User's Recipe Drafts: the parsed Recipe as a document, its Ingredients by name and
-- proposed values, never linked to the Ingredients they may turn out to be.
CREATE TABLE recipe_draft (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id  uuid NOT NULL DEFAULT current_user_id(),
    run_id   bigint NOT NULL UNIQUE,
    document jsonb NOT NULL,
    FOREIGN KEY (user_id, run_id) REFERENCES run (user_id, id) ON DELETE CASCADE
);

ALTER TABLE run ENABLE ROW LEVEL SECURITY;
ALTER TABLE trace_event ENABLE ROW LEVEL SECURITY;
ALTER TABLE recipe_draft ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE ON run, recipe_draft TO authenticated;
GRANT SELECT, INSERT ON trace_event TO authenticated;

CREATE POLICY own_rows ON run
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

CREATE POLICY own_rows ON trace_event
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

CREATE POLICY own_rows ON recipe_draft
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE recipe_draft;
DROP TABLE trace_event;
DROP TABLE run;
