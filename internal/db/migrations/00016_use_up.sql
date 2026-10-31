-- +goose Up
-- A Prep Session uses up its Ingredients' Stock Items once, from its day
-- on. Those whose day had passed already were left to the stock as the
-- user corrected it by hand.
ALTER TABLE prep_session ADD COLUMN used_up boolean NOT NULL DEFAULT false;
UPDATE prep_session SET used_up = true WHERE prepared_on < current_date;

-- What a Prep Session took of each Stock Item, as the Stock Item was, so
-- it can be given back even once the Stock Item is used up and gone.
CREATE TABLE stock_taken (
    prep_session_id bigint  NOT NULL REFERENCES prep_session ON DELETE CASCADE,
    stock_item_id   bigint  NOT NULL,
    ingredient_id   bigint  NOT NULL REFERENCES ingredient,
    quantity        numeric NOT NULL CHECK (quantity > 0),
    location        text    NOT NULL CHECK (location IN ('pantry', 'fridge', 'freezer')),
    expires_on      date
);
CREATE INDEX ON stock_taken (prep_session_id);

-- +goose Down
DROP TABLE stock_taken;
ALTER TABLE prep_session DROP COLUMN used_up;
