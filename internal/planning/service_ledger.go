package planning

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/runs"
)

// The Plan Proposal ledger: turn, submit and decide (in the store) and end are the only
// ways a Plan Proposal's status changes or a planning Run is cancelled.

var (
	// ErrPlanned is planning a Cycle that already has a Plan.
	ErrPlanned = errors.New("the Cycle already has a Plan")
	// ErrNothingLeft is planning a Cycle whose every Slot has passed.
	ErrNothingLeft = errors.New("no Slot of the Cycle is left to plan")
	// ErrNoProposal is deciding on a Plan Proposal the User has none pending of.
	ErrNoProposal = errors.New("no such pending Plan Proposal")
	// ErrStartsInThePast is approving a Plan Proposal whose first Slot has passed.
	ErrStartsInThePast = errors.New("the Plan Proposal's first Slot has passed")
	// ErrNoFailure is planning a Cycle again when its last planning Run didn't fail.
	ErrNoFailure = errors.New("the Cycle's last planning Run didn't fail")
)

// turn is whether the Run runID may plan the Cycle id now, read in tx, locked so two
// Runs can't both find it their turn: ErrPlanned when the Cycle has a Plan, ErrBusy while
// another planning Run is going or a Plan Proposal pending, for any Cycle.
func turn(ctx context.Context, tx pgx.Tx, id, runID int64) error {
	if err := lockOpening(ctx, tx); err != nil {
		return err
	}
	planned, err := hasPlan(ctx, tx, id)
	if err != nil {
		return err
	}
	if planned {
		return ErrPlanned
	}
	busy, err := planningBusy(ctx, tx, runID)
	if err != nil {
		return err
	}
	if busy {
		return ErrBusy
	}
	return nil
}

// end cancels the planning Run going for the Cycle id in tx, as something that happened
// ends it, and is what stops it: call stop only once tx has committed.
func (s *Service) end(ctx context.Context, tx pgx.Tx, id int64) (stop func(context.Context), err error) {
	runID, err := goingRun(ctx, tx, id)
	if err != nil || runID == 0 {
		return func(context.Context) {}, err
	}
	return s.runs.CancelIn(ctx, tx, runID)
}

// Approve makes the signed-in User's pending Plan Proposal id their Cycle's Plan at now,
// unless a rule blocking approval refuses it: ErrStartsInThePast once its first Slot has
// passed. It is ErrNoProposal when it isn't pending.
func (s *Service) Approve(ctx context.Context, id int64, now time.Time) error {
	times, err := s.profile.SlotTimes(ctx)
	if err != nil {
		return err
	}
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		_, first, err := pendingProposal(ctx, tx, id, now.Location())
		if err != nil {
			return err
		}
		for _, v := range domain.StartsInThePast(first, times, now) {
			if v.Rule.BlocksApproval() {
				return ErrStartsInThePast
			}
		}
		return decide(ctx, tx, id, Approved, now)
	})
}

// Reject rejects the signed-in User's pending Plan Proposal id at now. With a Planning
// Note it starts a planning Run of its Cycle, with no Kitchen Check, from its first Slot,
// or the first still to come once that has passed, and is the Run's id; the rejection,
// the note and the Run commit together. It is ErrNoProposal when it isn't pending.
func (s *Service) Reject(ctx context.Context, id int64, note string, now time.Time) (runID int64, err error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return 0, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return decide(ctx, tx, id, Rejected, now) })
	}
	var cycleID int64
	var first domain.Slot
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		cycleID, first, err = pendingProposal(ctx, tx, id, now.Location())
		return err
	})
	if err != nil {
		return 0, err
	}
	c, err := s.Find(ctx, cycleID, now)
	if err != nil {
		return 0, err
	}
	return s.start(ctx, c, first, now, note, func(tx pgx.Tx) error {
		if err := decide(ctx, tx, id, Rejected, now); err != nil {
			return err
		}
		return saveNote(ctx, tx, cycleID, note, now, &id)
	})
}

// PlanAgain starts a planning Run of the signed-in User's Cycle id at now, with no
// Kitchen Check, from the first Slot its last planning Run planned from, or the first
// still to come once that has passed, and is the Run's id. A Planning Note is kept on the
// Cycle and told to the Run; a blank one is none. It is ErrNoFailure unless that Run
// failed: after any other, planning goes through the Kitchen Check.
func (s *Service) PlanAgain(ctx context.Context, id int64, note string, now time.Time) (int64, error) {
	note = strings.TrimSpace(note)
	c, err := s.Find(ctx, id, now)
	if err != nil {
		return 0, err
	}
	var last lastRun
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		last, err = loadLastRun(ctx, tx, id, now.Location())
		return err
	})
	if err != nil {
		return 0, err
	}
	if last.status != runs.Failed {
		return 0, ErrNoFailure
	}
	if note == "" {
		return s.start(ctx, c, last.first, now, "", nil)
	}
	return s.start(ctx, c, last.first, now, note, func(tx pgx.Tx) error {
		return saveNote(ctx, tx, id, note, now, nil)
	})
}

// CycleOfRun is the Cycle the signed-in User's planning Run runID plans, while it is
// upcoming or current at now, and whether that Run is the latest of its Cycle. It is
// ErrNoCycle once the Cycle has closed or been dropped.
func (s *Service) CycleOfRun(ctx context.Context, runID int64, now time.Time) (c Cycle, latest bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		id, err := cycleOfRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		if c, err = findOpen(ctx, tx, id, now); err != nil {
			return err
		}
		last, err := loadLastRun(ctx, tx, id, now.Location())
		latest = last.id == runID
		return err
	})
	return c, latest, err
}
