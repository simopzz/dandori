-- +goose Up
-- A Plan Proposal's freezer splits, in order: how much of an Ingredient goes in the freezer
-- when the Delivery is put away.
CREATE TABLE freezer_split (
    plan_proposal_id bigint NOT NULL,
    position         integer NOT NULL,
    user_id          uuid NOT NULL DEFAULT current_user_id(),
    ingredient_id    bigint NOT NULL,
    quantity         numeric(8, 1) NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (plan_proposal_id, position),
    FOREIGN KEY (user_id, plan_proposal_id) REFERENCES plan_proposal (user_id, id) ON DELETE CASCADE,
    -- Deferred, so deleting a User, which cascades in no set order, can finish.
    FOREIGN KEY (user_id, ingredient_id) REFERENCES ingredient (user_id, id) DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE freezer_split ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT ON freezer_split TO authenticated;

CREATE POLICY own_rows ON freezer_split
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- +goose Down
DROP TABLE freezer_split;
