-- +goose Up
-- A planner's Recipe Draft belongs to the Plan Proposal that prepares it, not to a parse
-- Run, so it is never listed with the User's own, which join their Run. It stays with
-- its Plan Proposal when that is rejected or ends; approving it accepts the Recipe Draft.
ALTER TABLE recipe_draft
    ALTER COLUMN run_id DROP NOT NULL,
    ADD COLUMN plan_proposal_id bigint,
    ADD CHECK ((run_id IS NULL) <> (plan_proposal_id IS NULL)),
    ADD UNIQUE (user_id, id),
    ADD FOREIGN KEY (user_id, plan_proposal_id) REFERENCES plan_proposal (user_id, id) ON DELETE CASCADE;

-- A Prep Session prepares a Recipe, a Recipe Draft until its Plan Proposal is approved,
-- or an Ingredient as it is.
ALTER TABLE prep_session
    ADD COLUMN recipe_draft_id bigint,
    DROP CONSTRAINT prep_session_check,
    DROP CONSTRAINT prep_session_check2,
    ADD CHECK ((recipe_id IS NULL AND recipe_draft_id IS NULL) = (size IS NULL)),
    ADD CHECK (num_nonnulls(recipe_id, recipe_draft_id, ingredient_id) = 1),
    -- Deferred, so deleting a User, which cascades in no set order, can finish.
    ADD FOREIGN KEY (user_id, recipe_draft_id) REFERENCES recipe_draft (user_id, id) DEFERRABLE INITIALLY DEFERRED;

GRANT UPDATE (recipe_id, recipe_draft_id) ON prep_session TO authenticated;

-- +goose Down
REVOKE UPDATE (recipe_id, recipe_draft_id) ON prep_session FROM authenticated;
DELETE FROM prep_session WHERE recipe_draft_id IS NOT NULL;
ALTER TABLE prep_session
    DROP COLUMN recipe_draft_id,
    ADD CONSTRAINT prep_session_check CHECK ((recipe_id IS NULL) = (size IS NULL)),
    ADD CONSTRAINT prep_session_check2 CHECK ((recipe_id IS NULL) <> (ingredient_id IS NULL));
DELETE FROM recipe_draft WHERE plan_proposal_id IS NOT NULL;
ALTER TABLE recipe_draft
    DROP COLUMN plan_proposal_id,
    DROP CONSTRAINT recipe_draft_user_id_id_key,
    ALTER COLUMN run_id SET NOT NULL;
