-- +goose Up
-- The Plan happens at its Slots' times: a Prep Session uses up Stock Items and prepares
-- its Portions, and the Portion filling a Slot is eaten. Each such assumed Movement
-- records the Slot it is assumed at, which its time may be later than, and a Prep
-- Session's records the Prep Session in cause_ref. A Portion eaten lists it in
-- movement_portion.
ALTER TABLE movement
    ADD COLUMN slot_on date,
    ADD COLUMN slot_kind text CHECK (slot_kind IN ('breakfast', 'lunch', 'snack', 'dinner')),
    ADD CONSTRAINT movement_slot_check CHECK ((slot_on IS NULL) = (slot_kind IS NULL)),
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (
        cause_kind IN ('counted', 'moved', 'staple', 'added', 'delivered', 'used_up', 'prepared', 'eaten')
    ),
    ADD CONSTRAINT movement_assumed_check CHECK (
        CASE cause_kind
            WHEN 'used_up' THEN ingredient_id IS NOT NULL AND change < 0 AND cause_ref IS NOT NULL AND slot_on IS NOT NULL
            WHEN 'prepared' THEN ingredient_id IS NULL AND change > 0 AND cause_ref IS NOT NULL AND slot_on IS NOT NULL
            WHEN 'eaten' THEN ingredient_id IS NULL AND change = -1 AND cause_ref IS NULL AND slot_on IS NOT NULL
            ELSE slot_on IS NULL AND cause_ref IS NULL
        END
    );

-- A Portion a Prep Session prepared records the Planned Portion it came from, so a Slot
-- that Planned Portion fills, in whichever Plan Proposal, is filled by it.
ALTER TABLE portion
    ADD COLUMN planned_portion_id bigint,
    ADD UNIQUE (user_id, planned_portion_id),
    -- Deferred, so deleting a User, which cascades in no set order, can finish.
    ADD FOREIGN KEY (user_id, planned_portion_id) REFERENCES planned_portion (user_id, id) DEFERRABLE INITIALLY DEFERRED;

-- +goose Down
DELETE FROM movement_portion WHERE movement_id IN (SELECT id FROM movement WHERE cause_kind IN ('used_up', 'prepared', 'eaten'));
DELETE FROM movement WHERE cause_kind IN ('used_up', 'prepared', 'eaten');
ALTER TABLE portion DROP COLUMN planned_portion_id;
ALTER TABLE movement
    DROP CONSTRAINT movement_assumed_check,
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted', 'moved', 'staple', 'added', 'delivered')),
    DROP CONSTRAINT movement_slot_check,
    DROP COLUMN slot_kind,
    DROP COLUMN slot_on;
