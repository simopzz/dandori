// Package planning is the Plan of each Cycle: opening Cycles, their
// Deliveries and their arrival, and the Plan Proposals that change a
// Cycle's Plan once the user approves them.
package planning

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/profile"
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

// ErrCycleUpcoming means a Cycle is already upcoming, so no other can be
// opened.
var ErrCycleUpcoming = errors.New("a cycle is already upcoming")

// ErrNotUpcoming means the Cycle isn't the upcoming one: its Delivery has
// arrived, or it was dropped.
var ErrNotUpcoming = errors.New("cycle is not upcoming")

// ErrDeliveryOrdered means the Cycle's Delivery was ordered, so it can no
// longer be dropped.
var ErrDeliveryOrdered = errors.New("delivery already ordered")

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
	// NeedsProposal is why its Plan needs a new Plan Proposal, e.g. "A
	// Delivery moved"; empty when it doesn't.
	NeedsProposal string
}

// ProposalSummary is a Plan Proposal in a list.
type ProposalSummary struct {
	ID     int64
	Origin string
	Status string
}

// CyclePlan is a Cycle, its Plan, and its Plan Proposals.
type CyclePlan struct {
	Cycle Cycle
	// Plan is the Cycle's Plan, or nil until a Plan Proposal is approved.
	Plan      *domain.PlanCheck
	Proposals []ProposalSummary
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
}

// Proposal is a Plan Proposal with its Plan checked.
type Proposal struct {
	ID     int64
	Cycle  Cycle
	Origin string
	Status string
	Note   string
	// Unreadable is why its Plan can't be read, if it can't; Check and
	// Violations are then empty.
	Unreadable string
	// Check is the proposed Plan with what it was validated against.
	Check      domain.PlanCheck
	Violations []domain.Violation
}

// Service is the planning use cases.
type Service struct {
	pool    *pgxpool.Pool
	profile *profile.Service
	now     func() time.Time
}

// New creates the Service; Plans are checked against the Profile in
// profile, and Deliveries arrive, at the time now says it is.
func New(pool *pgxpool.Pool, profile *profile.Service, now func() time.Time) *Service {
	return &Service{pool: pool, profile: profile, now: now}
}

func (s *Service) today() time.Time { return domain.DayOf(s.now()) }

// snapshot reads several queries as of one moment.
var snapshot = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// Arrive takes the upcoming Cycle's Delivery as arrived once its window has
// ended: the Cycle becomes current, and the current one closes, early if
// it hadn't ended, carrying what is left over as Portions without a Slot
// and rejecting its pending Plan Proposals. Every use case calls it first,
// so what it shows and changes is as of now.
func (s *Service) Arrive(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
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
			note := "Its Cycle closed: the next Delivery arrived on " + upcoming.Delivery.On.Format(shortDate) + "."
			if err := closeCycle(ctx, tx, current.ID, arrival, note); err != nil {
				return err
			}
		}
		return setStatus(ctx, tx, upcoming.ID, "current")
	})
}

// Overview loads the current and upcoming Cycles with their Plans and Plan
// Proposals.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	if err := s.Arrive(ctx); err != nil {
		return Overview{}, err
	}
	var o Overview
	err := pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		var err error
		if o.Current, err = s.cyclePlan(ctx, tx, "current"); err != nil {
			return err
		}
		if o.Upcoming, err = s.cyclePlan(ctx, tx, "upcoming"); err != nil {
			return err
		}
		o.PlanNext = o.Upcoming == nil && o.Current != nil && o.Current.Cycle.PlanNextDue(s.today())
		return nil
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
// keeping it ordered once it was. A move shifts the Cycle's first Slot,
// and its end date when the Delivery is later; it drops what the Plan has
// outside the moved Cycle, rejects its pending Plan Proposals and marks
// its Plan as needing a new one; so too the current Cycle's Plan, if it no
// longer reaches the Delivery.
func (s *Service) ChangeDelivery(ctx context.Context, id int64, d domain.Delivery) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
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
		d.Ordered = d.Ordered || upcoming.Delivery.Ordered
		moved, err := s.newCycle(ctx, func(times domain.SlotTimes) domain.Cycle { return upcoming.MovedTo(d, times) })
		if err != nil {
			return err
		}
		if d.On.Equal(upcoming.Delivery.On) && d.From == upcoming.Delivery.From && d.To == upcoming.Delivery.To {
			return updateDelivery(ctx, tx, id, moved)
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
		if err := moveCycle(ctx, tx, id, moved, "Its Delivery moved to "+describeDelivery(d, shortDate)+"."); err != nil {
			return err
		}
		if flagCurrent {
			return setNeedsProposal(ctx, tx, current.ID, deliveryMoved)
		}
		return nil
	})
}

// Drop drops the upcoming Cycle with id, while its Delivery is tentative,
// with its Plan and Plan Proposals. Its days become days between Cycles.
func (s *Service) Drop(ctx context.Context, id int64) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
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
	if ch.from, err = ch.proposed.start(ch.cycle); err != nil {
		return ch, err
	}
	plan, err := ch.proposed.resolve(ctx, q)
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
		ch, err := s.checkProposal(ctx, tx, stored)
		p = Proposal{ID: stored.ID, Cycle: ch.cycle, Origin: stored.Origin, Status: stored.Status, Note: stored.Note, Check: ch.check}
		if unreadable := (*UnreadableError)(nil); errors.As(err, &unreadable) {
			p.Unreadable = unreadable.Reason
			return nil
		}
		if err != nil {
			return err
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
// stays: Prep Sessions taken as prepared, and Portions taken as eaten.
func (s *Service) Approve(ctx context.Context, id int64) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialises approvals of one Cycle with each other and with the
		// Delivery that closes it. The Cycle is locked before the Plan
		// Proposal, in the order Arrive locks them, or the two deadlock.
		if err := lockCycleOf(ctx, tx, id); err != nil {
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
		if slices.ContainsFunc(domain.ValidatePlan(ch.check), func(v domain.Violation) bool { return v.Rule == domain.RuleStartsInPast }) {
			return ErrStartsInPast
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
		return decideProposal(ctx, tx, id, "approved", "")
	})
}

// Reject rejects the Plan Proposal with id, with the user's note.
func (s *Service) Reject(ctx context.Context, id int64, note string) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := pending(ctx, tx, id); err != nil {
			return err
		}
		return decideProposal(ctx, tx, id, "rejected", note)
	})
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
