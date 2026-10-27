package planning

import (
	"cmp"
	"context"
	"errors"
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
	starts_on, starts_with, ends_on, closed_on, closed_with, status, needs_proposal`

func scanCycle(row pgx.Row) (*Cycle, error) {
	var c Cycle
	var closedOn *time.Time
	var closedWith *domain.SlotKind
	d := &c.Delivery
	err := row.Scan(&c.ID, &d.On, &d.From, &d.To, &d.Ordered,
		&c.Start.On, &c.Start.Kind, &c.EndsOn, &closedOn, &closedWith, &c.Status, &c.NeedsProposal)
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

// loadCycleWith loads the Cycle with status upcoming or current, or nil
// when none has it.
func loadCycleWith(ctx context.Context, q db.Querier, status string) (*Cycle, error) {
	return scanCycle(q.QueryRow(ctx, `SELECT `+cycleColumns+` FROM cycle WHERE status = $1`, status))
}

func loadCycle(ctx context.Context, q db.Querier, id int64) (Cycle, error) {
	c, err := scanCycle(q.QueryRow(ctx, `SELECT `+cycleColumns+` FROM cycle WHERE id = $1`, id))
	if c == nil {
		return Cycle{}, cmp.Or(err, pgx.ErrNoRows)
	}
	return *c, err
}

// lockCycles serialises changes to Cycles: opening, moving, dropping and
// arriving.
func lockCycles(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `LOCK TABLE cycle IN EXCLUSIVE MODE`)
	return err
}

// lockCycleOf locks the Cycle of the Plan Proposal with id, if there is one.
func lockCycleOf(ctx context.Context, q db.Querier, id int64) error {
	_, err := q.Exec(ctx, `
		SELECT FROM cycle WHERE id = (SELECT cycle_id FROM plan_proposal WHERE id = $1) FOR UPDATE`, id)
	return err
}

// loadSince loads the first Cycle's first day.
func loadSince(ctx context.Context, q db.Querier) (time.Time, error) {
	var since time.Time
	err := q.QueryRow(ctx, `SELECT min(starts_on) FROM cycle`).Scan(&since)
	return since, err
}

// insertCycle stores c as the upcoming Cycle.
func insertCycle(ctx context.Context, q db.Querier, c domain.Cycle) error {
	d := c.Delivery
	_, err := q.Exec(ctx, `
		INSERT INTO cycle (delivery_on, window_from, window_to, ordered, starts_on, starts_with, ends_on)
		VALUES ($1, `+db.Time("$2")+`, `+db.Time("$3")+`, $4, $5, $6, $7)`,
		d.On, d.From, d.To, d.Ordered, c.Start.On, c.Start.Kind, c.EndsOn)
	return err
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
// new Plan Proposal: it rejects the pending ones with note, and drops the
// Prep Sessions and Slots now outside the Cycle. Its Delivery hasn't
// arrived, so none of its Prep Sessions was prepared.
func moveCycle(ctx context.Context, q db.Querier, id int64, c domain.Cycle, note string) error {
	if err := setNeedsProposal(ctx, q, id, true); err != nil {
		return err
	}
	if err := rejectPending(ctx, q, id, note); err != nil {
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

func setNeedsProposal(ctx context.Context, q db.Querier, id int64, needs bool) error {
	_, err := q.Exec(ctx, `UPDATE cycle SET needs_proposal = $2 WHERE id = $1`, id, needs)
	return err
}

func setStatus(ctx context.Context, q db.Querier, id int64, status string) error {
	_, err := q.Exec(ctx, `UPDATE cycle SET status = $2 WHERE id = $1`, id, status)
	return err
}

// rejectPending rejects the Plan Proposals of the Cycle with id still
// pending, with note.
func rejectPending(ctx context.Context, q db.Querier, id int64, note string) error {
	_, err := q.Exec(ctx, `
		UPDATE plan_proposal SET status = 'rejected', note = $2, decided_at = now()
		WHERE cycle_id = $1 AND status = 'pending'`, id, note)
	return err
}

// closeCycle closes the Cycle with id as the next Delivery's arrival a
// says, rejecting the Plan Proposals still pending, which can no longer
// apply, with note. Closing early, it drops the Prep Sessions never
// prepared and takes the Portions carried over out of their Slots.
func closeCycle(ctx context.Context, q db.Querier, id int64, a domain.Arrival, note string) error {
	var closedOn *time.Time
	var closedWith *domain.SlotKind
	if c := a.Current.ClosedAt; c != nil {
		closedOn, closedWith = &c.On, &c.Kind
	}
	if _, err := q.Exec(ctx, `UPDATE cycle SET status = 'closed', closed_on = $2, closed_with = $3 WHERE id = $1`, id, closedOn, closedWith); err != nil {
		return err
	}
	if err := rejectPending(ctx, q, id, note); err != nil {
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
	for _, ps := range sessions {
		plan.PrepSessions = append(plan.PrepSessions, *ps)
	}
	slices.SortFunc(plan.PrepSessions, func(a, b domain.PrepSession) int {
		switch {
		case a.At.Before(b.At):
			return -1
		case b.At.Before(a.At):
			return 1
		}
		return int(a.ID - b.ID)
	})

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
// are earlier Cycles' Slots, and c's own that a Plan from from on keeps.
func loadEarlier(ctx context.Context, q db.Querier, c Cycle, from domain.Slot) ([]domain.DayNutrition, error) {
	refs, err := loadPortionRefs(ctx, q, `
		SELECT p.id, p.prep_session_id, s.day, s.kind
		FROM slot s JOIN portion p ON p.id = s.portion_id
		WHERE s.day >= $1::date - 6 AND (s.day, `+order("s.kind")+`) < ($2::date, $3)`,
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
	Origin      string
	Status      string
	Note        string
	Plan        []byte
}

// loadProposal loads the Plan Proposal with id, locking it until the
// transaction ends when lock is set. It returns ErrNotFound when there is
// none.
func loadProposal(ctx context.Context, q db.Querier, id int64, lock bool) (storedProposal, error) {
	sql := `SELECT id, cycle_id, origin, status, note, plan FROM plan_proposal WHERE id = $1`
	if lock {
		sql += ` FOR UPDATE`
	}
	var p storedProposal
	err := q.QueryRow(ctx, sql, id).Scan(&p.ID, &p.CycleID, &p.Origin, &p.Status, &p.Note, &p.Plan)
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
