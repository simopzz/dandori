-- +goose Up
-- The Profile's time of day of each kind of Slot, the same every day, in the order of a
-- day. They are null until the User sets them, all four at once: until then Dandori's
-- defaults apply, which live in the code (domain.DefaultSlotTimes), not here.
ALTER TABLE profile
    ADD COLUMN breakfast_at time,
    ADD COLUMN lunch_at     time,
    ADD COLUMN snack_at     time,
    ADD COLUMN dinner_at    time,
    ADD CHECK (num_nulls(breakfast_at, lunch_at, snack_at, dinner_at) IN (0, 4)),
    ADD CHECK (breakfast_at < lunch_at AND lunch_at < snack_at AND snack_at < dinner_at);

-- +goose Down
ALTER TABLE profile DROP COLUMN dinner_at, DROP COLUMN snack_at, DROP COLUMN lunch_at, DROP COLUMN breakfast_at;
