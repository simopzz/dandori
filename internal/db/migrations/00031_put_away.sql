-- +goose Up
-- When a Cycle's Delivery was put away, once its window ended: null until it is. One put
-- away with ordered_at null was never marked ordered.
ALTER TABLE cycle ADD COLUMN put_away_at timestamptz;
GRANT UPDATE (put_away_at) ON cycle TO authenticated;

-- What a Delivery put away adds to a Stock Item.
ALTER TABLE movement
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted', 'moved', 'staple', 'added', 'delivered')),
    ADD CONSTRAINT movement_delivered_check CHECK (
        cause_kind <> 'delivered'
        OR (ingredient_id IS NOT NULL AND moved_to IS NULL AND change > 0 AND believed IS NULL AND stated IS NULL)
    );

-- +goose Down
DELETE FROM movement WHERE cause_kind = 'delivered';
ALTER TABLE movement
    DROP CONSTRAINT movement_delivered_check,
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted', 'moved', 'staple', 'added'));
REVOKE UPDATE (put_away_at) ON cycle FROM authenticated;
ALTER TABLE cycle DROP COLUMN put_away_at;
