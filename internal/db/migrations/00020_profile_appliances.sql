-- +goose Up
-- The kinds of appliance the User owns (ADR-0020, domain.ApplianceKinds), each the key of
-- their note on its model, "" without one. Every Profile starts owning a hob and an oven.
ALTER TABLE profile ADD COLUMN appliances jsonb NOT NULL DEFAULT '{"hob": "", "oven": ""}'
    CHECK (jsonb_typeof(appliances) = 'object' AND appliances - ARRAY[
        'oven', 'hob', 'microwave', 'air_fryer', 'pressure_cooker', 'slow_cooker',
        'blender', 'food_processor', 'stand_mixer', 'grill', 'toaster', 'steamer'
    ] = '{}');

-- +goose Down
ALTER TABLE profile DROP COLUMN appliances;
