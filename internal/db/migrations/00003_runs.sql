-- +goose Up
-- A Run is one background agent job (planning, Recipe parsing, catalog seed
-- or refresh, Cart Fill) and its stored trace.
CREATE TABLE run (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text        NOT NULL,
    status      text        NOT NULL DEFAULT 'running'
                CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled')),
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);

-- The Trace Events of a Run, numbered from 1 in the order they happened.
CREATE TABLE trace_event (
    run_id  bigint      NOT NULL REFERENCES run ON DELETE CASCADE,
    seq     integer     NOT NULL,
    at      timestamptz NOT NULL,
    kind    text        NOT NULL,
    summary text        NOT NULL,
    detail  text        NOT NULL DEFAULT '',
    PRIMARY KEY (run_id, seq)
);

-- +goose Down
DROP TABLE trace_event;
DROP TABLE run;
