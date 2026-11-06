-- +goose Up
-- A move is one Movement from one Storage Location to another, so the Kitchen Ledger shows
-- it as a single event on both Stock Items: storage_location is where it comes from,
-- moved_to where it goes, and change, above nothing, how much goes. It keeps no believed or
-- stated: what each Stock Item held before and after is what the Movements add up to.
ALTER TABLE movement
    ADD COLUMN moved_to text CHECK (moved_to IN ('pantry', 'fridge', 'freezer')),
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted', 'moved')),
    ADD CONSTRAINT movement_moved_check CHECK (
        CASE cause_kind
            WHEN 'moved' THEN moved_to IS NOT NULL AND moved_to <> storage_location AND change > 0
                AND believed IS NULL AND stated IS NULL
            ELSE moved_to IS NULL
        END
    );

-- +goose Down
ALTER TABLE movement
    DROP CONSTRAINT movement_moved_check,
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted')),
    DROP COLUMN moved_to;
