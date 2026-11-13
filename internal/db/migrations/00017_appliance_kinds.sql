-- +goose Up
-- A Recipe's appliances are kinds from a fixed list (ADR-0020, domain.ApplianceKinds).
ALTER TABLE recipe ADD CONSTRAINT recipe_appliances_check CHECK (appliances <@ ARRAY[
    'oven', 'hob', 'microwave', 'air_fryer', 'pressure_cooker', 'slow_cooker',
    'blender', 'food_processor', 'stand_mixer', 'grill', 'toaster', 'steamer'
]);

-- +goose Down
ALTER TABLE recipe DROP CONSTRAINT recipe_appliances_check;
