-- +goose Up
-- Stock Items have no expiry date: the user watches for spoiled food and
-- corrects the Stock Item. Dates already entered are discarded. See ADR
-- 0011.
ALTER TABLE stock_item DROP COLUMN expires_on;
ALTER TABLE stock_taken DROP COLUMN expires_on;

-- +goose Down
ALTER TABLE stock_taken ADD COLUMN expires_on date;
ALTER TABLE stock_item ADD COLUMN expires_on date;
