-- +goose Up
-- A Catalog Product: one purchasable Esselunga product in the Known Catalog,
-- under Esselunga's own name. A pack holds pack_quantity of unit; its
-- nutritional values are per 100 g, per 100 ml or per piece accordingly,
-- and all null when the store shows no nutrition label.
CREATE TABLE catalog_product (
    id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Esselunga's product code; a seed run again updates the product.
    code                 text        NOT NULL UNIQUE,
    name                 text        NOT NULL,
    pack_quantity        numeric     NOT NULL CHECK (pack_quantity > 0),
    unit                 text        NOT NULL CHECK (unit IN ('g', 'ml', 'piece')),
    kcal                 numeric     CHECK (kcal >= 0),
    protein_g            numeric     CHECK (protein_g >= 0),
    carbs_g              numeric     CHECK (carbs_g >= 0),
    fat_g                numeric     CHECK (fat_g >= 0),
    aisle                text        NOT NULL DEFAULT '',
    -- The cached price of one pack, in euros.
    price                numeric     NOT NULL CHECK (price >= 0),
    available            boolean     NOT NULL,
    availability_seen_at timestamptz NOT NULL,
    CHECK ((kcal IS NULL) = (protein_g IS NULL) AND (kcal IS NULL) = (carbs_g IS NULL) AND (kcal IS NULL) = (fat_g IS NULL))
);

-- A Product Mapping: the Catalog Product preferred for buying an Ingredient.
-- One the agent proposed stays proposed until the user saves it.
CREATE TABLE product_mapping (
    ingredient_id      bigint  PRIMARY KEY REFERENCES ingredient ON DELETE CASCADE,
    catalog_product_id bigint  NOT NULL REFERENCES catalog_product ON DELETE CASCADE,
    proposed           boolean NOT NULL
);

-- +goose Down
DROP TABLE product_mapping;
DROP TABLE catalog_product;
