package planning

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// ingredientColumns are an Ingredient i's columns, as scanIngredient reads
// them, with the terms it falls under.
const ingredientColumns = `i.name, i.unit, i.kcal, i.protein_g, i.carbs_g, i.fat_g,
	coalesce((SELECT array_agg(f.term ORDER BY f.term) FROM ingredient_falls_under f WHERE f.ingredient_id = i.id), '{}')`

func ingredientFields(ing *domain.Ingredient) []any {
	n := &ing.Nutrition
	return []any{&ing.Name, &ing.Unit, &n.Kcal, &n.ProteinG, &n.CarbsG, &n.FatG, &ing.FallsUnder}
}

// cycleColumns are a Cycle's columns, as scanCycle reads them.
var cycleColumns = `id, delivery_on, ` + db.Minutes("window_from") + `, ` + db.Minutes("window_to") + `, ordered,
	starts_on, starts_with, ends_on, closed_on, closed_with, status, needs_proposal_because`

func scanCycle(row pgx.Row) (*Cycle, error) {
	var c Cycle
	var closedOn *time.Time
	var closedWith *domain.SlotKind
	d := &c.Delivery
	err := row.Scan(&c.ID, &d.On, &d.From, &d.To, &d.Ordered,
		&c.Start.On, &c.Start.Kind, &c.EndsOn, &closedOn, &closedWith, &c.Status, &c.NeedsProposalBecause)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if closedOn != nil {
		c.ClosedAt = &domain.Slot{On: *closedOn, Kind: *closedWith}
	}
	return &c, err
}

// loadLatestCycle loads the Cycle delivered last, or nil when there is
// none.
func loadLatestCycle(ctx context.Context, q db.Querier) (*Cycle, error) {
	return scanCycle(q.QueryRow(ctx, `SELECT `+cycleColumns+` FROM cycle ORDER BY delivery_on DESC LIMIT 1`))
}

// loadPreviousCycle loads the closed Cycle delivered last, or nil when
// none has closed.
func loadPreviousCycle(ctx context.Context, q db.Querier) (*Cycle, error) {
	return scanCycle(q.QueryRow(ctx, `SELECT `+cycleColumns+` FROM cycle WHERE status = 'closed' ORDER BY delivery_on DESC LIMIT 1`))
}

// loadCycleWith loads the Cycle with status upcoming or current, or nil
// when none has it.
func loadCycleWith(ctx context.Context, q db.Querier, status string) (*Cycle, error) {
	return scanCycle(q.QueryRow(ctx, `SELECT `+cycleColumns+` FROM cycle WHERE status = $1`, status))
}

// findCycle loads the Cycle with id, or nil when there is none.
func findCycle(ctx context.Context, q db.Querier, id int64) (*Cycle, error) {
	return scanCycle(q.QueryRow(ctx, `SELECT `+cycleColumns+` FROM cycle WHERE id = $1`, id))
}

func loadCycle(ctx context.Context, q db.Querier, id int64) (Cycle, error) {
	c, err := findCycle(ctx, q, id)
	if c == nil {
		return Cycle{}, cmp.Or(err, pgx.ErrNoRows)
	}
	return *c, err
}

// loadCyclesFrom loads the Cycles planned to end on or after the day
// first, closed ones included.
func loadCyclesFrom(ctx context.Context, q db.Querier, first time.Time) ([]Cycle, error) {
	rows, _ := q.Query(ctx, `SELECT `+cycleColumns+` FROM cycle WHERE ends_on >= $1 ORDER BY delivery_on`, first)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Cycle, error) {
		c, err := scanCycle(row)
		return *c, err
	})
}

// lockCycles serialises changes to Cycles: opening, moving, dropping and
// arriving.
func lockCycles(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `LOCK TABLE cycle IN EXCLUSIVE MODE`)
	return err
}

// loadSince loads the first Cycle's first day.
func loadSince(ctx context.Context, q db.Querier) (time.Time, error) {
	var since time.Time
	err := q.QueryRow(ctx, `SELECT min(starts_on) FROM cycle`).Scan(&since)
	return since, err
}

// insertCycle stores c as the upcoming Cycle, returning its id.
func insertCycle(ctx context.Context, q db.Querier, c domain.Cycle) (int64, error) {
	d := c.Delivery
	var id int64
	err := q.QueryRow(ctx, `
		INSERT INTO cycle (delivery_on, window_from, window_to, ordered, starts_on, starts_with, ends_on)
		VALUES ($1, `+db.Time("$2")+`, `+db.Time("$3")+`, $4, $5, $6, $7) RETURNING id`,
		d.On, d.From, d.To, d.Ordered, c.Start.On, c.Start.Kind, c.EndsOn).Scan(&id)
	return id, err
}

// updateDelivery stores c's Delivery, first Slot and end date as those of
// the Cycle with id.
func updateDelivery(ctx context.Context, q db.Querier, id int64, c domain.Cycle) error {
	d := c.Delivery
	_, err := q.Exec(ctx, `
		UPDATE cycle SET delivery_on = $2, window_from = `+db.Time("$3")+`, window_to = `+db.Time("$4")+`,
			ordered = $5, starts_on = $6, starts_with = $7, ends_on = $8
		WHERE id = $1`, id, d.On, d.From, d.To, d.Ordered, c.Start.On, c.Start.Kind, c.EndsOn)
	return err
}

// moveCycle marks the Plan of the Cycle with id, moved to c, as needing a
// new Plan Proposal, and drops the Prep Sessions and Slots now outside the
// Cycle. Its Delivery hasn't arrived, so none of its Prep Sessions was
// prepared.
func moveCycle(ctx context.Context, q db.Querier, id int64, c domain.Cycle) error {
	if err := setNeedsProposal(ctx, q, id, deliveryMoved); err != nil {
		return err
	}
	start, until := c.Start, c.Until()
	outside := func(day, kind string) string {
		return `((` + day + `, ` + order(kind) + `) < ($2::date, $3) OR (` + day + `, ` + order(kind) + `) >= ($4::date, $5))`
	}
	args := []any{id, start.On, slotOrder(start), until.On, slotOrder(until)}
	if _, err := q.Exec(ctx, `DELETE FROM prep_session WHERE cycle_id = $1 AND `+outside("prepared_on", "prepared_for"), args...); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `DELETE FROM slot WHERE cycle_id = $1 AND `+outside("day", "kind"), args...)
	return err
}

// deliveryMoved is why a Plan a Delivery moved under needs a new Plan
// Proposal.
const deliveryMoved = "A Delivery moved"

// exceptionChanged is why a Plan needs a new Plan Proposal once an
// Exception changed one of its Slots.
const exceptionChanged = "An Exception changed"

// carryOverChanged is why the upcoming Cycle's Plan needs a new Plan
// Proposal once one approved in the current Cycle changed what will carry
// over.
const carryOverChanged = "The current Cycle's Plan changed what carries over"

// setNeedsProposal marks the Plan of the Cycle with id as needing a new
// Plan Proposal, because of what because says, or, when it is empty, as
// not needing one.
func setNeedsProposal(ctx context.Context, q db.Querier, id int64, because string) error {
	_, err := q.Exec(ctx, `UPDATE cycle SET needs_proposal_because = $2 WHERE id = $1`, id, because)
	return err
}

func setStatus(ctx context.Context, q db.Querier, id int64, status string) error {
	_, err := q.Exec(ctx, `UPDATE cycle SET status = $2 WHERE id = $1`, id, status)
	return err
}

// decidePending decides the Plan Proposals of the Cycles with ids still
// pending as status, rejected or replaced, because of cause: not the
// user's doing, so not a Planning Note.
func decidePending(ctx context.Context, q db.Querier, ids []int64, status, cause string) error {
	_, err := q.Exec(ctx, `
		UPDATE plan_proposal SET status = $2, cause = $3, decided_at = now()
		WHERE cycle_id = ANY($1) AND status = 'pending'`, ids, status, cause)
	return err
}

// closeCycle closes the Cycle with id as the next Delivery's arrival a
// says, rejecting the Plan Proposals still pending, which can no longer
// apply, because of cause. Closing early, it drops the Prep Sessions never
// prepared and takes the Portions carried over out of their Slots.
func closeCycle(ctx context.Context, q db.Querier, id int64, a domain.Arrival, cause string) error {
	var closedOn *time.Time
	var closedWith *domain.SlotKind
	if c := a.Current.ClosedAt; c != nil {
		closedOn, closedWith = &c.On, &c.Kind
	}
	if _, err := q.Exec(ctx, `UPDATE cycle SET status = 'closed', closed_on = $2, closed_with = $3 WHERE id = $1`, id, closedOn, closedWith); err != nil {
		return err
	}
	if err := decidePending(ctx, q, []int64{id}, "rejected", cause); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM prep_session WHERE id = ANY($1)`, a.DroppedPrepSessions); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `DELETE FROM slot WHERE portion_id = ANY($1)`, a.CarriedOver)
	return err
}

// deleteCycle deletes the Cycle with id, with its Prep Sessions and their
// Portions, its Slots and its Plan Proposals.
func deleteCycle(ctx context.Context, q db.Querier, id int64) error {
	if _, err := q.Exec(ctx, `DELETE FROM prep_session WHERE cycle_id = $1`, id); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `DELETE FROM cycle WHERE id = $1`, id)
	return err
}

// loadRecipes loads the Recipes with ids, with their Ingredients.
func loadRecipes(ctx context.Context, q db.Querier, ids []int64) (map[int64]*domain.Recipe, error) {
	rows, _ := q.Query(ctx, `SELECT id, name, portions FROM recipe WHERE id = ANY($1)`, ids)
	recipes := map[int64]*domain.Recipe{}
	var id int64
	var r domain.Recipe
	if _, err := pgx.ForEachRow(rows, []any{&id, &r.Name, &r.Portions}, func() error {
		rec := r
		recipes[id] = &rec
		return nil
	}); err != nil {
		return nil, err
	}
	rows, _ = q.Query(ctx, `
		SELECT ri.recipe_id, ri.quantity, `+ingredientColumns+`
		FROM recipe_ingredient ri JOIN ingredient i ON i.id = ri.ingredient_id
		WHERE ri.recipe_id = ANY($1) ORDER BY ri.recipe_id, ri.position`, ids)
	var ri domain.RecipeIngredient
	_, err := pgx.ForEachRow(rows, append([]any{&id, &ri.Quantity}, ingredientFields(&ri.Ingredient)...), func() error {
		recipes[id].Ingredients = append(recipes[id].Ingredients, ri)
		return nil
	})
	return recipes, err
}

// loadIngredients loads the Ingredients named names, by name.
func loadIngredients(ctx context.Context, q db.Querier, names []string) (map[string]domain.Ingredient, error) {
	rows, _ := q.Query(ctx, `SELECT `+ingredientColumns+` FROM ingredient i WHERE i.name = ANY($1)`, names)
	known := map[string]domain.Ingredient{}
	var ing domain.Ingredient
	_, err := pgx.ForEachRow(rows, ingredientFields(&ing), func() error {
		known[ing.Name] = ing
		return nil
	})
	return known, err
}

// notUsedUp loads the ids of the Prep Sessions that haven't used up their
// stock yet.
func notUsedUp(ctx context.Context, q db.Querier) ([]int64, error) {
	rows, _ := q.Query(ctx, `SELECT id FROM prep_session WHERE NOT used_up`)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// markUsedUp marks the Prep Sessions with ids as having used up their
// stock, and returns those that hadn't. A concurrent mark waits for this
// one's transaction, then finds them marked.
func markUsedUp(ctx context.Context, q db.Querier, ids []int64) ([]int64, error) {
	rows, _ := q.Query(ctx, `
		UPDATE prep_session SET used_up = true
		WHERE id = ANY($1) AND NOT used_up
		RETURNING id`, ids)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// loadPrepSessions loads the Prep Sessions with ids, by id, with what they
// prepare and every Portion they yield, each eaten at its Slot if it has
// one.
func loadPrepSessions(ctx context.Context, q db.Querier, ids []int64) (map[int64]*domain.PrepSession, error) {
	rows, _ := q.Query(ctx, `
		SELECT id, recipe_id, prepared_on, prepared_for
		FROM prep_session WHERE id = ANY($1)`, ids)
	sessions := map[int64]*domain.PrepSession{}
	recipeOf := map[int64]int64{}
	var ps domain.PrepSession
	var recipeID *int64
	// Prep Sessions from before planning have no Slot they are prepared for.
	var on *time.Time
	var kind *domain.SlotKind
	if _, err := pgx.ForEachRow(rows, []any{&ps.ID, &recipeID, &on, &kind}, func() error {
		s := ps
		if on != nil {
			s.At = domain.Slot{On: *on, Kind: *kind}
		}
		sessions[ps.ID] = &s
		if recipeID != nil {
			recipeOf[ps.ID] = *recipeID
		}
		return nil
	}); err != nil {
		return nil, err
	}
	var recipeIDs []int64
	for _, id := range recipeOf {
		recipeIDs = append(recipeIDs, id)
	}
	recipes, err := loadRecipes(ctx, q, recipeIDs)
	if err != nil {
		return nil, err
	}
	for session, recipe := range recipeOf {
		sessions[session].Recipe = recipes[recipe]
	}

	rows, _ = q.Query(ctx, `
		SELECT psi.prep_session_id, psi.quantity, `+ingredientColumns+`
		FROM prep_session_ingredient psi JOIN ingredient i ON i.id = psi.ingredient_id
		WHERE psi.prep_session_id = ANY($1) ORDER BY psi.prep_session_id, psi.position`, ids)
	var id int64
	var ri domain.RecipeIngredient
	if _, err := pgx.ForEachRow(rows, append([]any{&id, &ri.Quantity}, ingredientFields(&ri.Ingredient)...), func() error {
		sessions[id].Ingredients = append(sessions[id].Ingredients, ri)
		return nil
	}); err != nil {
		return nil, err
	}

	rows, _ = q.Query(ctx, `
		SELECT p.id, p.prep_session_id, p.location, s.day, s.kind
		FROM portion p LEFT JOIN slot s ON s.portion_id = p.id
		WHERE p.prep_session_id = ANY($1) ORDER BY p.id`, ids)
	var p domain.PlannedPortion
	var eatOn *time.Time
	var eatKind *domain.SlotKind
	_, err = pgx.ForEachRow(rows, []any{&p.ID, &id, &p.Location, &eatOn, &eatKind}, func() error {
		portion := p
		if eatOn != nil {
			portion.EatAt = &domain.Slot{On: *eatOn, Kind: *eatKind}
		}
		sessions[id].Portions = append(sessions[id].Portions, portion)
		return nil
	})
	return sessions, err
}

// portionRef is a stored Portion, its Prep Session and its Slot, if any.
type portionRef struct {
	id, prepSession int64
	slot            *domain.Slot
}

// loadPortionRefs runs sql, which selects a Portion's id, its Prep
// Session's id and its Slot's day and kind (or nulls), with args.
func loadPortionRefs(ctx context.Context, q db.Querier, sql string, args ...any) ([]portionRef, error) {
	rows, _ := q.Query(ctx, sql, args...)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (portionRef, error) {
		var r portionRef
		var day *time.Time
		var kind *domain.SlotKind
		err := row.Scan(&r.id, &r.prepSession, &day, &kind)
		if day != nil {
			r.slot = &domain.Slot{On: *day, Kind: *kind}
		}
		return r, err
	})
}

// keptPortions turns refs into Portions a Plan can use, with what they are
// and hold.
func keptPortions(ctx context.Context, q db.Querier, refs []portionRef) ([]domain.KeptPortion, error) {
	var ids []int64
	for _, r := range refs {
		ids = append(ids, r.prepSession)
	}
	sessions, err := loadPrepSessions(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	var kept []domain.KeptPortion
	for _, r := range refs {
		ps := sessions[r.prepSession]
		kept = append(kept, domain.KeptPortion{ID: r.id, Name: ps.Name(), Nutrition: ps.NutritionPerPortion()})
	}
	return kept, nil
}

// loadPlan loads the Plan of the Cycle with id, with the Portions without
// a Slot it puts in its Slots.
func loadPlan(ctx context.Context, q db.Querier, id int64) (domain.Plan, []domain.KeptPortion, error) {
	var plan domain.Plan
	rows, _ := q.Query(ctx, `SELECT id FROM prep_session WHERE cycle_id = $1`, id)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return plan, nil, err
	}
	sessions, err := loadPrepSessions(ctx, q, ids)
	if err != nil {
		return plan, nil, err
	}
	plan.PrepSessions = inDayOrder(sessions)

	refs, err := loadPortionRefs(ctx, q, `
		SELECT p.id, p.prep_session_id, s.day, s.kind
		FROM slot s JOIN portion p ON p.id = s.portion_id JOIN prep_session ps ON ps.id = p.prep_session_id
		WHERE s.cycle_id = $1 AND ps.cycle_id IS DISTINCT FROM $1 ORDER BY s.day, p.id`, id)
	if err != nil {
		return plan, nil, err
	}
	for _, r := range refs {
		plan.Kept = append(plan.Kept, domain.KeptAssignment{PortionID: r.id, Slot: *r.slot})
	}
	kept, err := keptPortions(ctx, q, refs)
	return plan, kept, err
}

// order is the SQL position of the eating occasion in column kind within
// its day, as slotOrder gives it in Go: compare (day, order) pairs to
// compare Slots.
func order(kind string) string {
	return `array_position(ARRAY['breakfast', 'lunch', 'snack', 'dinner'], ` + kind + `)`
}

func slotOrder(s domain.Slot) int { return slices.Index(domain.SlotKinds, s.Kind) + 1 }

// loadKept loads the Portions a Plan of the Cycle with id from the Slot
// from on may put in Slots: those of Prep Sessions before prepared, which
// stay, not eaten before from, either without a Slot or in one of the
// Cycle's from from on, which the Plan replaces.
func loadKept(ctx context.Context, q db.Querier, id int64, from, prepared domain.Slot) ([]domain.KeptPortion, error) {
	refs, err := loadPortionRefs(ctx, q, `
		SELECT p.id, p.prep_session_id, NULL::date, NULL::text
		FROM portion p JOIN prep_session ps ON ps.id = p.prep_session_id
		LEFT JOIN slot s ON s.portion_id = p.id
		WHERE (ps.cycle_id IS DISTINCT FROM $1 OR (ps.prepared_on, `+order("ps.prepared_for")+`) < ($4::date, $5))
		  AND (s.id IS NULL OR (s.cycle_id = $1 AND (s.day, `+order("s.kind")+`) >= ($2::date, $3)))
		ORDER BY p.id`, id, from.On, slotOrder(from), prepared.On, slotOrder(prepared))
	if err != nil {
		return nil, err
	}
	return keptPortions(ctx, q, refs)
}

// loadEarlier loads what the Slots before from eat, from six days before
// c's first day on: all a trailing seven days of c can reach. They
// are earlier Cycles' Slots, and c's own that a Plan from from on keeps;
// an excepted Slot's Portion isn't eaten, and only its Exception counts.
func loadEarlier(ctx context.Context, q db.Querier, c Cycle, from domain.Slot) ([]domain.DayNutrition, error) {
	refs, err := loadPortionRefs(ctx, q, `
		SELECT p.id, p.prep_session_id, s.day, s.kind
		FROM slot s JOIN portion p ON p.id = s.portion_id
		WHERE s.day >= $1::date - 6 AND (s.day, `+order("s.kind")+`) < ($2::date, $3)
			AND NOT EXISTS (SELECT FROM slot_exception e WHERE e.day = s.day AND e.kind = s.kind)`,
		c.Start.On, from.On, slotOrder(from))
	if err != nil {
		return nil, err
	}
	kept, err := keptPortions(ctx, q, refs)
	if err != nil {
		return nil, err
	}
	var days []domain.DayNutrition
	for i, r := range refs {
		days = append(days, domain.DayNutrition{On: r.slot.On, Nutrition: kept[i].Nutrition})
	}
	return days, nil
}

// replacePlan replaces the Plan of c from the Slot from on with plan: its
// Prep Sessions, their Portions, and the Slots they and the kept Portions
// fill. Prep Sessions before prepared stay, with their Portions; those not
// eaten before from lose their Slot, for plan to fill again.
func replacePlan(ctx context.Context, q db.Querier, c Cycle, from, prepared domain.Slot, plan proposedPlan) error {
	if _, err := q.Exec(ctx, `
		DELETE FROM slot WHERE cycle_id = $1 AND (day, `+order("kind")+`) >= ($2::date, $3)`,
		c.ID, from.On, slotOrder(from)); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		DELETE FROM prep_session WHERE cycle_id = $1 AND (prepared_on, `+order("prepared_for")+`) >= ($2::date, $3)`,
		c.ID, prepared.On, slotOrder(prepared)); err != nil {
		return err
	}
	// Slots before from are what stays; validation flags a Plan filling them.
	inPlan := func(s domain.Slot) bool { return !s.Before(from) && c.Contains(s) }
	fill := func(portion int64, s domain.Slot) error {
		_, err := q.Exec(ctx, `INSERT INTO slot (cycle_id, day, kind, portion_id) VALUES ($1, $2, $3, $4)`, c.ID, s.On, s.Kind, portion)
		return err
	}
	for _, ps := range plan.PrepSessions {
		var id int64
		if err := q.QueryRow(ctx, `
			INSERT INTO prep_session (recipe_id, cycle_id, prepared_on, prepared_for)
			VALUES ($1, $2, $3, $4) RETURNING id`, ps.RecipeID, c.ID, ps.at.On, ps.at.Kind).Scan(&id); err != nil {
			return err
		}
		for i, ing := range ps.Ingredients {
			if _, err := q.Exec(ctx, `
				INSERT INTO prep_session_ingredient (prep_session_id, position, ingredient_id, quantity)
				SELECT $1, $2, id, $4 FROM ingredient WHERE name = $3`, id, i, ing.Name, ing.Quantity); err != nil {
				return err
			}
		}
		for _, p := range ps.Portions {
			var portion int64
			if err := q.QueryRow(ctx, `INSERT INTO portion (prep_session_id, location) VALUES ($1, $2) RETURNING id`, id, p.Location).Scan(&portion); err != nil {
				return err
			}
			if p.eatAt != nil && inPlan(*p.eatAt) {
				if err := fill(portion, *p.eatAt); err != nil {
					return err
				}
			}
		}
	}
	for _, k := range plan.Kept {
		if !inPlan(k.slot) {
			continue
		}
		if err := fill(k.PortionID, k.slot); err != nil {
			return err
		}
	}
	return nil
}

// storedProposal is a Plan Proposal as stored, its Plan not yet read.
type storedProposal struct {
	ID, CycleID int64
	// RunID is the planning Run that submitted it; 0 when none did.
	RunID  int64
	Origin string
	Status string
	Note   string
	Cause  string
	Plan   []byte
}

// loadProposal loads the Plan Proposal with id, locking it until the
// transaction ends when lock is set. It returns ErrNotFound when there is
// none.
func loadProposal(ctx context.Context, q db.Querier, id int64, lock bool) (storedProposal, error) {
	sql := `SELECT id, cycle_id, coalesce(run_id, 0), origin, status, note, cause, plan FROM plan_proposal WHERE id = $1`
	if lock {
		sql += ` FOR UPDATE`
	}
	var p storedProposal
	err := q.QueryRow(ctx, sql, id).Scan(&p.ID, &p.CycleID, &p.RunID, &p.Origin, &p.Status, &p.Note, &p.Cause, &p.Plan)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// loadProposals loads the Plan Proposals of the Cycle with id, newest
// first.
func loadProposals(ctx context.Context, q db.Querier, id int64) ([]ProposalSummary, error) {
	rows, _ := q.Query(ctx, `
		SELECT id, origin, status FROM plan_proposal
		WHERE cycle_id = $1 ORDER BY id DESC`, id)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ProposalSummary, error) {
		var p ProposalSummary
		err := row.Scan(&p.ID, &p.Origin, &p.Status)
		return p, err
	})
}

func decideProposal(ctx context.Context, q db.Querier, id int64, status, note string) error {
	_, err := q.Exec(ctx, `UPDATE plan_proposal SET status = $2, note = $3, decided_at = now() WHERE id = $1`, id, status, note)
	return err
}

// hasPlan reports whether a Plan Proposal of the Cycle with id was approved.
func hasPlan(ctx context.Context, q db.Querier, id int64) (bool, error) {
	var approved bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT FROM plan_proposal WHERE cycle_id = $1 AND status = 'approved')`, id).Scan(&approved)
	return approved, err
}

// loadExceptions loads the Exceptions logged for the Slots from day first
// to day last, in order.
func loadExceptions(ctx context.Context, q db.Querier, first, last time.Time) ([]domain.Exception, error) {
	rows, _ := q.Query(ctx, `
		SELECT day, kind, exception, kcal, protein_g, carbs_g, fat_g FROM slot_exception
		WHERE day BETWEEN $1 AND $2 ORDER BY day, `+order("kind"), first, last)
	return collectExceptions(rows)
}

// loadExceptionsFrom loads the Exceptions from the day first on.
func loadExceptionsFrom(ctx context.Context, q db.Querier, first time.Time) ([]domain.Exception, error) {
	rows, _ := q.Query(ctx, `
		SELECT day, kind, exception, kcal, protein_g, carbs_g, fat_g FROM slot_exception
		WHERE day >= $1 ORDER BY day, `+order("kind"), first)
	return collectExceptions(rows)
}

func collectExceptions(rows pgx.Rows) ([]domain.Exception, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Exception, error) {
		var e domain.Exception
		est := &e.Estimate
		err := row.Scan(&e.Slot.On, &e.Slot.Kind, &e.Kind, &est.Kcal, &est.ProteinG, &est.CarbsG, &est.FatG)
		return e, err
	})
}

// saveException logs e for slot, replacing what it had, or removes the
// Exception slot has when e is nil.
func saveException(ctx context.Context, q db.Querier, slot domain.Slot, e *domain.Exception) error {
	if e == nil {
		_, err := q.Exec(ctx, `DELETE FROM slot_exception WHERE day = $1 AND kind = $2`, slot.On, slot.Kind)
		return err
	}
	est := e.Estimate
	_, err := q.Exec(ctx, `
		INSERT INTO slot_exception (day, kind, exception, kcal, protein_g, carbs_g, fat_g)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (day, kind) DO UPDATE SET exception = EXCLUDED.exception,
			kcal = EXCLUDED.kcal, protein_g = EXCLUDED.protein_g, carbs_g = EXCLUDED.carbs_g, fat_g = EXCLUDED.fat_g`,
		slot.On, slot.Kind, e.Kind, est.Kcal, est.ProteinG, est.CarbsG, est.FatG)
	return err
}

// lockCycle locks the Cycle with id until the transaction ends.
func lockCycle(ctx context.Context, q db.Querier, id int64) error {
	_, err := q.Exec(ctx, `SELECT FROM cycle WHERE id = $1 FOR UPDATE`, id)
	return err
}

// pendingFor is the Cycle a Plan Proposal is pending for, with its
// status; 0 when none is pending.
func pendingFor(ctx context.Context, q db.Querier) (id int64, status string, err error) {
	err = q.QueryRow(ctx, `
		SELECT c.id, c.status FROM plan_proposal p JOIN cycle c ON c.id = p.cycle_id
		WHERE p.status = 'pending' ORDER BY p.id LIMIT 1`).Scan(&id, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil
	}
	return id, status, err
}

// goingPlanningRun is the planning Run going, the Cycle it plans and that
// Cycle's status; run is 0 when none is going.
func goingPlanningRun(ctx context.Context, q db.Querier) (run, id int64, status string, err error) {
	err = q.QueryRow(ctx, `
		SELECT p.run_id, c.id, c.status FROM planning_run p
		JOIN run r ON r.id = p.run_id JOIN cycle c ON c.id = p.cycle_id
		WHERE r.status = 'running' ORDER BY p.run_id LIMIT 1`).Scan(&run, &id, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, "", nil
	}
	return run, id, status, err
}

// planningRunsGoing are the planning Runs going for the Cycles with ids
// that have yet to store their Plan Proposal.
func planningRunsGoing(ctx context.Context, q db.Querier, ids []int64) ([]int64, error) {
	rows, _ := q.Query(ctx, `
		SELECT p.run_id FROM planning_run p JOIN run r ON r.id = p.run_id
		WHERE p.cycle_id = ANY($1) AND r.status = 'running'
			AND NOT EXISTS (SELECT FROM plan_proposal pp WHERE pp.run_id = p.run_id)`, ids)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// runStatus is the status of the Run with id.
func runStatus(ctx context.Context, q db.Querier, id int64) (string, error) {
	var status string
	err := q.QueryRow(ctx, `SELECT status FROM run WHERE id = $1`, id).Scan(&status)
	return status, err
}

// openCycleIDs are the ids of the current and the upcoming Cycle, those
// there are.
func openCycleIDs(ctx context.Context, q db.Querier) ([]int64, error) {
	rows, _ := q.Query(ctx, `SELECT id FROM cycle WHERE status IN ('current', 'upcoming') ORDER BY delivery_on`)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// latestPlanningRun is the last planning Run of the Cycle with id and its
// status; status is empty when it has had none.
func latestPlanningRun(ctx context.Context, q db.Querier, id int64) (run int64, status string, err error) {
	err = q.QueryRow(ctx, `
		SELECT p.run_id, r.status FROM planning_run p JOIN run r ON r.id = p.run_id
		WHERE p.cycle_id = $1 ORDER BY p.run_id DESC LIMIT 1`, id).Scan(&run, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil
	}
	return run, status, err
}

// planningRunAfter is the planning Run the rejection of the Plan Proposal
// with id started, if it started one.
func planningRunAfter(ctx context.Context, q db.Querier, id int64) (int64, error) {
	var run int64
	err := q.QueryRow(ctx, `SELECT coalesce(max(run_id), 0) FROM planning_run WHERE after_proposal = $1`, id).Scan(&run)
	return run, err
}

// insertPlanningRun records run as a planning Run of the Cycle with id,
// started by rejecting the Plan Proposal with id after, or by the user
// when after is 0.
func insertPlanningRun(ctx context.Context, q db.Querier, run, id, after int64) error {
	_, err := q.Exec(ctx, `INSERT INTO planning_run (run_id, cycle_id, after_proposal) VALUES ($1, $2, nullif($3, 0))`, run, id, after)
	return err
}

// loadNotes loads the notes of the Cycle with id's rejected Plan
// Proposals, newest first.
func loadNotes(ctx context.Context, q db.Querier, id int64) ([]string, error) {
	rows, _ := q.Query(ctx, `
		SELECT note FROM plan_proposal WHERE cycle_id = $1 AND status = 'rejected' AND note <> ''
		ORDER BY decided_at DESC, id DESC`, id)
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// insertProposal stores plan as the initial Plan Proposal of the Cycle
// with id, submitted by the planning Run with run.
func insertProposal(ctx context.Context, q db.Querier, id, run int64, plan []byte) error {
	_, err := q.Exec(ctx, `INSERT INTO plan_proposal (cycle_id, origin, plan, run_id) VALUES ($1, 'initial', $2, $3)`, id, plan, run)
	return err
}

// groceryListFixed reports whether the Grocery List of the Cycle with id is
// fixed, its lines stored.
func groceryListFixed(ctx context.Context, q db.Querier, id int64) (bool, error) {
	var fixed bool
	err := q.QueryRow(ctx, `SELECT grocery_list_fixed FROM cycle WHERE id = $1`, id).Scan(&fixed)
	return fixed, err
}

// loadGroceryList loads the Grocery List fixed for the Cycle with id, by
// Ingredient name.
func loadGroceryList(ctx context.Context, q db.Querier, id int64) (domain.GroceryList, error) {
	rows, _ := q.Query(ctx, `
		SELECT g.needed, g.product_code, g.product_name, g.pack_quantity, g.aisle, g.proposed, g.packs, `+ingredientColumns+`
		FROM grocery_line g JOIN ingredient i ON i.id = g.ingredient_id
		WHERE g.cycle_id = $1 ORDER BY i.name`, id)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.GroceryLine, error) {
		var l domain.GroceryLine
		var code, name, aisle *string
		var pack *float64
		var proposed *bool
		var packs *int
		err := row.Scan(append([]any{&l.Needed, &code, &name, &pack, &aisle, &proposed, &packs}, ingredientFields(&l.Ingredient)...)...)
		if code != nil {
			l.Mapping = &domain.ProductMapping{
				Product:  domain.CatalogProduct{Code: *code, Name: *name, PackQuantity: *pack, Unit: l.Ingredient.Unit, Aisle: *aisle},
				Proposed: *proposed,
			}
			l.Packs = *packs
		}
		return l, err
	})
}

// storeGroceryList stores list as the fixed Grocery List of the Cycle with
// id.
func storeGroceryList(ctx context.Context, q db.Querier, id int64, list domain.GroceryList) error {
	for _, l := range list {
		var code, name, aisle *string
		var pack *float64
		var proposed *bool
		var packs *int
		if m := l.Mapping; m != nil {
			code, name, pack, aisle, proposed, packs = &m.Product.Code, &m.Product.Name, &m.Product.PackQuantity, &m.Product.Aisle, &m.Proposed, &l.Packs
		}
		tag, err := q.Exec(ctx, `
			INSERT INTO grocery_line (cycle_id, ingredient_id, needed, product_code, product_name, pack_quantity, aisle, proposed, packs)
			SELECT $1, id, $3, $4, $5, $6, $7, $8, $9 FROM ingredient WHERE name = $2`,
			id, l.Ingredient.Name, l.Needed, code, name, pack, aisle, proposed, packs)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("grocery list of cycle %d: no ingredient %q", id, l.Ingredient.Name)
		}
	}
	_, err := q.Exec(ctx, `UPDATE cycle SET grocery_list_fixed = true WHERE id = $1`, id)
	return err
}
