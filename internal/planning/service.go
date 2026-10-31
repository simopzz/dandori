// Package planning is the Plan of each Cycle: opening Cycles, their
// Deliveries and their arrival, and the Plan Proposals that change a
// Cycle's Plan once the user approves them.
package planning

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/catalog"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/profile"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
	"github.com/simopzz/dandori/internal/stock"
)

// ErrNotFound means there is no Plan Proposal with that id.
var ErrNotFound = errors.New("no such plan proposal")

// ErrDecided means the Plan Proposal was already approved or rejected.
var ErrDecided = errors.New("plan proposal already decided")

// ErrCycleClosed means the Plan Proposal's Cycle was closed by the next
// Delivery, so its Plan can no longer change. Closing rejects the Cycle's
// pending Plan Proposals, so only one stored after it meets this.
var ErrCycleClosed = errors.New("plan proposal's cycle has closed")

// ErrStartsInPast means the Plan Proposal would replace the Plan from a
// day already past.
var ErrStartsInPast = errors.New("plan proposal starts in the past")

// ErrInapplicable means a Plan Proposal no longer fits what is kept: it
// fills a Slot twice, or puts a Portion in a Slot that is gone or already
// in one.
var ErrInapplicable = errors.New("plan proposal does not apply")

// ErrFillsExceptedSlot means a Plan Proposal puts a Portion or a Prep
// Session in a Slot logged as a Meal Out or skipped since it was made.
var ErrFillsExceptedSlot = errors.New("plan proposal fills an excepted slot")

// ErrCycleUpcoming means a Cycle is already upcoming, so no other can be
// opened.
var ErrCycleUpcoming = errors.New("a cycle is already upcoming")

// ErrNotUpcoming means the Cycle isn't the upcoming one: its Delivery has
// arrived, or it was dropped.
var ErrNotUpcoming = errors.New("cycle is not upcoming")

// ErrDeliveryOrdered means the Cycle's Delivery was ordered, so it can no
// longer be dropped.
var ErrDeliveryOrdered = errors.New("delivery already ordered")

// ErrNoPlanToOrder means the Cycle has no Plan yet, so its Delivery can't
// be marked ordered: ordering fixes the Grocery List the Plan makes.
var ErrNoPlanToOrder = errors.New("cycle has no plan to order for")

// InvalidError means a Cycle breaks the domain rules. Fields is what is
// wrong, keyed by field name as domain.Cycle.Validate gives it.
type InvalidError struct{ Fields map[string]string }

func (e *InvalidError) Error() string { return "invalid cycle" }

// OverlapError means a new Cycle would be delivered on or before the
// current Cycle's delivery date, Current.
type OverlapError struct{ Current time.Time }

func (e *OverlapError) Error() string { return domain.ErrCycleOverlaps.Error() }

// Cycle is a stored Cycle.
type Cycle struct {
	ID int64
	domain.Cycle
	// Status is upcoming, current or closed.
	Status string
	// NeedsProposalBecause is why its Plan needs a new Plan Proposal,
	// e.g. "A Delivery moved"; empty when it doesn't.
	NeedsProposalBecause string
}

// ProposalSummary is a Plan Proposal in a list.
type ProposalSummary struct {
	ID     int64
	Origin string
	Status string
}

// CyclePlan is a Cycle, its Plan, its Plan Proposals, and how planning
// it stands.
type CyclePlan struct {
	Cycle Cycle
	// Plan is the Cycle's Plan, or nil until a Plan Proposal is approved.
	Plan      *domain.PlanCheck
	Proposals []ProposalSummary
	Planning  Planning
}

// Overview is the current Cycle and the upcoming one.
type Overview struct {
	// Current is the current Cycle, or nil before the first Delivery
	// arrives.
	Current *CyclePlan
	// Upcoming is the upcoming Cycle, or nil when none is.
	Upcoming *CyclePlan
	// PlanNext is whether it is time to plan the Cycle after the current
	// one: none is upcoming, and the current one ends within days.
	PlanNext bool
	// Previous is the Cycle the current one closed, or nil when none did.
	Previous *Cycle
	// Between is the Exceptions on days between Cycles that a trailing
	// seven days still reaches.
	Between []ExceptionSlot
	// MealOut is the Profile's default Meal Out estimate.
	MealOut domain.Nutrition
}

// Proposal is a Plan Proposal with its Plan checked.
type Proposal struct {
	ID     int64
	Cycle  Cycle
	Origin string
	Status string
	Note   string
	// Cause is why it was replaced, or rejected without the user: what
	// happened, never a Planning Note.
	Cause string
	// Unreadable is why its Plan can't be read, if it can't; Check and
	// Violations are then empty.
	Unreadable string
	// Check is the proposed Plan with what it was validated against.
	Check      domain.PlanCheck
	Violations []domain.Violation
	// Drafts are the Recipe Drafts it prepares.
	Drafts []recipes.Drafted
	// RunID is the planning Run that submitted it; ReplanID the one its
	// rejection started. Either is 0 when there is none.
	RunID, ReplanID int64
}

// Service is the planning use cases.
type Service struct {
	pool     *pgxpool.Pool
	profile  *profile.Service
	recipes  *recipes.Service
	stock    *stock.Service
	catalog  *catalog.Service
	runs     *runs.Engine
	language domain.ContentLanguage
	attempts int
	now      func() time.Time
}

// Config is what planning needs besides its database.
type Config struct {
	// Profile is what Plans are checked against.
	Profile *profile.Service
	// Recipes, Stock and Catalog are what the planner plans with: the
	// Recipe Library and the Recipe Drafts it adds, the Stock Items, and the
	// Known Catalog.
	Recipes *recipes.Service
	Stock   *stock.Service
	Catalog *catalog.Service
	// Runs is where planning Runs go through.
	Runs *runs.Engine
	// Language is the Content Language food terms are written in.
	Language domain.ContentLanguage
	// PlanAttempts is how many Plans breaking the rules a planning Run may
	// submit; the last one fails it.
	PlanAttempts int
	// Now is the clock: Deliveries arrive, and the Plan is taken to happen,
	// at the time it says.
	Now func() time.Time
}

// New creates the Service.
func New(pool *pgxpool.Pool, cfg Config) *Service {
	return &Service{
		pool: pool, profile: cfg.Profile, recipes: cfg.Recipes, stock: cfg.Stock, catalog: cfg.Catalog,
		runs: cfg.Runs, language: cfg.Language, attempts: cfg.PlanAttempts, now: cfg.Now,
	}
}

func (s *Service) today() time.Time { return domain.DayOf(s.now()) }

// snapshot reads several queries as of one moment.
var snapshot = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// cancels are the planning Runs a transaction recorded cancelled, to stop
// once it commits.
type cancels []int64

// write runs f in a transaction, and stops the planning Runs f cancelled
// once it has committed.
func (s *Service) write(ctx context.Context, f func(tx pgx.Tx, c *cancels) error) error {
	var c cancels
	if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return f(tx, &c) }); err != nil {
		return err
	}
	for _, id := range c {
		s.runs.Cancel(id)
	}
	return nil
}

// replace replaces the Plan Proposal pending for any of the Cycles with
// ids, and cancels the planning Run going for any of them, because of
// cause, what happened, as part of q: the next planning Run reads what
// happened for itself, so cause is never a Planning Note. Whatever would
// lead to a new Plan Proposal replaces through it.
func (s *Service) replace(ctx context.Context, q db.Querier, c *cancels, ids []int64, cause string) error {
	if err := decidePending(ctx, q, ids, "replaced", cause); err != nil {
		return err
	}
	return s.cancelPlanning(ctx, q, c, ids, cause)
}

// cancelPlanning cancels the planning Run going for any of the Cycles with
// ids because of cause, as part of q, adding it to c. One that has stored
// its Plan Proposal is left to end: that Plan Proposal is what is
// replaced.
func (s *Service) cancelPlanning(ctx context.Context, q db.Querier, c *cancels, ids []int64, cause string) error {
	going, err := planningRunsGoing(ctx, q, ids)
	if err != nil {
		return err
	}
	if err := s.runs.CancelIn(ctx, q, going, cause); err != nil {
		return err
	}
	*c = append(*c, going...)
	return nil
}

// Arrive takes the upcoming Cycle's Delivery as arrived once its window has
// ended: the Cycle becomes current, everything on its Grocery List becomes
// Stock Items, and the current one closes, early if it hadn't ended,
// carrying what is left over as Portions without a Slot and rejecting its
// pending Plan Proposals. Then every Prep Session whose day has come uses
// up the Stock Items of its Ingredients, once. Every use case calls it
// first, so what it shows and changes is as of now.
func (s *Service) Arrive(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.arrive(ctx, tx); err != nil {
			return err
		}
		return s.useUp(ctx, tx)
	})
}

// arrive takes the upcoming Cycle's Delivery as arrived through tx, once
// its window has ended. The Prep Sessions of the Cycle it closes whose day
// has come use up their stock before what it brings is received.
func (s *Service) arrive(ctx context.Context, tx pgx.Tx) error {
	upcoming, err := loadCycleWith(ctx, tx, "upcoming")
	if err != nil || upcoming == nil || !upcoming.Arrived(s.now()) {
		return err
	}
	// Serialises arrivals with each other and with changes to Cycles.
	if err := lockCycles(ctx, tx); err != nil {
		return err
	}
	if upcoming, err = loadCycleWith(ctx, tx, "upcoming"); err != nil || upcoming == nil {
		return err
	}
	current, err := loadCycleWith(ctx, tx, "current")
	if err != nil {
		return err
	}
	if current != nil {
		plan, _, err := loadPlan(ctx, tx, current.ID)
		if err != nil {
			return err
		}
		arrival := domain.Arrive(&current.Cycle, plan, upcoming.Cycle)
		cause := "Its Cycle closed when the Delivery on " + upcoming.Delivery.On.Format(shortDate) + " arrived"
		if err := closeCycle(ctx, tx, current.ID, arrival, cause); err != nil {
			return err
		}
		if err := s.useUp(ctx, tx); err != nil {
			return err
		}
	}
	list, err := s.fixGroceryList(ctx, tx, *upcoming)
	if err != nil {
		return err
	}
	if err := s.stock.Receive(ctx, tx, list.StockItems()); err != nil {
		return err
	}
	return setStatus(ctx, tx, upcoming.ID, "current")
}

// useUp has every Prep Session outside the upcoming Cycle whose day has
// come, and hasn't used up its stock yet, use up the Stock Items of its
// Ingredients, as part of q.
func (s *Service) useUp(ctx context.Context, q db.Querier) error {
	ids, err := claimUseUp(ctx, q, s.today())
	if err != nil || len(ids) == 0 {
		return err
	}
	sessions, err := loadPrepSessions(ctx, q, ids)
	if err != nil {
		return err
	}
	return s.stock.UseUp(ctx, q, inDayOrder(sessions))
}

// inDayOrder is sessions in the order of the Slots they are prepared at.
func inDayOrder(sessions map[int64]*domain.PrepSession) []domain.PrepSession {
	var list []domain.PrepSession
	for _, ps := range sessions {
		list = append(list, *ps)
	}
	slices.SortFunc(list, func(a, b domain.PrepSession) int {
		switch {
		case a.At.Before(b.At):
			return -1
		case b.At.Before(a.At):
			return 1
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return list
}

// Overview loads the current and upcoming Cycles with their Plans and Plan
// Proposals.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	if err := s.Arrive(ctx); err != nil {
		return Overview{}, err
	}
	p, _, err := s.profile.Get(ctx)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{MealOut: p.MealOut}
	err = pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		var err error
		if o.Current, err = s.cyclePlan(ctx, tx, "current"); err != nil {
			return err
		}
		if o.Upcoming, err = s.cyclePlan(ctx, tx, "upcoming"); err != nil {
			return err
		}
		o.PlanNext = o.Upcoming == nil && o.Current != nil && o.Current.Cycle.PlanNextDue(s.today())
		first := s.today().AddDate(0, 0, -6)
		if o.Current != nil && o.Current.Cycle.Start.On.AddDate(0, 0, -6).Before(first) {
			first = o.Current.Cycle.Start.On.AddDate(0, 0, -6)
		}
		if o.Between, err = s.between(ctx, tx, first); err != nil {
			return err
		}
		o.Previous, err = loadPreviousCycle(ctx, tx)
		return err
	})
	return o, err
}

// cyclePlan loads the Cycle with status, its Plan and Plan Proposals, or
// nil when no Cycle has it.
func (s *Service) cyclePlan(ctx context.Context, tx pgx.Tx, status string) (*CyclePlan, error) {
	c, err := loadCycleWith(ctx, tx, status)
	if err != nil || c == nil {
		return nil, err
	}
	cp := &CyclePlan{Cycle: *c}
	if cp.Proposals, err = loadProposals(ctx, tx, c.ID); err != nil {
		return nil, err
	}
	if cp.Planning, err = s.planning(ctx, tx, *c); err != nil {
		return nil, err
	}
	approved, err := hasPlan(ctx, tx, c.ID)
	if err != nil || !approved {
		return cp, err
	}
	plan, kept, err := loadPlan(ctx, tx, c.ID)
	if err != nil {
		return nil, err
	}
	check, err := s.check(ctx, tx, *c, c.Start, plan, kept)
	cp.Plan = &check
	return cp, err
}

// check gathers what a Plan of c from the Slot from on, putting the
// Portions kept in Slots, is validated against.
func (s *Service) check(ctx context.Context, q db.Querier, c Cycle, from domain.Slot, plan domain.Plan, kept []domain.KeptPortion) (domain.PlanCheck, error) {
	p, _, err := s.profile.Get(ctx)
	if err != nil {
		return domain.PlanCheck{}, err
	}
	check := domain.PlanCheck{Cycle: c.Cycle, Plan: plan, From: from, Today: s.today(), Profile: p, Kept: kept}
	if check.Earlier, err = loadEarlier(ctx, q, c, from); err != nil {
		return check, err
	}
	if check.Exceptions, err = loadExceptions(ctx, q, c.Start.On.AddDate(0, 0, -6), c.LastDay()); err != nil {
		return check, err
	}
	check.Since, err = loadSince(ctx, q)
	return check, err
}

// newCycle is the Cycle build makes with the Profile's Slot times, which
// place its first Slot after the window.
func (s *Service) newCycle(ctx context.Context, build func(domain.SlotTimes) domain.Cycle) (domain.Cycle, error) {
	p, _, err := s.profile.Get(ctx)
	c := build(p.SlotTimes)
	if invalid := c.Validate(); err == nil && len(invalid) > 0 {
		err = &InvalidError{Fields: invalid}
	}
	return c, err
}

// OpenCycle opens the Cycle delivered by d and planned to end on endsOn as
// the upcoming Cycle, returning its id. The current Cycle is left as it is
// until the Delivery arrives.
func (s *Service) OpenCycle(ctx context.Context, d domain.Delivery, endsOn time.Time) (int64, error) {
	if err := s.Arrive(ctx); err != nil {
		return 0, err
	}
	c, err := s.newCycle(ctx, func(times domain.SlotTimes) domain.Cycle { return domain.NewCycle(d, endsOn, times) })
	if err != nil {
		return 0, err
	}
	var id int64
	return id, pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		latest, err := loadLatestCycle(ctx, tx)
		if err != nil {
			return err
		}
		var latestCycle *domain.Cycle
		if latest != nil {
			latestCycle = &latest.Cycle
		}
		switch err := domain.OpenCycle(latestCycle, s.now(), c); {
		case errors.Is(err, domain.ErrCycleUpcoming):
			return ErrCycleUpcoming
		case errors.Is(err, domain.ErrCycleOverlaps):
			return &OverlapError{Current: latest.Delivery.On}
		case err != nil:
			return err
		}
		id, err = insertCycle(ctx, tx, c)
		return err
	})
}

// ChangeDelivery changes the Delivery of the upcoming Cycle with id to d,
// keeping it ordered once it was; marking it ordered, once the Cycle has
// a Plan, fixes its Grocery List. A move shifts the Cycle's first Slot,
// and its end date when the Delivery is later; it drops what the Plan has
// outside the moved Cycle and marks its Plan as needing a new Plan
// Proposal; so too the current Cycle's Plan, if it no longer reaches the
// Delivery. It replaces the pending Plan Proposal and cancels the planning
// Run going, of either Cycle.
func (s *Service) ChangeDelivery(ctx context.Context, id int64, d domain.Delivery) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return s.write(ctx, func(tx pgx.Tx, cancelled *cancels) error {
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		upcoming, err := loadCycleWith(ctx, tx, "upcoming")
		if err != nil {
			return err
		}
		if upcoming == nil || upcoming.ID != id {
			return ErrNotUpcoming
		}
		ordering := d.Ordered && !upcoming.Delivery.Ordered
		if ordering {
			planned, err := hasPlan(ctx, tx, id)
			if err != nil {
				return err
			}
			if !planned {
				return ErrNoPlanToOrder
			}
		}
		d.Ordered = d.Ordered || upcoming.Delivery.Ordered
		moved, err := s.newCycle(ctx, func(times domain.SlotTimes) domain.Cycle { return upcoming.MovedTo(d, times) })
		if err != nil {
			return err
		}
		// Ordering fixes the Grocery List of the Cycle as it is moved.
		fixGroceries := func() error {
			if !ordering {
				return nil
			}
			_, err := s.fixGroceryList(ctx, tx, Cycle{ID: id, Cycle: moved})
			return err
		}
		if d.On.Equal(upcoming.Delivery.On) && d.From == upcoming.Delivery.From && d.To == upcoming.Delivery.To {
			if err := updateDelivery(ctx, tx, id, moved); err != nil {
				return err
			}
			return fixGroceries()
		}
		current, err := loadCycleWith(ctx, tx, "current")
		if err != nil {
			return err
		}
		var currentCycle *domain.Cycle
		if current != nil {
			currentCycle = &current.Cycle
		}
		flagCurrent, err := domain.MoveDelivery(currentCycle, upcoming.Cycle, moved)
		if errors.Is(err, domain.ErrCycleOverlaps) {
			return &OverlapError{Current: current.Delivery.On}
		}
		if err != nil {
			return err
		}
		if err := updateDelivery(ctx, tx, id, moved); err != nil {
			return err
		}
		if err := moveCycle(ctx, tx, id, moved); err != nil {
			return err
		}
		open, err := openCycleIDs(ctx, tx)
		if err != nil {
			return err
		}
		if err := s.replace(ctx, tx, cancelled, open, "Replaced by the Delivery moved to "+describeDelivery(d, shortDate)); err != nil {
			return err
		}
		if err := fixGroceries(); err != nil {
			return err
		}
		if flagCurrent {
			return setNeedsProposal(ctx, tx, current.ID, deliveryMoved)
		}
		return nil
	})
}

// Drop drops the upcoming Cycle with id, while its Delivery is tentative,
// with its Plan and Plan Proposals, cancelling its planning Run going. Its
// days become days between Cycles.
func (s *Service) Drop(ctx context.Context, id int64) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return s.write(ctx, func(tx pgx.Tx, cancelled *cancels) error {
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		upcoming, err := loadCycleWith(ctx, tx, "upcoming")
		if err != nil {
			return err
		}
		if upcoming == nil || upcoming.ID != id {
			return ErrNotUpcoming
		}
		if errors.Is(upcoming.Drop(), domain.ErrDeliveryOrdered) {
			return ErrDeliveryOrdered
		}
		if err := s.cancelPlanning(ctx, tx, cancelled, []int64{id}, "Its Cycle was dropped"); err != nil {
			return err
		}
		return deleteCycle(ctx, tx, id)
	})
}

// checked is a stored Plan Proposal read and checked.
type checked struct {
	cycle    Cycle
	proposed proposedPlan
	// from is the first Slot it replaces, prepared the first Prep Session.
	from, prepared domain.Slot
	check          domain.PlanCheck
}

// checkProposal reads stored's Plan, with its Recipes and Ingredients, and
// gathers what it is validated against.
func (s *Service) checkProposal(ctx context.Context, q db.Querier, stored storedProposal) (checked, error) {
	var ch checked
	var err error
	if ch.cycle, err = loadCycle(ctx, q, stored.CycleID); err != nil {
		return ch, err
	}
	if ch.proposed, err = readPlan(stored.Plan); err != nil {
		return ch, err
	}
	ch.proposed.run = stored.RunID
	if ch.from, err = ch.proposed.start(ch.cycle); err != nil {
		return ch, err
	}
	plan, err := s.resolve(ctx, q, ch.proposed)
	if err != nil {
		return ch, err
	}
	ch.prepared = domain.FirstUnprepared(ch.from, s.today())
	kept, err := loadKept(ctx, q, ch.cycle.ID, ch.from, ch.prepared)
	if err != nil {
		return ch, err
	}
	ch.check, err = s.check(ctx, q, ch.cycle, ch.from, plan, kept)
	return ch, err
}

// Proposal loads the Plan Proposal with id and checks its Plan.
func (s *Service) Proposal(ctx context.Context, id int64) (Proposal, error) {
	var p Proposal
	if err := s.Arrive(ctx); err != nil {
		return p, err
	}
	err := pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		stored, err := loadProposal(ctx, tx, id, false)
		if err != nil {
			return err
		}
		ch, checkErr := s.checkProposal(ctx, tx, stored)
		p = Proposal{ID: stored.ID, Cycle: ch.cycle, Origin: stored.Origin, Status: stored.Status, Note: stored.Note, Cause: stored.Cause, Check: ch.check, RunID: stored.RunID}
		if p.ReplanID, err = planningRunAfter(ctx, tx, id); err != nil {
			return err
		}
		if p.Drafts, err = s.drafts(ctx, tx, ch.proposed); err != nil {
			return err
		}
		if unreadable := (*UnreadableError)(nil); errors.As(checkErr, &unreadable) {
			p.Unreadable = unreadable.Reason
			return nil
		}
		if checkErr != nil {
			return checkErr
		}
		// A decided Plan Proposal's rules were for the day it was decided.
		if stored.Status == "pending" {
			p.Violations = domain.ValidatePlan(p.Check)
		}
		return nil
	})
	return p, err
}

// Approve makes the Plan Proposal with id its Cycle's Plan from the Slot
// it starts at, all at once or not at all. What the Plan has before that
// stays: Prep Sessions taken as prepared, and Portions taken as eaten. A
// current Plan approved that changes what will carry over leaves the
// upcoming Cycle's Plan needing a new Plan Proposal.
func (s *Service) Approve(ctx context.Context, id int64) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialises approvals with each other, with the Delivery that
		// closes the Cycle and with what replaces Plan Proposals. The
		// Cycles are locked before the Plan Proposal, in the order Arrive
		// locks them, or the two deadlock.
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		stored, err := pending(ctx, tx, id)
		if err != nil {
			return err
		}
		ch, err := s.checkProposal(ctx, tx, stored)
		if err != nil {
			return err
		}
		// Its days and what is left of them now belong to the next Cycle.
		if ch.cycle.Status == "closed" {
			return ErrCycleClosed
		}
		// What was prepared and eaten since its start can't be replaced.
		violations := domain.ValidatePlan(ch.check)
		breaks := func(rule domain.Rule) bool {
			return slices.ContainsFunc(violations, func(v domain.Violation) bool { return v.Rule == rule })
		}
		if breaks(domain.RuleStartsInPast) {
			return ErrStartsInPast
		}
		// It would eat what the user won't, or cook for nobody.
		if breaks(domain.RuleExceptedSlot) {
			return ErrFillsExceptedSlot
		}
		// Approving it accepts the Recipe Drafts it prepares, as drafted.
		if err := s.acceptDrafts(ctx, tx, &ch.proposed); err != nil {
			return err
		}
		// What the current Plan carried over before, to compare with after.
		var before domain.Plan
		var keptBefore []domain.KeptPortion
		if ch.cycle.Status == "current" {
			if before, keptBefore, err = loadPlan(ctx, tx, ch.cycle.ID); err != nil {
				return err
			}
		}
		err = replacePlan(ctx, tx, ch.cycle, ch.from, ch.prepared, ch.proposed)
		// 23503 and 23505: a Slot for a Portion that is gone, or a Slot or
		// Portion taken twice.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23505") {
			return ErrInapplicable
		}
		if err != nil {
			return err
		}
		if err := setNeedsProposal(ctx, tx, ch.cycle.ID, ""); err != nil {
			return err
		}
		if ch.cycle.Status == "current" {
			if err := flagCarryOver(ctx, tx, ch.cycle, before, keptBefore); err != nil {
				return err
			}
		}
		return decideProposal(ctx, tx, id, "approved", "")
	})
}

// flagCarryOver marks the upcoming Cycle's Plan as needing a new Plan
// Proposal when the Plan of current, the current Cycle, changed from
// before, with the Portions without a Slot kept, to what is stored now in
// what the upcoming Delivery's arrival would carry over. An upcoming Cycle
// without a Plan is planned against what carries over then.
func flagCarryOver(ctx context.Context, q db.Querier, current Cycle, before domain.Plan, kept []domain.KeptPortion) error {
	upcoming, err := loadCycleWith(ctx, q, "upcoming")
	if err != nil || upcoming == nil {
		return err
	}
	if planned, err := hasPlan(ctx, q, upcoming.ID); err != nil || !planned {
		return err
	}
	after, keptAfter, err := loadPlan(ctx, q, current.ID)
	if err != nil {
		return err
	}
	if !domain.CarryOverChanged(current.Cycle, before, after, append(kept, keptAfter...), upcoming.Cycle) {
		return nil
	}
	return setNeedsProposal(ctx, q, upcoming.ID, carryOverChanged)
}

// drafts loads the Recipe Drafts p prepares, in order, each accepted one
// as the Recipe it became; a discarded one is left out.
func (s *Service) drafts(ctx context.Context, q db.Querier, p proposedPlan) ([]recipes.Drafted, error) {
	ids := p.draftIDs()
	drafted, err := s.ownDrafts(ctx, q, p)
	if err != nil {
		return nil, err
	}
	var recipeIDs []int64
	for _, d := range drafted {
		recipeIDs = append(recipeIDs, d.RecipeID)
	}
	accepted, err := loadRecipes(ctx, q, recipeIDs)
	if err != nil {
		return nil, err
	}
	var drafts []recipes.Drafted
	for _, id := range ids {
		d, ok := drafted[id]
		if !ok {
			continue
		}
		if r := accepted[d.RecipeID]; r != nil {
			d.Recipe = *r
		}
		drafts = append(drafts, d)
	}
	return drafts, nil
}

// acceptDrafts adds the Recipe Drafts p prepares still pending to the
// Recipe Library, and has p prepare the Recipes they became.
func (s *Service) acceptDrafts(ctx context.Context, q db.Querier, p *proposedPlan) error {
	ids := p.draftIDs()
	if len(ids) == 0 {
		return nil
	}
	recipeOf, err := s.recipes.AcceptDrafted(ctx, q, ids)
	if err != nil {
		return err
	}
	for i := range p.PrepSessions {
		if ps := &p.PrepSessions[i]; ps.DraftID != nil {
			id := recipeOf[*ps.DraftID]
			ps.RecipeID, ps.DraftID = &id, nil
		}
	}
	return nil
}

// Reject rejects the Plan Proposal with id, with the user's note. A note
// asks for another: while the Cycle has no Plan, a new planning Run that
// takes it into account starts with the rejection, all at once or not at
// all, and its id is returned; 0 when none started. waits is whether none
// started because another Cycle's turn comes first.
func (s *Service) Reject(ctx context.Context, id int64, note string) (run int64, waits bool, err error) {
	if err := s.Arrive(ctx); err != nil {
		return 0, false, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// The Cycles are locked before the Plan Proposal, as Approve does.
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		stored, err := pending(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := decideProposal(ctx, tx, id, "rejected", note); err != nil || note == "" {
			return err
		}
		run, err = s.startPlanningIn(ctx, tx, stored.CycleID, id)
		if errors.Is(err, ErrCannotPlan) {
			run, waits, err = 0, waitsTurn(err) != "", nil
		}
		return err
	})
	return run, waits, err
}

// WaitsFor is why the Cycle with id can't be planned now because another
// Cycle's turn comes first; empty when it can, or can't for its own
// reasons.
func (s *Service) WaitsFor(ctx context.Context, id int64) (string, error) {
	c, err := loadCycle(ctx, s.pool, id)
	if err != nil {
		return "", err
	}
	_, waits, err := s.canPlan(ctx, s.pool, c)
	return waits, err
}

// waitsTurn is why err refuses planning when another Cycle's turn comes
// first; empty when it refuses for the Cycle's own reasons.
func waitsTurn(err error) string {
	if cannot := (*CannotPlanError)(nil); errors.As(err, &cannot) && cannot.WaitsTurn {
		return cannot.Reason
	}
	return ""
}

// pending loads the Plan Proposal with id, locked, if it is still to be
// decided.
func pending(ctx context.Context, tx pgx.Tx, id int64) (storedProposal, error) {
	stored, err := loadProposal(ctx, tx, id, true)
	if err == nil && stored.Status != "pending" {
		err = ErrDecided
	}
	return stored, err
}
