-- +goose Up
-- The Profile's time of day of each kind of Slot, the same every day, in the order of a
-- day. A User's Profile starts with these until they change them.
ALTER TABLE profile
    ADD COLUMN breakfast_at time NOT NULL DEFAULT '08:00',
    ADD COLUMN lunch_at     time NOT NULL DEFAULT '13:00',
    ADD COLUMN snack_at     time NOT NULL DEFAULT '17:00',
    ADD COLUMN dinner_at    time NOT NULL DEFAULT '20:00',
    ADD CHECK (breakfast_at < lunch_at AND lunch_at < snack_at AND snack_at < dinner_at);

-- +goose Down
ALTER TABLE profile DROP COLUMN dinner_at, DROP COLUMN snack_at, DROP COLUMN lunch_at, DROP COLUMN breakfast_at;
