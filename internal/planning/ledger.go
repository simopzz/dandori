package planning

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// The ledger is every change to a Plan Proposal's status and to the
// planning Run going: whose turn it is to be planned (turn), a planning Run
// storing its Plan Proposal (submit), the user deciding on it (decide), and
// something that happened ending it (end). Nothing else writes a Plan
// Proposal's status or cancels a planning Run: a transaction records the
// Runs it cancels, and write stops them once it commits.

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

// ProposalStatus is how a Plan Proposal stands, as stored.
type ProposalStatus string

const (
	ProposalPending  ProposalStatus = "pending"
	ProposalApproved ProposalStatus = "approved"
	ProposalRejected ProposalStatus = "rejected"
	ProposalReplaced ProposalStatus = "replaced"
)

// asking is what a planning Run is started for.
type asking int

const (
	// askPlan: a Cycle's initial Plan, while it has none.
	askPlan asking = iota
	// askChanges: changes to a Cycle's Plan, with a Planning Note.
	askChanges
	// askAny: a Plan Proposal whether or not the Cycle has a Plan, as
	// after the user rejected one with a Planning Note.
	askAny
)

// turn checks a planning Run can start for the Cycle c, planned from the
// Slot from on, for what ask says: it is open, has no Plan for an initial
// one and has one for changes, it has Slots left to plan, and no Plan
// Proposal is pending nor planning Run going for any Cycle, since one is
// planned at a time.
func (s *Service) turn(ctx context.Context, q db.Querier, c Cycle, from domain.Slot, ask asking) error {
	if c.Status == "closed" {
		return &CannotPlanError{Reason: "the next Delivery has arrived, so it is closed"}
	}
	if planned, err := hasPlan(ctx, q, c.ID); err != nil {
		return err
	} else if planned && ask == askPlan {
		return &CannotPlanError{Reason: "it already has a Plan: ask for changes instead"}
	} else if !planned && ask == askChanges {
		return &CannotPlanError{Reason: "it has no Plan yet: plan it instead"}
	}
	if id, status, err := pendingFor(ctx, q); err != nil {
		return err
	} else if id == c.ID {
		return &CannotPlanError{Reason: "a Plan Proposal for it is pending: approve or reject it"}
	} else if id != 0 {
		return &CannotPlanError{Reason: "a Plan Proposal for the " + status + " Cycle is pending: approve or reject it", WaitsTurn: true}
	}
	if !c.Contains(from) {
		return &CannotPlanError{Reason: "it has no Slots left to plan"}
	}
	going, err := s.runs.Going(ctx, q, planningKind)
	if err != nil {
		return err
	}
	if run, id, status, err := goingPlanningRun(ctx, q, going); err != nil {
		return err
	} else if id == c.ID {
		return &CannotPlanError{Reason: "it is being planned", RunID: run}
	} else if run != 0 {
		return &CannotPlanError{Reason: "the " + status + " Cycle is being planned", WaitsTurn: true}
	}
	return nil
}

// stillTurn checks the planning Run with run, going for the Cycle c from
// the Slot from on, can still store its Plan Proposal: not cancelled since,
// by what ended its Plan Proposal.
func (s *Service) stillTurn(ctx context.Context, q db.Querier, c Cycle, from domain.Slot, run int64) error {
	// Nothing approves a Plan while it goes, so whether c has one is as
	// when it started.
	err := s.turn(ctx, q, c, from, askAny)
	if going := (*CannotPlanError)(nil); errors.As(err, &going) && going.RunID == run {
		return nil
	}
	if err != nil {
		return err
	}
	// Its turn, yet not because run is going: run was cancelled.
	status, err := s.runs.Status(ctx, q, run)
	if err != nil {
		return err
	}
	return fmt.Errorf("the planning Run is %s", status)
}

// submit stores raw, the Plan the planning Run with run submitted for c
// from the Slot from on, as c's Plan Proposal, as part of tx, if it keeps
// every rule: its initial one, or changes once c has a Plan: every Violation blocks submitting. It returns the
// problems with it: why it can't be read, or the rules it breaks.
func (s *Service) submit(ctx context.Context, tx pgx.Tx, c Cycle, from domain.Slot, run int64, raw []byte) ([]string, error) {
	// The user may have planned it another way meanwhile, or it closed.
	if err := s.stillTurn(ctx, tx, c, from, run); err != nil {
		return nil, agent.Stop(err)
	}
	ch, err := s.checkProposal(ctx, tx, storedProposal{CycleID: c.ID, RunID: run, Plan: raw})
	if unreadable := (*UnreadableError)(nil); errors.As(err, &unreadable) {
		return []string{"the Plan can't be read: " + unreadable.Reason}, nil
	}
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, v := range domain.ValidatePlan(ch.check) {
		if v.Blocks(domain.Submitting) {
			problems = append(problems, v.String())
		}
	}
	if len(problems) > 0 {
		return problems, nil
	}
	origin := "initial"
	if planned, err := hasPlan(ctx, tx, c.ID); err != nil {
		return nil, err
	} else if planned {
		origin = "changes"
	}
	if substitution, err := substitutes(ctx, tx, run); err != nil {
		return nil, err
	} else if substitution {
		origin = "substitution"
	}
	_, err = tx.Exec(ctx, `INSERT INTO plan_proposal (cycle_id, origin, plan, run_id) VALUES ($1, $2, $3, $4)`, c.ID, origin, raw, run)
	return nil, err
}

// approvalErrors are what approving a Plan Proposal meets for the
// Violations that block it, the most telling first.
var approvalErrors = []struct {
	rule domain.Rule
	err  error
}{
	// What was prepared and eaten since its start can't be replaced.
	{domain.RuleStartsInPast, ErrStartsInPast},
	// It would eat what the user won't, or cook for nobody.
	{domain.RuleExceptedSlot, ErrFillsExceptedSlot},
}

// decide decides the pending Plan Proposal with id as status, approved or
// rejected, with the user's note, as part of tx, returning it. Approving
// checks it first: its Cycle still open, and no Violation blocking
// approval; apply then makes it the Plan.
func (s *Service) decide(ctx context.Context, tx pgx.Tx, id int64, status ProposalStatus, note string, apply func(checked) error) (storedProposal, error) {
	stored, err := pending(ctx, tx, id)
	if err != nil {
		return stored, err
	}
	if status == ProposalApproved {
		ch, err := s.checkProposal(ctx, tx, stored)
		if err != nil {
			return stored, err
		}
		// Its days and what is left of them now belong to the next Cycle.
		if ch.cycle.Status == "closed" {
			return stored, ErrCycleClosed
		}
		if err := approvalBlocked(domain.ValidatePlan(ch.check)); err != nil {
			return stored, err
		}
		if err := apply(ch); err != nil {
			return stored, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE plan_proposal SET status = $2, note = $3, decided_at = $4 WHERE id = $1`, id, status, note, s.now())
	return stored, err
}

// approvalBlocked is what stops approving a Plan Proposal that breaks
// violations, or nil when none of them blocks approval.
func approvalBlocked(violations []domain.Violation) error {
	var blocking []domain.Violation
	for _, v := range violations {
		if v.Blocks(domain.Approving) {
			blocking = append(blocking, v)
		}
	}
	for _, a := range approvalErrors {
		for _, v := range blocking {
			if v.Rule == a.rule {
				return a.err
			}
		}
	}
	if len(blocking) > 0 {
		return fmt.Errorf("plan proposal breaks %s", blocking[0])
	}
	return nil
}

// ending is how something that happened ends a Cycle's Plan Proposal.
type ending int

const (
	// endReplaced: what it planned around changed, such as an Exception or
	// a moved Delivery, so a new one replaces it.
	endReplaced ending = iota
	// endClosed: the next Delivery arrived and closed its Cycle, so its
	// Plan no longer changes.
	endClosed
	// endDropped: its Cycle was dropped, and it with it.
	endDropped
)

// end ends the Plan Proposal pending for any of the Cycles with ids, as
// how says, and cancels the planning Run going for any of them, because
// of cause, what happened, as part of q, adding the Run to cancelled: the
// next planning Run reads what happened for itself, so cause is never a
// Planning Note.
func (s *Service) end(ctx context.Context, q db.Querier, cancelled *cancels, ids []int64, how ending, cause string) error {
	var status ProposalStatus
	switch how {
	case endReplaced:
		status = ProposalReplaced
	case endClosed:
		status = ProposalRejected
	case endDropped:
		// Deleting its Cycle deletes it.
	}
	if status != "" {
		if _, err := q.Exec(ctx, `
			UPDATE plan_proposal SET status = $2, cause = $3, decided_at = $4
			WHERE cycle_id = ANY($1) AND status = 'pending'`, ids, status, cause, s.now()); err != nil {
			return err
		}
	}
	// One that has stored its Plan Proposal is left to end: that Plan
	// Proposal is what ends.
	running, err := s.runs.Going(ctx, q, planningKind)
	if err != nil {
		return err
	}
	going, err := planningRunsGoing(ctx, q, running, ids)
	if err != nil {
		return err
	}
	if err := s.runs.CancelIn(ctx, q, going, cause); err != nil {
		return err
	}
	*cancelled = append(*cancelled, going...)
	return nil
}
