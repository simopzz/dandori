-- +goose Up
-- A Slot logged ahead as a Meal Out or skipped. It belongs to its day and
-- Slot kind, not to a Cycle, so it outlives a moved Delivery or a dropped
-- Cycle. A Meal Out's estimate is its own nutrient by nutrient; a null one
-- takes the Profile's default when counted. A skipped Slot has none.
CREATE TABLE slot_exception (
    day       date    NOT NULL,
    kind      text    NOT NULL CHECK (kind IN ('breakfast', 'lunch', 'snack', 'dinner')),
    exception text    NOT NULL CHECK (exception IN ('meal-out', 'skipped')),
    kcal      integer CHECK (kcal >= 0),
    protein_g integer CHECK (protein_g >= 0),
    carbs_g   integer CHECK (carbs_g >= 0),
    fat_g     integer CHECK (fat_g >= 0),
    PRIMARY KEY (day, kind),
    CHECK (exception = 'meal-out' OR num_nonnulls(kcal, protein_g, carbs_g, fat_g) = 0)
);

-- Why a Cycle's Plan needs a new Plan Proposal, said for the user (a
-- Delivery moved, an Exception changed); empty when it doesn't.
ALTER TABLE cycle ADD COLUMN needs_proposal_because text NOT NULL DEFAULT '';
UPDATE cycle SET needs_proposal_because = 'A Delivery moved' WHERE needs_proposal;
ALTER TABLE cycle DROP COLUMN needs_proposal;

-- +goose Down
ALTER TABLE cycle ADD COLUMN needs_proposal boolean NOT NULL DEFAULT false;
UPDATE cycle SET needs_proposal = needs_proposal_because <> '';
ALTER TABLE cycle DROP COLUMN needs_proposal_because;
DROP TABLE slot_exception;
