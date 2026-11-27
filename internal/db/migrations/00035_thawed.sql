-- +goose Up
-- Frozen food is thawed the evening before: a move, freezer to fridge, of a Stock Item for
-- the Prep Session in cause_ref, or of a Portion for the Slot it fills, recording that
-- Slot either way.
ALTER TABLE movement
    DROP CONSTRAINT movement_moved_check,
    ADD CONSTRAINT movement_moved_check CHECK (
        CASE
            WHEN cause_kind IN ('moved', 'thawed') THEN moved_to IS NOT NULL AND moved_to <> storage_location AND change > 0
                AND believed IS NULL AND stated IS NULL
            ELSE moved_to IS NULL
        END
    ),
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (
        cause_kind IN ('counted', 'moved', 'staple', 'added', 'delivered', 'used_up', 'prepared', 'eaten', 'thawed')
    ),
    DROP CONSTRAINT movement_assumed_check,
    ADD CONSTRAINT movement_assumed_check CHECK (
        CASE cause_kind
            WHEN 'used_up' THEN ingredient_id IS NOT NULL AND change < 0 AND cause_ref IS NOT NULL AND slot_on IS NOT NULL
            WHEN 'prepared' THEN ingredient_id IS NULL AND change > 0 AND cause_ref IS NOT NULL AND slot_on IS NOT NULL
            WHEN 'eaten' THEN ingredient_id IS NULL AND change = -1 AND cause_ref IS NULL AND slot_on IS NOT NULL
            WHEN 'thawed' THEN storage_location = 'freezer' AND moved_to = 'fridge' AND slot_on IS NOT NULL
                AND (ingredient_id IS NULL) = (cause_ref IS NULL)
            ELSE slot_on IS NULL AND cause_ref IS NULL
        END
    );

-- +goose Down
DELETE FROM movement_portion WHERE movement_id IN (SELECT id FROM movement WHERE cause_kind = 'thawed');
DELETE FROM movement WHERE cause_kind = 'thawed';
ALTER TABLE movement
    DROP CONSTRAINT movement_assumed_check,
    ADD CONSTRAINT movement_assumed_check CHECK (
        CASE cause_kind
            WHEN 'used_up' THEN ingredient_id IS NOT NULL AND change < 0 AND cause_ref IS NOT NULL AND slot_on IS NOT NULL
            WHEN 'prepared' THEN ingredient_id IS NULL AND change > 0 AND cause_ref IS NOT NULL AND slot_on IS NOT NULL
            WHEN 'eaten' THEN ingredient_id IS NULL AND change = -1 AND cause_ref IS NULL AND slot_on IS NOT NULL
            ELSE slot_on IS NULL AND cause_ref IS NULL
        END
    ),
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (
        cause_kind IN ('counted', 'moved', 'staple', 'added', 'delivered', 'used_up', 'prepared', 'eaten')
    ),
    DROP CONSTRAINT movement_moved_check,
    ADD CONSTRAINT movement_moved_check CHECK (
        CASE cause_kind
            WHEN 'moved' THEN moved_to IS NOT NULL AND moved_to <> storage_location AND change > 0
                AND believed IS NULL AND stated IS NULL
            ELSE moved_to IS NULL
        END
    );
