-- +goose Up
-- A change of the times of day takes effect from the next day, times_from, so that a Slot
-- of the day it is saved on keeps its time. Until then the times in force are the was_
-- ones: null as the times are while the defaults (domain.DefaultSlotTimes) apply.
-- times_from is null when there's no change to wait for.
ALTER TABLE profile
    ADD COLUMN was_breakfast_at time,
    ADD COLUMN was_lunch_at     time,
    ADD COLUMN was_snack_at     time,
    ADD COLUMN was_dinner_at    time,
    ADD COLUMN times_from       date,
    ADD CHECK (num_nulls(was_breakfast_at, was_lunch_at, was_snack_at, was_dinner_at) IN (0, 4)),
    ADD CHECK (was_breakfast_at < was_lunch_at AND was_lunch_at < was_snack_at AND was_snack_at < was_dinner_at);

-- +goose Down
ALTER TABLE profile
    DROP COLUMN times_from, DROP COLUMN was_dinner_at, DROP COLUMN was_snack_at,
    DROP COLUMN was_lunch_at, DROP COLUMN was_breakfast_at;
