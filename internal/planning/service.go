// Package planning is the Plan of each Cycle: opening Cycles, and the Plan
// Proposals that change a Cycle's Plan once the user approves them.
package planning

import (
	"context"
	"errors"
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
// next delivery, so its Plan can no longer change.
var ErrCycleClosed = errors.New("plan proposal's cycle has closed")

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
	// Check is the proposed Plan with what it was validated against.
	Check      domain.PlanCheck
	Violations []domain.Violation
}

// Service is the planning use cases.
type Service struct {
	pool    *pgxpool.Pool
	profile *profile.Service
}

// New creates the Service; Plans are checked against the Profile in
// profile.
func New(pool *pgxpool.Pool, profile *profile.Service) *Service {
	return &Service{pool: pool, profile: profile}
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
		check, err := s.check(ctx, tx, *o.Cycle, plan, kept)
		o.Plan = &check
		return err
	})
	return o, err
}

// check gathers what a Plan of c, using the Portions without a Slot kept,
// is validated against.
func (s *Service) check(ctx context.Context, q db.Querier, c Cycle, plan domain.Plan, kept []domain.KeptPortion) (domain.PlanCheck, error) {
	p, _, err := s.profile.Get(ctx)
	if err != nil {
		return domain.PlanCheck{}, err
	}
	check := domain.PlanCheck{Cycle: c.Cycle, Plan: plan, Profile: p, Kept: kept}
	if check.Earlier, err = loadEarlier(ctx, q, c); err != nil {
		return check, err
	}
	check.Since, err = loadSince(ctx, q)
	return check, err
}

// OpenCycle opens c as the current Cycle. A delivery before the current
// Cycle ends closes it early, carrying what is left over as Portions
// without a Slot.
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
			if err := closeCycle(ctx, tx, current.ID, c.DeliveryOn, opening); err != nil {
				return err
			}
		}
		return insertCycle(ctx, tx, c)
	})
}

// Proposal loads the Plan Proposal with id and checks its Plan.
func (s *Service) Proposal(ctx context.Context, id int64) (Proposal, error) {
	var p Proposal
	err := pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		stored, err := loadProposal(ctx, tx, id, false)
		if err != nil {
			return err
		}
		p = Proposal{ID: stored.ID, Origin: stored.Origin, Status: stored.Status, Note: stored.Note}
		if p.Cycle, err = loadCycle(ctx, tx, stored.CycleID); err != nil {
			return err
		}
		proposed, err := readPlan(stored.Plan)
		if err != nil {
			return err
		}
		plan, err := proposed.resolve(ctx, tx)
		if err != nil {
			return err
		}
		kept, err := loadKept(ctx, tx, p.Cycle.ID)
		if err != nil {
			return err
		}
		if p.Check, err = s.check(ctx, tx, p.Cycle, plan, kept); err != nil {
			return err
		}
		p.Violations = domain.ValidatePlan(p.Check)
		return nil
	})
	return p, err
}

// Approve makes the Plan Proposal with id its Cycle's Plan, replacing the
// Plan it had, all at once or not at all.
func (s *Service) Approve(ctx context.Context, id int64) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		stored, err := pending(ctx, tx, id)
		if err != nil {
			return err
		}
		c, err := loadCycle(ctx, tx, stored.CycleID)
		if err != nil {
			return err
		}
		// Its days and leftovers now belong to the next Cycle.
		if c.ClosedOn != nil {
			return ErrCycleClosed
		}
		plan, err := readPlan(stored.Plan)
		if err != nil {
			return err
		}
		// Every Recipe and Ingredient must exist, or the Plan stored would
		// quietly miss them.
		if _, err := plan.resolve(ctx, tx); err != nil {
			return err
		}
		err = replacePlan(ctx, tx, c, plan)
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
