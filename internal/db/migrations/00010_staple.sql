-- +goose Up
-- Marking an Ingredient a Staple counts each of its Stock Items to nothing: a Count of
-- its own cause, which keeps what Dandori believed, states nothing and has no note.
ALTER TABLE movement
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted', 'moved', 'staple')),
    ADD CONSTRAINT movement_staple_check CHECK (
        cause_kind <> 'staple'
        OR (believed > 0 AND stated = 0 AND change = -believed AND note IS NULL)
    );

-- +goose Down
-- The narrower CHECK can't hold while Staples' Counts to nothing remain, so they go: the
-- Kitchen Ledger forgets them, and each Staple's stock adds up to what it was before.
DELETE FROM movement WHERE cause_kind = 'staple';
ALTER TABLE movement
    DROP CONSTRAINT movement_staple_check,
    DROP CONSTRAINT movement_cause_kind_check,
    ADD CONSTRAINT movement_cause_kind_check CHECK (cause_kind IN ('counted', 'moved'));
