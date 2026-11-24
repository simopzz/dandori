-- +goose Up
-- When the User marked a Cycle's Delivery ordered, having checked out: null while it is
-- tentative.
ALTER TABLE cycle ADD COLUMN ordered_at timestamptz;
GRANT UPDATE (ordered_at) ON cycle TO authenticated;

-- A Cycle's fixed Grocery List, a line per Ingredient: written when its Delivery is
-- marked ordered, and deleted by undoing that.
CREATE TABLE grocery_line (
    cycle_id      bigint NOT NULL,
    user_id       uuid NOT NULL DEFAULT current_user_id(),
    ingredient_id bigint NOT NULL,
    quantity      numeric(8, 1) NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (cycle_id, ingredient_id),
    FOREIGN KEY (user_id, cycle_id) REFERENCES cycle (user_id, id) ON DELETE CASCADE,
    -- Deferred, so deleting a User, which cascades in no set order, can finish.
    FOREIGN KEY (user_id, ingredient_id) REFERENCES ingredient (user_id, id) DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE grocery_line ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, DELETE ON grocery_line TO authenticated;

CREATE POLICY own_rows ON grocery_line
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE grocery_line;
REVOKE UPDATE (ordered_at) ON cycle FROM authenticated;
ALTER TABLE cycle DROP COLUMN ordered_at;
