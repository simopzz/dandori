-- +goose Up
-- A planner's Recipe Drafts belong to the Plan Proposal that prepares them, as the
-- Recipe it drafts, written as draft_recipe took it. They stay with it when it is
-- rejected or ends; approving it accepts them into the Recipe Library.
CREATE TABLE proposal_recipe_draft (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          uuid NOT NULL DEFAULT current_user_id(),
    plan_proposal_id bigint NOT NULL,
    document         jsonb NOT NULL,
    UNIQUE (user_id, id),
    FOREIGN KEY (user_id, plan_proposal_id) REFERENCES plan_proposal (user_id, id) ON DELETE CASCADE
);

ALTER TABLE proposal_recipe_draft ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, DELETE ON proposal_recipe_draft TO authenticated;

CREATE POLICY own_rows ON proposal_recipe_draft
    USING (user_id = (SELECT current_user_id()))
    WITH CHECK (user_id = (SELECT current_user_id()));

-- A Prep Session prepares a Recipe, a Recipe Draft until its Plan Proposal is approved,
-- or an Ingredient as it is.
ALTER TABLE prep_session
    ADD COLUMN draft_id bigint,
    DROP CONSTRAINT prep_session_check,
    DROP CONSTRAINT prep_session_check2,
    ADD CHECK ((recipe_id IS NULL AND draft_id IS NULL) = (size IS NULL)),
    ADD CHECK (num_nonnulls(recipe_id, draft_id, ingredient_id) = 1),
    -- Deferred, so approving can delete the Recipe Drafts before pointing their Prep
    -- Sessions at the Recipes, and deleting a User, which cascades in no set order, can
    -- finish.
    ADD FOREIGN KEY (user_id, draft_id) REFERENCES proposal_recipe_draft (user_id, id) DEFERRABLE INITIALLY DEFERRED;

GRANT UPDATE (recipe_id, draft_id) ON prep_session TO authenticated;

-- +goose Down
REVOKE UPDATE (recipe_id, draft_id) ON prep_session FROM authenticated;
DELETE FROM prep_session WHERE draft_id IS NOT NULL;
ALTER TABLE prep_session
    DROP COLUMN draft_id,
    ADD CONSTRAINT prep_session_check CHECK ((recipe_id IS NULL) = (size IS NULL)),
    ADD CONSTRAINT prep_session_check2 CHECK ((recipe_id IS NULL) <> (ingredient_id IS NULL));
DROP TABLE proposal_recipe_draft;
