-- +goose Up
-- The Targets a Plan Proposal aimed at, so changing them later never changes what it
-- shows, and the soft rules it breaks (ADR-0018), as domain.Targets and domain.Miss.
ALTER TABLE plan_proposal ADD COLUMN targets jsonb, ADD COLUMN misses jsonb NOT NULL DEFAULT '[]';

-- A Plan Proposal made before aimed at its User's Targets as they are now: planning needs
-- them set.
UPDATE plan_proposal pp SET targets = jsonb_build_object(
    'Kcal', p.target_kcal, 'Protein', p.target_protein, 'Fibre', p.target_fibre,
    'Sugar', p.target_sugar, 'Tolerance', p.kcal_tolerance)
FROM profile p WHERE p.user_id = pp.user_id;

ALTER TABLE plan_proposal ALTER COLUMN targets SET NOT NULL;

-- +goose Down
ALTER TABLE plan_proposal DROP COLUMN misses, DROP COLUMN targets;
