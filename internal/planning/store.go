package planning

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/runs"
)

// lockOpening locks the signed-in User's Profile until the transaction ends, so opening
// Cycles waits its turn.
func lockOpening(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM profile FOR UPDATE`)
	return err
}

// loadNow reads the signed-in User's current and upcoming Cycles at now, each nil when
// they have none, its dates as days in now's zone. It has no WHERE on purpose: run in
// db.AsUser, row-level security leaves only their rows.
func loadNow(ctx context.Context, q db.Querier, now time.Time) (current, upcoming *Cycle, err error) {
	rows, err := q.Query(ctx, `
		SELECT id, delivery_on, extract(epoch FROM window_from)::int / 60, extract(epoch FROM window_to)::int / 60, ends_on
		FROM cycle ORDER BY delivery_on, window_to`)
	if err != nil {
		return nil, nil, err
	}
	day := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
	}
	cycles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Cycle, error) {
		var c Cycle
		err := row.Scan(&c.ID, &c.Delivery.On, &c.Delivery.From, &c.Delivery.To, &c.Ends)
		c.Delivery.On, c.Ends = day(c.Delivery.On), day(c.Ends)
		return c, err
	})
	if err != nil {
		return nil, nil, err
	}
	plain := make([]domain.Cycle, len(cycles))
	for i, c := range cycles {
		plain[i] = c.Cycle
	}
	for i, status := range domain.CycleStatuses(plain, now) {
		switch status {
		case domain.Current:
			current = &cycles[i]
		case domain.Upcoming:
			upcoming = &cycles[i]
		}
	}
	return current, upcoming, nil
}

// saveCycle adds c to the signed-in User's Cycles, and is its id.
func saveCycle(ctx context.Context, q db.Querier, c domain.Cycle) (id int64, err error) {
	err = q.QueryRow(ctx, `INSERT INTO cycle (delivery_on, window_from, window_to, ends_on) VALUES ($1, $2, $3, $4) RETURNING id`,
		c.Delivery.On.Format(time.DateOnly), c.Delivery.From.String(), c.Delivery.To.String(), c.Ends.Format(time.DateOnly)).Scan(&id)
	return id, err
}

// deleteCycle deletes the signed-in User's Cycle id.
func deleteCycle(ctx context.Context, q db.Querier, id int64) error {
	_, err := q.Exec(ctx, `DELETE FROM cycle WHERE id = $1`, id)
	return err
}

// planningBusy reports whether the signed-in User has a planning Run going other than
// the Run except, or a pending Plan Proposal, for any Cycle.
func planningBusy(ctx context.Context, q db.Querier, except int64) (busy bool, err error) {
	err = q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM run WHERE kind = $1 AND status = $2 AND id <> $3)
			OR EXISTS (SELECT 1 FROM plan_proposal WHERE status = $4)`,
		Kind, runs.Going, except, Pending).Scan(&busy)
	return busy, err
}

// savePlanningRun records that the Run runID plans the Cycle cycleID from the Slot first.
func savePlanningRun(ctx context.Context, q db.Querier, runID, cycleID int64, first domain.Slot) error {
	_, err := q.Exec(ctx, `INSERT INTO planning_run (run_id, cycle_id, first_on, first_kind) VALUES ($1, $2, $3, $4)`,
		runID, cycleID, first.On.Format(time.DateOnly), first.Kind)
	return err
}

// goingRun is the id of the signed-in User's planning Run going for the Cycle id, or 0.
func goingRun(ctx context.Context, q db.Querier, id int64) (runID int64, err error) {
	err = q.QueryRow(ctx, `
		SELECT coalesce(max(r.id), 0) FROM planning_run p JOIN run r ON r.id = p.run_id
		WHERE p.cycle_id = $1 AND r.status = $2`, id, runs.Going).Scan(&runID)
	return runID, err
}

// saveProposal stores plan, summed up by summary, as the pending Plan Proposal of the
// planning Run runID.
func saveProposal(ctx context.Context, q db.Querier, runID int64, plan domain.Plan, summary string) error {
	var id int64
	if err := q.QueryRow(ctx, `INSERT INTO plan_proposal (run_id, status, summary) VALUES ($1, $2, $3) RETURNING id`, runID, Pending, summary).Scan(&id); err != nil {
		return err
	}
	day := func(s *domain.Slot) (*string, *domain.SlotKind) {
		if s == nil {
			return nil, nil
		}
		on := s.On.Format(time.DateOnly)
		return &on, &s.Kind
	}
	for _, ps := range plan.PrepSessions {
		var recipe, ingredient *int64
		var size, quantity *float64
		if ps.Recipe != nil {
			recipe, size = &ps.Recipe.ID, &ps.Size
		} else {
			q := ps.Quantity.Number()
			ingredient, quantity = &ps.Ingredient.ID, &q
		}
		on, kind := day(&ps.At)
		var sessionID int64
		err := q.QueryRow(ctx, `
			INSERT INTO prep_session (plan_proposal_id, prepared_on, prepared_kind, recipe_id, size, ingredient_id, quantity)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			id, on, kind, recipe, size, ingredient, quantity).Scan(&sessionID)
		if err != nil {
			return err
		}
		for i, pp := range ps.Portions {
			on, kind := day(pp.EatAt)
			_, err := q.Exec(ctx, `
				INSERT INTO planned_portion (prep_session_id, position, location, eat_on, eat_kind) VALUES ($1, $2, $3, $4, $5)`,
				sessionID, i, pp.Location, on, kind)
			if err != nil {
				return err
			}
		}
	}
	for _, k := range plan.Kept {
		_, err := q.Exec(ctx, `INSERT INTO kept_portion (plan_proposal_id, portion_id, eat_on, eat_kind) VALUES ($1, $2, $3, $4)`,
			id, k.Portion, k.At.On.Format(time.DateOnly), k.At.Kind)
		if err != nil {
			return err
		}
	}
	return nil
}

// storedProposal is a Plan Proposal as stored: its Prep Sessions with their Recipes and
// Ingredients by id only.
type storedProposal struct {
	summary  string
	sessions []storedSession
	kept     []domain.Kept
}

type storedSession struct {
	domain.PrepSession
	recipeID, ingredientID int64
}

// loadPending reads the signed-in User's pending Plan Proposal of the Cycle id, its days
// in zone, or nil when it has none.
func loadPending(ctx context.Context, q db.Querier, id int64, zone *time.Location) (*storedProposal, error) {
	var sp storedProposal
	var proposalID int64
	err := q.QueryRow(ctx, `
		SELECT pp.id, pp.summary FROM plan_proposal pp JOIN planning_run p ON p.run_id = pp.run_id
		WHERE p.cycle_id = $1 AND pp.status = $2`, id, Pending).Scan(&proposalID, &sp.summary)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	in := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, zone) }
	rows, err := q.Query(ctx, `
		SELECT s.id, s.prepared_on, s.prepared_kind, coalesce(s.recipe_id, 0), coalesce(s.size, 0)::float8,
			coalesce(s.ingredient_id, 0), coalesce(s.quantity, 0)::float8, p.location, p.eat_on, coalesce(p.eat_kind, '')
		FROM prep_session s JOIN planned_portion p ON p.prep_session_id = s.id
		WHERE s.plan_proposal_id = $1 ORDER BY s.id, p.position`, proposalID)
	if err != nil {
		return nil, err
	}
	var last int64
	for rows.Next() {
		var sessionID int64
		var ps storedSession
		var quantity float64
		var pp domain.PlannedPortion
		var eatOn *time.Time
		var eatKind domain.SlotKind
		err := rows.Scan(&sessionID, &ps.At.On, &ps.At.Kind, &ps.recipeID, &ps.Size, &ps.ingredientID, &quantity, &pp.Location, &eatOn, &eatKind)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if eatOn != nil {
			pp.EatAt = &domain.Slot{On: in(*eatOn), Kind: eatKind}
		}
		if sessionID != last {
			ps.At.On, ps.Quantity = in(ps.At.On), domain.QuantityOf(quantity)
			sp.sessions = append(sp.sessions, ps)
			last = sessionID
		}
		session := &sp.sessions[len(sp.sessions)-1]
		session.Portions = append(session.Portions, pp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, _ = q.Query(ctx, `SELECT portion_id, eat_on, eat_kind FROM kept_portion WHERE plan_proposal_id = $1 ORDER BY eat_on`, proposalID)
	sp.kept, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (k domain.Kept, err error) {
		err = row.Scan(&k.Portion, &k.At.On, &k.At.Kind)
		k.At.On = in(k.At.On)
		return k, err
	})
	return &sp, err
}
