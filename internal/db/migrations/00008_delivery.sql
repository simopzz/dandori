-- +goose Up
-- A Cycle's Delivery has a time window, and is tentative until ordered. The
-- Cycle starts at its first Slot, starts_on and starts_with: the first at
-- or after the end of the window. It is upcoming until the Delivery
-- arrives, current until the next one does, and closed after that. Closing
-- early, closed_on is the day of the next Cycle's first Slot and
-- closed_with its kind.
--
-- Cycles from before had arrived, and started at breakfast on their
-- delivery date: a window just before breakfast keeps them as they were.
ALTER TABLE cycle
    ADD COLUMN window_from    time    NOT NULL DEFAULT '07:00',
    ADD COLUMN window_to      time    NOT NULL DEFAULT '07:30',
    ADD COLUMN ordered        boolean NOT NULL DEFAULT true,
    ADD COLUMN starts_on      date,
    ADD COLUMN starts_with    text CHECK (starts_with IN ('breakfast', 'lunch', 'snack', 'dinner')),
    ADD COLUMN closed_with    text CHECK (closed_with IN ('breakfast', 'lunch', 'snack', 'dinner')),
    ADD COLUMN status         text    NOT NULL DEFAULT 'closed' CHECK (status IN ('upcoming', 'current', 'closed')),
    -- Its Plan needs a new Plan Proposal: a Delivery moved under it.
    ADD COLUMN needs_proposal boolean NOT NULL DEFAULT false,
    ADD CHECK (window_to > window_from);
UPDATE cycle SET starts_on = delivery_on, starts_with = 'breakfast';
UPDATE cycle SET closed_with = 'breakfast' WHERE closed_on IS NOT NULL;
UPDATE cycle SET status = 'current' WHERE delivery_on = (SELECT max(delivery_on) FROM cycle);
ALTER TABLE cycle
    ALTER COLUMN window_from DROP DEFAULT,
    ALTER COLUMN window_to DROP DEFAULT,
    ALTER COLUMN ordered SET DEFAULT false,
    ALTER COLUMN starts_on SET NOT NULL,
    ALTER COLUMN starts_with SET NOT NULL,
    ALTER COLUMN status SET DEFAULT 'upcoming',
    -- Only once closed_with is filled in: Cycles closed early would break it.
    ADD CHECK ((closed_on IS NULL) = (closed_with IS NULL));
-- At most one Cycle is upcoming, and one current.
CREATE UNIQUE INDEX cycle_one_upcoming ON cycle (status) WHERE status IN ('upcoming', 'current');

-- The time of day of each kind of Slot.
ALTER TABLE profile
    ADD COLUMN breakfast_at time NOT NULL DEFAULT '08:00',
    ADD COLUMN lunch_at     time NOT NULL DEFAULT '13:00',
    ADD COLUMN snack_at     time NOT NULL DEFAULT '17:00',
    ADD COLUMN dinner_at    time NOT NULL DEFAULT '20:00',
    ADD CHECK (breakfast_at < lunch_at AND lunch_at < snack_at AND snack_at < dinner_at);

-- +goose Down
ALTER TABLE profile DROP COLUMN dinner_at, DROP COLUMN snack_at, DROP COLUMN lunch_at, DROP COLUMN breakfast_at;
DROP INDEX cycle_one_upcoming;
-- Upcoming Cycles didn't exist before.
DELETE FROM plan_proposal WHERE cycle_id IN (SELECT id FROM cycle WHERE status = 'upcoming');
DELETE FROM prep_session WHERE cycle_id IN (SELECT id FROM cycle WHERE status = 'upcoming');
DELETE FROM cycle WHERE status = 'upcoming';
ALTER TABLE cycle
    DROP COLUMN needs_proposal, DROP COLUMN status, DROP COLUMN closed_with,
    DROP COLUMN starts_with, DROP COLUMN starts_on, DROP COLUMN ordered,
    DROP COLUMN window_to, DROP COLUMN window_from;
