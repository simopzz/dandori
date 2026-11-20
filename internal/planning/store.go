package planning

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
)

// lockPlanning takes the signed-in User's planning lock until the transaction ends, so
// opening, dropping and planning their Cycles each wait their turn. It is an advisory
// lock, needing no row; a hash collision with another User only makes the two wait on
// each other.
func lockPlanning(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('planning'), hashtext(current_user_id()::text))`)
	return err
}

// loadNow reads the signed-in User's current and upcoming Cycles at now, each nil when
// they have none, its dates as days in now's zone, the current one lasting Until the
// upcoming one's Delivery arrives.
func loadNow(ctx context.Context, q db.Querier, now time.Time) (current, upcoming *Cycle, err error) {
	cycles, err := loadCycles(ctx, q, now.Location())
	if err != nil {
		return nil, nil, err
	}
	for i, status := range domain.CycleStatuses(plain(cycles), now) {
		switch status {
		case domain.Current:
			current = &cycles[i]
		case domain.Upcoming:
			upcoming = &cycles[i]
		}
	}
	if current != nil && upcoming != nil {
		current.Until = upcoming.Delivery.Arrives()
	}
	return current, upcoming, nil
}

// loadCycles reads every Cycle of the signed-in User's, in the order of their
// Deliveries, its dates as days in zone. It has no WHERE on purpose: run in db.AsUser,
// row-level security leaves only their rows.
func loadCycles(ctx context.Context, q db.Querier, zone *time.Location) ([]Cycle, error) {
	rows, err := q.Query(ctx, `
		SELECT id, delivery_on, extract(epoch FROM window_from)::int / 60, extract(epoch FROM window_to)::int / 60, ends_on
		FROM cycle ORDER BY delivery_on, window_to`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Cycle, error) {
		var c Cycle
		err := row.Scan(&c.ID, &c.Delivery.On, &c.Delivery.From, &c.Delivery.To, &c.Ends)
		c.Delivery.On, c.Ends = inZone(c.Delivery.On, zone), inZone(c.Ends, zone)
		return c, err
	})
}

// plain is cycles without their ids.
func plain(cycles []Cycle) []domain.Cycle {
	p := make([]domain.Cycle, len(cycles))
	for i, c := range cycles {
		p[i] = c.Cycle
	}
	return p
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
		planningKind, runs.Going, except, Pending).Scan(&busy)
	return busy, err
}

// busyCycles are the ids of the signed-in User's Cycles with a pending Plan Proposal or
// a planning Run going.
func busyCycles(ctx context.Context, q db.Querier) ([]int64, error) {
	rows, _ := q.Query(ctx, `
		SELECT DISTINCT cr.cycle_id FROM cycle_run cr JOIN run r ON r.id = cr.run_id
		LEFT JOIN plan_proposal pp ON pp.run_id = cr.run_id
		WHERE r.status = $1 OR pp.status = $2`, runs.Going, Pending)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// endPending rejects at at the signed-in User's pending Plan Proposal of the Cycle id, if
// any, as ended by cause.
func endPending(ctx context.Context, q db.Querier, id int64, cause []byte, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE plan_proposal pp SET status = $2, decided_at = $3, ended_by = $4
		FROM cycle_run cr WHERE cr.run_id = pp.run_id AND cr.cycle_id = $1 AND pp.status = $5`,
		id, Rejected, at, cause, Pending)
	return err
}

// saveCycleRun records that the Run runID plans the Cycle cycleID from the Slot first.
func saveCycleRun(ctx context.Context, q db.Querier, runID, cycleID int64, first domain.Slot) error {
	_, err := q.Exec(ctx, `INSERT INTO cycle_run (run_id, cycle_id, first_on, first_kind) VALUES ($1, $2, $3, $4)`,
		runID, cycleID, first.On.Format(time.DateOnly), first.Kind)
	return err
}

// goingRun is the id of the signed-in User's planning Run going for the Cycle id, or 0.
func goingRun(ctx context.Context, q db.Querier, id int64) (runID int64, err error) {
	err = q.QueryRow(ctx, `
		SELECT coalesce(max(r.id), 0) FROM cycle_run cr JOIN run r ON r.id = cr.run_id
		WHERE cr.cycle_id = $1 AND r.status = $2`, id, runs.Going).Scan(&runID)
	return runID, err
}

// submit stores plan, summed up by summary, as the pending Plan Proposal of the planning
// Run runID, with the Targets it aimed at, its Verdict v's days and misses, and those of
// the Recipe Drafts drafted that it prepares, as the Run is stored succeeded: a Run
// cancelled meanwhile, by the User, its Cycle dropped or a Delivery taken as arrived,
// never stores it, and the unique index keeps a second one from pending. One stored after
// its Cycle closed, before its Delivery was taken as arrived, is rejected once it is.
func submit(ctx context.Context, q db.Querier, runID int64, plan domain.Plan, targets domain.Targets, v domain.Verdict, summary string, drafted []*recipes.Drafted) error {
	var id int64
	misses := v.Misses
	if misses == nil {
		misses = []domain.Miss{} // stored as [], not null
	}
	err := q.QueryRow(ctx, `
		INSERT INTO plan_proposal (run_id, status, summary, targets, misses, days_before, days) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		runID, Pending, summary, targets, misses, v.Trailing, v.Days).Scan(&id)
	if err != nil {
		return err
	}
	day := func(s *domain.Slot) (*string, *domain.SlotKind) {
		if s == nil {
			return nil, nil
		}
		on := s.On.Format(time.DateOnly)
		return &on, &s.Kind
	}
	saved := map[*domain.Recipe]int64{} // the Recipe Drafts stored, by the Recipe each drafts
	for _, ps := range plan.PrepSessions {
		var recipe, draft, ingredient *int64
		var size, quantity *float64
		switch {
		case ps.Recipe != nil && ps.Recipe.IsDraft():
			if _, ok := saved[ps.Recipe]; !ok {
				i := slices.IndexFunc(drafted, func(d *recipes.Drafted) bool { return &d.Recipe == ps.Recipe })
				if i < 0 {
					return errors.New("a Prep Session prepares a Recipe neither in the Recipe Library nor drafted")
				}
				if saved[ps.Recipe], err = saveDraft(ctx, q, id, drafted[i].Document()); err != nil {
					return err
				}
			}
			d := saved[ps.Recipe]
			draft, size = &d, &ps.Size
		case ps.Recipe != nil:
			recipe, size = &ps.Recipe.ID, &ps.Size
		default:
			q := ps.Quantity.Number()
			ingredient, quantity = &ps.Ingredient.ID, &q
		}
		on, kind := day(&ps.At)
		var sessionID int64
		err := q.QueryRow(ctx, `
			INSERT INTO prep_session (plan_proposal_id, prepared_on, prepared_kind, recipe_id, draft_id, size, ingredient_id, quantity)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			id, on, kind, recipe, draft, size, ingredient, quantity).Scan(&sessionID)
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
	for i, fs := range plan.FreezerSplits {
		_, err := q.Exec(ctx, `INSERT INTO freezer_split (plan_proposal_id, position, ingredient_id, quantity) VALUES ($1, $2, $3, $4)`,
			id, i, fs.Ingredient.ID, fs.Quantity.Number())
		if err != nil {
			return err
		}
	}
	return nil
}

// storedProposal is a Plan Proposal as stored: its first Slot, the Targets it aimed at,
// the days and soft rules it was judged with, and its Prep Sessions and freezer splits
// with their Recipes and Ingredients by id only.
type storedProposal struct {
	id       int64
	summary  string
	first    domain.Slot
	targets  domain.Targets
	misses   []domain.Miss
	trailing []domain.Day
	days     []domain.Day
	sessions []storedSession
	kept     []domain.Kept
	splits   []domain.RecipeIngredient
}

type storedSession struct {
	domain.PrepSession
	recipeID, draftID, ingredientID int64
}

// proposalColumns are what scanProposal reads of a plan_proposal pp and its cycle_run cr.
const proposalColumns = `pp.id, pp.summary, cr.first_on, cr.first_kind, pp.targets, pp.misses, pp.days_before, pp.days`

// scanProposal reads proposalColumns of row, and the Plan they store, its days in zone.
func scanProposal(ctx context.Context, q db.Querier, row pgx.Row, zone *time.Location) (*storedProposal, error) {
	var sp storedProposal
	err := row.Scan(&sp.id, &sp.summary, &sp.first.On, &sp.first.Kind, &sp.targets, &sp.misses, &sp.trailing, &sp.days)
	if err != nil {
		return nil, err
	}
	sp.first.On = inZone(sp.first.On, zone)
	for _, d := range [][]domain.Day{sp.trailing, sp.days} {
		for i := range d {
			d[i].On = inZone(d[i].On, zone)
		}
	}
	return &sp, loadPlan(ctx, q, &sp, zone)
}

// loadProposal reads the signed-in User's Plan Proposal of the Cycle id with status, its
// days in zone, or nil when it has none: the pending one, or the approved one that is
// its Plan.
func loadProposal(ctx context.Context, q db.Querier, id int64, status ProposalStatus, zone *time.Location) (*storedProposal, error) {
	sp, err := scanProposal(ctx, q, q.QueryRow(ctx, `
		SELECT `+proposalColumns+` FROM plan_proposal pp JOIN cycle_run cr ON cr.run_id = pp.run_id
		WHERE cr.cycle_id = $1 AND pp.status = $2`, id, status), zone)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return sp, err
}

// loadApproved reads the signed-in User's approved Plan Proposals, of any Cycle, that put
// a Portion in a Slot on or after the day from, oldest first, their days in zone.
func loadApproved(ctx context.Context, q db.Querier, from time.Time, zone *time.Location) ([]*storedProposal, error) {
	rows, err := q.Query(ctx, `
		SELECT pp.id FROM plan_proposal pp
		WHERE pp.status = $1 AND (
			EXISTS (SELECT 1 FROM prep_session s JOIN planned_portion p ON p.prep_session_id = s.id
				WHERE s.plan_proposal_id = pp.id AND p.eat_on >= $2)
			OR EXISTS (SELECT 1 FROM kept_portion k WHERE k.plan_proposal_id = pp.id AND k.eat_on >= $2))
		ORDER BY pp.id`, Approved, from.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, err
	}
	var approved []*storedProposal
	for _, id := range ids {
		sp, err := scanProposal(ctx, q, q.QueryRow(ctx, `
			SELECT `+proposalColumns+` FROM plan_proposal pp JOIN cycle_run cr ON cr.run_id = pp.run_id WHERE pp.id = $1`, id), zone)
		if err != nil {
			return nil, err
		}
		approved = append(approved, sp)
	}
	return approved, nil
}

// loadPlan reads the Prep Sessions, the Portions without a Slot put in Slots and the
// freezer splits of sp, their days in zone.
func loadPlan(ctx context.Context, q db.Querier, sp *storedProposal, zone *time.Location) error {
	in := func(t time.Time) time.Time { return inZone(t, zone) }
	rows, err := q.Query(ctx, `
		SELECT s.id, s.prepared_on, s.prepared_kind, coalesce(s.recipe_id, 0), coalesce(s.draft_id, 0), coalesce(s.size, 0)::float8,
			coalesce(s.ingredient_id, 0), coalesce(s.quantity, 0)::float8, p.location, p.eat_on, coalesce(p.eat_kind, '')
		FROM prep_session s JOIN planned_portion p ON p.prep_session_id = s.id
		WHERE s.plan_proposal_id = $1 ORDER BY s.id, p.position`, sp.id)
	if err != nil {
		return err
	}
	var last int64
	for rows.Next() {
		var sessionID int64
		var ps storedSession
		var quantity float64
		var pp domain.PlannedPortion
		var eatOn *time.Time
		var eatKind domain.SlotKind
		err := rows.Scan(&sessionID, &ps.At.On, &ps.At.Kind, &ps.recipeID, &ps.draftID, &ps.Size, &ps.ingredientID, &quantity, &pp.Location, &eatOn, &eatKind)
		if err != nil {
			rows.Close()
			return err
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
		return err
	}
	rows, _ = q.Query(ctx, `SELECT portion_id, eat_on, eat_kind FROM kept_portion WHERE plan_proposal_id = $1 ORDER BY eat_on`, sp.id)
	sp.kept, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (k domain.Kept, err error) {
		err = row.Scan(&k.Portion, &k.At.On, &k.At.Kind)
		k.At.On = in(k.At.On)
		return k, err
	})
	if err != nil {
		return err
	}
	rows, _ = q.Query(ctx, `SELECT ingredient_id, quantity::float8 FROM freezer_split WHERE plan_proposal_id = $1 ORDER BY position`, sp.id)
	sp.splits, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (fs domain.RecipeIngredient, err error) {
		var quantity float64
		err = row.Scan(&fs.Ingredient.ID, &quantity)
		fs.Quantity = domain.QuantityOf(quantity)
		return fs, err
	})
	return err
}

// hasPlan reports whether the signed-in User's Cycle id has a Plan, which is the Plan
// Proposal they approved.
func hasPlan(ctx context.Context, q db.Querier, id int64) (planned bool, err error) {
	err = q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM plan_proposal pp JOIN cycle_run cr ON cr.run_id = pp.run_id
			WHERE cr.cycle_id = $1 AND pp.status = $2)`, id, Approved).Scan(&planned)
	return planned, err
}

// pendingProposal is the Cycle and the first Slot, its day in zone, of the signed-in
// User's Plan Proposal id while it is pending, and ErrNoProposal otherwise.
func pendingProposal(ctx context.Context, q db.Querier, id int64, zone *time.Location) (cycleID int64, first domain.Slot, err error) {
	err = q.QueryRow(ctx, `
		SELECT cr.cycle_id, cr.first_on, cr.first_kind FROM plan_proposal pp JOIN cycle_run cr ON cr.run_id = pp.run_id
		WHERE pp.id = $1 AND pp.status = $2`, id, Pending).Scan(&cycleID, &first.On, &first.Kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, first, ErrNoProposal
	}
	first.On = inZone(first.On, zone)
	return cycleID, first, err
}

// Prepares reports whether a Plan Proposal of the signed-in User's, whatever its status,
// prepares the Recipe id, which its Prep Sessions keep. The recipes feature, which this
// one depends on, reads it, and app wires it.
func Prepares(ctx context.Context, q db.Querier, id int64) (prepared bool, err error) {
	err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM prep_session WHERE recipe_id = $1)`, id).Scan(&prepared)
	return prepared, err
}

// UsesIngredient reports whether a Plan Proposal of the signed-in User's, whatever its
// status, uses the Ingredient id, which its Prep Sessions and freezer splits keep. The
// ingredients feature, which this one depends on, reads it, and app wires it.
func UsesIngredient(ctx context.Context, q db.Querier, id int64) (used bool, err error) {
	err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM prep_session WHERE ingredient_id = $1)
		OR EXISTS (SELECT 1 FROM freezer_split WHERE ingredient_id = $1)`, id).Scan(&used)
	return used, err
}

// prepareAccepted has the Prep Sessions of the Recipe Draft draftID prepare the Recipe
// recipeID it was accepted as.
func prepareAccepted(ctx context.Context, q db.Querier, draftID, recipeID int64) error {
	_, err := q.Exec(ctx, `UPDATE prep_session SET recipe_id = $2, draft_id = NULL WHERE draft_id = $1`, draftID, recipeID)
	return err
}

// pendingID is the id of the signed-in User's pending Plan Proposal, or ErrNoProposal.
func pendingID(ctx context.Context, q db.Querier) (id int64, err error) {
	err = q.QueryRow(ctx, `SELECT id FROM plan_proposal WHERE status = $1`, Pending).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoProposal
	}
	return id, err
}

// storedDraft is a planner's Recipe Draft as stored with its Plan Proposal: its id, and
// the Recipe it drafts as draft_recipe took it.
type storedDraft struct {
	id  int64
	doc []byte
}

// saveDraft stores doc as a Recipe Draft of the Plan Proposal proposalID, and is its id.
func saveDraft(ctx context.Context, q db.Querier, proposalID int64, doc []byte) (id int64, err error) {
	err = q.QueryRow(ctx, `INSERT INTO proposal_recipe_draft (plan_proposal_id, document) VALUES ($1, $2) RETURNING id`,
		proposalID, doc).Scan(&id)
	return id, err
}

// loadDrafts are the Recipe Drafts of the signed-in User's Plan Proposal proposalID, in
// the order drafted.
func loadDrafts(ctx context.Context, q db.Querier, proposalID int64) ([]storedDraft, error) {
	rows, _ := q.Query(ctx, `SELECT id, document FROM proposal_recipe_draft WHERE plan_proposal_id = $1 ORDER BY id`, proposalID)
	return pgx.CollectRows(rows, scanDraft)
}

// deleteDrafts deletes the Recipe Drafts of the Plan Proposal proposalID, and is them, in
// the order drafted.
func deleteDrafts(ctx context.Context, q db.Querier, proposalID int64) ([]storedDraft, error) {
	rows, _ := q.Query(ctx, `
		WITH deleted AS (DELETE FROM proposal_recipe_draft WHERE plan_proposal_id = $1 RETURNING id, document)
		SELECT id, document FROM deleted ORDER BY id`, proposalID)
	return pgx.CollectRows(rows, scanDraft)
}

// scanDraft reads a Recipe Draft's id and document.
func scanDraft(row pgx.CollectableRow) (d storedDraft, err error) {
	return d, row.Scan(&d.id, &d.doc)
}

// inZone is the day d in zone, at midnight.
func inZone(d time.Time, zone *time.Location) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, zone)
}

// saveNote records the Planning Note note on the signed-in User's Cycle id, given at
// at, rejecting the Plan Proposal proposalID, if any.
func saveNote(ctx context.Context, q db.Querier, id int64, note string, at time.Time, proposalID *int64) error {
	_, err := q.Exec(ctx, `INSERT INTO planning_note (cycle_id, note, given_at, plan_proposal_id) VALUES ($1, $2, $3, $4)`,
		id, note, at, proposalID)
	return err
}

// loadNotes are the Planning Notes on the signed-in User's Cycle id, newest first.
func loadNotes(ctx context.Context, q db.Querier, id int64) ([]string, error) {
	rows, _ := q.Query(ctx, `SELECT note FROM planning_note WHERE cycle_id = $1 ORDER BY given_at DESC, id DESC`, id)
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// lastRun is a Cycle's latest planning Run: its id, 0 when it has none, its status and
// the first Slot it planned from.
type lastRun struct {
	id     int64
	status runs.Status
	first  domain.Slot
}

// loadLastRun is the signed-in User's Cycle id's latest planning Run, its day in zone.
func loadLastRun(ctx context.Context, q db.Querier, id int64, zone *time.Location) (last lastRun, err error) {
	err = q.QueryRow(ctx, `
		SELECT r.id, r.status, cr.first_on, cr.first_kind FROM cycle_run cr JOIN run r ON r.id = cr.run_id
		WHERE cr.cycle_id = $1 ORDER BY r.id DESC LIMIT 1`, id).Scan(&last.id, &last.status, &last.first.On, &last.first.Kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return lastRun{}, nil
	}
	last.first.On = inZone(last.first.On, zone)
	return last, err
}

// cycleOfRun is the id of the signed-in User's Cycle the planning Run runID plans, and
// ErrNoCycle when it plans none.
func cycleOfRun(ctx context.Context, q db.Querier, runID int64) (id int64, err error) {
	err = q.QueryRow(ctx, `SELECT cycle_id FROM cycle_run WHERE run_id = $1`, runID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoCycle
	}
	return id, err
}

// cycleOfProposal is the id of the signed-in User's Cycle their Plan Proposal id plans,
// and ErrNoProposal when they have no such Plan Proposal.
func cycleOfProposal(ctx context.Context, q db.Querier, id int64) (cycleID int64, err error) {
	err = q.QueryRow(ctx, `SELECT cr.cycle_id FROM plan_proposal pp JOIN cycle_run cr ON cr.run_id = pp.run_id WHERE pp.id = $1`, id).Scan(&cycleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoProposal
	}
	return cycleID, err
}

// decide sets the signed-in User's pending Plan Proposal id to status, decided at at,
// and is ErrNoProposal when it isn't pending.
func decide(ctx context.Context, q db.Querier, id int64, status ProposalStatus, at time.Time) error {
	tag, err := q.Exec(ctx, `UPDATE plan_proposal SET status = $2, decided_at = $3 WHERE id = $1 AND status = $4`, id, status, at, Pending)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNoProposal
	}
	return err
}
