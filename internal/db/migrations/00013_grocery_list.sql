-- +goose Up
-- A Cycle's Grocery List, stored once it is fixed: when its Delivery is
-- marked ordered, or arrives still tentative. Until then it is computed
-- from the Plan. A line keeps its Catalog Product as it was when stored,
-- all null when the Ingredient had no Product Mapping.
CREATE TABLE grocery_line (
    cycle_id      bigint  NOT NULL REFERENCES cycle ON DELETE CASCADE,
    ingredient_id bigint  NOT NULL REFERENCES ingredient,
    -- What the Plan needs beyond stock, in the Ingredient's unit.
    needed        numeric NOT NULL CHECK (needed > 0),
    product_code  text,
    product_name  text,
    pack_quantity numeric CHECK (pack_quantity > 0),
    aisle         text,
    proposed      boolean,
    packs         integer CHECK (packs > 0),
    PRIMARY KEY (cycle_id, ingredient_id),
    CHECK ((product_code IS NULL) = (product_name IS NULL) AND (product_code IS NULL) = (pack_quantity IS NULL)
       AND (product_code IS NULL) = (aisle IS NULL) AND (product_code IS NULL) = (proposed IS NULL)
       AND (product_code IS NULL) = (packs IS NULL))
);

-- Whether the Cycle's Grocery List is fixed, its lines in grocery_line.
ALTER TABLE cycle ADD COLUMN grocery_list_fixed boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE cycle DROP COLUMN grocery_list_fixed;
DROP TABLE grocery_line;
