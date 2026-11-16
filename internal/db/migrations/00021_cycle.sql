-- +goose Up
-- A User's Cycles, each with its Delivery's date and time window on that day, and its end
-- date. Whether one is upcoming, current or closed follows from the clock and the
-- Deliveries after it (domain.CycleStatuses), so it isn't stored; a dropped Cycle is
-- deleted.
CREATE TABLE cycle (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     uuid NOT NULL DEFAULT current_user_id() REFERENCES "user" (id) ON DELETE CASCADE,
    delivery_on date NOT NULL,
    window_from time NOT NULL,
    window_to   time NOT NULL CHECK (window_to > window_from),
    ends_on     date NOT NULL CHECK (ends_on > delivery_on AND ends_on <= delivery_on + 14),
    UNIQUE (user_id, id)
);

CREATE INDEX ON cycle (user_id, delivery_on);

ALTER TABLE cycle ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, DELETE ON cycle TO authenticated;

CREATE POLICY own_rows ON cycle
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE cycle;
