package planning

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
)

// The Plan Proposal ledger: turn, submit, decide and endPending (in the store) and end
// are the only ways a Plan Proposal's status changes or a planning Run is cancelled.

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
	if err := lockPlanning(ctx, tx); err != nil {
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

// cause is what ends a Plan Proposal or cancels a planning Run when the User doesn't, as
// stored with it: the Delivery that closed its Cycle, or its Cycle dropped.
type cause struct {
	ClosedBy *domain.Delivery `json:"closed_by,omitempty"`
	Dropped  bool             `json:"dropped,omitempty"`
}

// end cancels, in tx, the planning Run going for the Cycle id, as something that
// happened, why, ends it, and is what stops it: call stop only once tx has committed.
// Cancelled once, it stays so.
func (s *Service) end(ctx context.Context, tx pgx.Tx, id int64, why cause) (stop func(context.Context), err error) {
	doc, err := json.Marshal(why)
	if err != nil {
		return nil, err
	}
	runID, err := goingRun(ctx, tx, id)
	if err != nil || runID == 0 {
		return func(context.Context) {}, err
	}
	return s.runs.CancelIn(ctx, tx, runID, doc)
}

// Arrive takes every Delivery of the signed-in User's whose window has ended by now as
// arrived: a Cycle it closes has its pending Plan Proposal rejected and its planning Run
// cancelled, naming the Delivery. Run again for a moment it has reached, it changes
// nothing. Every request runs it, after bringing the kitchen up to now, as the beat does.
func (s *Service) Arrive(ctx context.Context, now time.Time) error {
	var stops []func(context.Context)
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		busy, err := busyCycles(ctx, tx)
		if err != nil || len(busy) == 0 {
			return err
		}
		// turn's lock, then busy read again: a planning Run that started meanwhile
		// is seen.
		if err := lockPlanning(ctx, tx); err != nil {
			return err
		}
		if busy, err = busyCycles(ctx, tx); err != nil {
			return err
		}
		cycles, err := loadCycles(ctx, tx, now.Location())
		if err != nil {
			return err
		}
		for i, status := range domain.CycleStatuses(plain(cycles), now) {
			if status != domain.Closed || !slices.Contains(busy, cycles[i].ID) {
				continue
			}
			next := cycles[i+1].Delivery // closed: the next Cycle's Delivery has arrived
			why := cause{ClosedBy: &next}
			if err := endPending(ctx, tx, cycles[i].ID, why, now); err != nil {
				return err
			}
			stop, err := s.end(ctx, tx, cycles[i].ID, why)
			if err != nil {
				return err
			}
			stops = append(stops, stop)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, stop := range stops {
		stop(ctx)
	}
	return nil
}

// Beat takes the Deliveries arrived by at as arrived for every User, the scheduler's
// beat. One User's failure is logged and doesn't stop the others.
func (s *Service) Beat(ctx context.Context, at time.Time, log *slog.Logger) {
	users, err := db.UserIDs(ctx, s.pool)
	if err != nil {
		log.ErrorContext(ctx, "arrival beat: list the Users", "err", err)
		return
	}
	for _, id := range users {
		if err := s.Arrive(db.WithUser(ctx, id), at); err != nil {
			log.ErrorContext(ctx, "arrival beat", "user", id, "err", err)
		}
	}
}

// Approve makes the signed-in User's pending Plan Proposal id their Cycle's Plan at now,
// accepting its Recipe Drafts into their Recipe Library, unless a rule blocking approval
// refuses it: ErrStartsInThePast once its first Slot has passed, and a
// *recipes.ClashError for a Recipe Draft that no longer fits the Recipe Library. It is
// ErrNoProposal when it isn't pending.
func (s *Service) Approve(ctx context.Context, id int64, now time.Time) error {
	p, err := s.profile.Profile(ctx)
	if err != nil {
		return err
	}
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		_, first, err := pendingProposal(ctx, tx, id, now.Location())
		if err != nil {
			return err
		}
		for _, v := range domain.StartsInThePast(first, p.SlotTimes, now) {
			if v.Rule.BlocksApproval() {
				return ErrStartsInThePast
			}
		}
		stored, err := deleteDrafts(ctx, tx, id) // their Prep Sessions' deferred key waits for prepareAccepted
		if err != nil {
			return err
		}
		known, err := ingredients.LoadIngredients(ctx, tx)
		if err != nil {
			return err
		}
		drafted, err := readDrafts(p.Language, known, stored)
		if err != nil {
			return err
		}
		accepted, err := recipes.AcceptDrafted(ctx, tx, p.Language, drafted)
		if err != nil {
			return err
		}
		for draftID, recipeID := range accepted {
			if err := prepareAccepted(ctx, tx, draftID, recipeID); err != nil {
				return err
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

// CycleOfProposal is the id of the Cycle the signed-in User's Plan Proposal id, whatever
// its status, plans, and ErrNoProposal when they have no such Plan Proposal.
func (s *Service) CycleOfProposal(ctx context.Context, id int64) (cycleID int64, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		cycleID, err = cycleOfProposal(ctx, tx, id)
		return err
	})
	return cycleID, err
}
