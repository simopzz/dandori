// Package planning is the Plan of each Cycle: opening Cycles, and the Plan
// Proposals that change a Cycle's Plan once the user approves them.
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

// ErrCycleClosed means the Plan Proposal's Cycle was closed early by the
// next delivery, so its Plan can no longer change. Closing rejects the
// Cycle's pending Plan Proposals, so only one stored after it meets this.
var ErrCycleClosed = errors.New("plan proposal's cycle has closed")

// ErrStartsInPast means the Plan Proposal would replace the Plan from a
// day already past.
var ErrStartsInPast = errors.New("plan proposal starts in the past")

// ErrInapplicable means a Plan Proposal no longer fits what is kept: it
// fills a Slot twice, or puts a Portion in a Slot that is gone or already
// in one.
var ErrInapplicable = errors.New("plan proposal does not apply")

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
}

func (c Cycle) inCycle(d time.Time) bool {
	return !d.Before(c.DeliveryOn) && !d.After(c.LastDay())
}

// firstSlot is the Cycle's first eating occasion.
func (c Cycle) firstSlot() domain.Slot {
	return domain.Slot{On: c.DeliveryOn, Kind: domain.SlotKinds[0]}
}

// ProposalSummary is a Plan Proposal in a list.
type ProposalSummary struct {
	ID     int64
	Origin string
	Status string
}

// Overview is the current Cycle, its Plan, and its Plan Proposals.
type Overview struct {
	// Cycle is the current Cycle, or nil before the first one opens.
	Cycle *Cycle
	// Plan is the Cycle's Plan, or nil until a Plan Proposal is approved.
	Plan      *domain.PlanCheck
	Proposals []ProposalSummary
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
	today   func() time.Time
}

// New creates the Service; Plans are checked against the Profile in
// profile, on the day today says it is.
func New(pool *pgxpool.Pool, profile *profile.Service, today func() time.Time) *Service {
	return &Service{pool: pool, profile: profile, today: today}
}

// snapshot reads several queries as of one moment.
var snapshot = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// Overview loads the current Cycle with its Plan and Plan Proposals.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	err := pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		var err error
		if o.Cycle, err = loadLatestCycle(ctx, tx); err != nil || o.Cycle == nil {
			return err
		}
		if o.Proposals, err = loadProposals(ctx, tx, o.Cycle.ID); err != nil {
			return err
		}
		approved, err := hasPlan(ctx, tx, o.Cycle.ID)
		if err != nil || !approved {
			return err
		}
		plan, kept, err := loadPlan(ctx, tx, o.Cycle.ID)
		if err != nil {
			return err
		}
		check, err := s.check(ctx, tx, *o.Cycle, o.Cycle.firstSlot(), plan, kept)
		o.Plan = &check
		return err
	})
	return o, err
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
	check.Since, err = loadSince(ctx, q)
	return check, err
}

// OpenCycle opens c as the current Cycle. A delivery before the current
// Cycle ends closes it early, carrying what is left over as Portions
// without a Slot and rejecting its pending Plan Proposals.
func (s *Service) OpenCycle(ctx context.Context, c domain.Cycle) error {
	if invalid := c.Validate(); len(invalid) > 0 {
		return &InvalidError{Fields: invalid}
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialises opening Cycles, so two can't both close the current one.
		if _, err := tx.Exec(ctx, `LOCK TABLE cycle IN EXCLUSIVE MODE`); err != nil {
			return err
		}
		current, err := loadLatestCycle(ctx, tx)
		if err != nil {
			return err
		}
		var currentCycle *domain.Cycle
		var plan domain.Plan
		if current != nil {
			currentCycle = &current.Cycle
			if plan, _, err = loadPlan(ctx, tx, current.ID); err != nil {
				return err
			}
		}
		opening, err := domain.OpenCycle(currentCycle, plan, c)
		if errors.Is(err, domain.ErrCycleOverlaps) {
			return &OverlapError{Current: current.DeliveryOn}
		}
		if err != nil {
			return err
		}
		if opening.ClosesCurrent {
			note := "Its Cycle closed early: the next delivery arrived on " + c.DeliveryOn.Format(shortDate) + "."
			if err := closeCycle(ctx, tx, current.ID, c.DeliveryOn, opening, note); err != nil {
				return err
			}
		}
		return insertCycle(ctx, tx, c)
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
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialises approvals of one Cycle with each other and with the
		// delivery that closes it. The Cycle is locked before the Plan
		// Proposal, in the order OpenCycle locks them, or the two deadlock.
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
		if ch.cycle.ClosedOn != nil {
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
		return decideProposal(ctx, tx, id, "approved", "")
	})
}

// Reject rejects the Plan Proposal with id, with the user's note.
func (s *Service) Reject(ctx context.Context, id int64, note string) error {
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
