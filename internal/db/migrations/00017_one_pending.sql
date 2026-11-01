-- +goose Up
-- At most one Plan Proposal is pending, across every Cycle. Before that
-- was the rule, several could be: the oldest stays pending, as planning
-- already took it to be the one.
UPDATE plan_proposal SET status = 'replaced', cause = 'Replaced: only one Plan Proposal is pending at a time', decided_at = now()
WHERE status = 'pending' AND id > (SELECT min(id) FROM plan_proposal WHERE status = 'pending');
CREATE UNIQUE INDEX plan_proposal_one_pending ON plan_proposal ((true)) WHERE status = 'pending';

-- +goose Down
DROP INDEX plan_proposal_one_pending;
