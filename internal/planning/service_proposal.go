package planning

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
)

// The Plan Proposal lifecycle: turn, submit, decide and endPending (in the store) and end
// are the only ways a Plan Proposal's status changes or a planning Run is cancelled.

// planningKind is the kind of Run that plans a Cycle into a Plan Proposal.
const planningKind runs.Kind = "planning"

// ProposalStatus is where a Plan Proposal is: pending until the User decides on it or
// something ends it.
type ProposalStatus string

// The statuses a Plan Proposal has, as the database checks them.
const (
	Pending  ProposalStatus = "pending"
	Approved ProposalStatus = "approved"
	Rejected ProposalStatus = "rejected"
	Replaced ProposalStatus = "replaced"
)

// ErrBusy is planning while another planning Run is going, or a Plan Proposal pending,
// for any of the User's Cycles.
var ErrBusy = errors.New("another planning Run is going or a Plan Proposal is pending")

var (
	// ErrPlanned is planning a Cycle that already has a Plan.
	ErrPlanned = errors.New("the Cycle already has a Plan")
	// ErrNoPlan is asking for changes to a Cycle that has no Plan.
	ErrNoPlan = errors.New("the Cycle has no Plan")
	// ErrNoNote is asking for changes without a Planning Note.
	ErrNoNote = errors.New("asking for changes needs a Planning Note")
	// ErrNothingLeft is planning a Cycle whose every Slot has passed.
	ErrNothingLeft = errors.New("no Slot of the Cycle is left to plan")
	// ErrNoProposal is deciding on a Plan Proposal the User has none pending of.
	ErrNoProposal = errors.New("no such pending Plan Proposal")
	// ErrStartsInThePast is approving a Plan Proposal whose first Slot has passed.
	ErrStartsInThePast = errors.New("the Plan Proposal's first Slot has passed")
	// ErrNoFailure is planning a Cycle again when its last planning Run didn't fail.
	ErrNoFailure = errors.New("the Cycle's last planning Run didn't fail")
)

// plans is what a planning Run asks of its Cycle's Plan.
type plans int

const (
	// eitherWay plans whether or not the Cycle has a Plan: again after a rejection with a
	// Planning Note or a failure.
	eitherWay plans = iota
	// firstPlan plans a Cycle with no Plan yet.
	firstPlan
	// changes plans changes to the Cycle's Plan.
	changes
)

// turn is whether the Run `runID` may plan the Cycle `id` now, asking `wants` of its
// Plan, read in `tx`, locked so two Runs can't both find it their turn: ErrPlanned for a
// first Plan of a Cycle with one, ErrNoPlan for changes to one with none, ErrBusy while
// another planning Run is going or a Plan Proposal pending, for any Cycle.
func turn(ctx context.Context, tx pgx.Tx, id, runID int64, wants plans) error {
	if err := lockPlanning(ctx, tx); err != nil {
		return err
	}
	planned, err := hasPlan(ctx, tx, id)
	if err != nil {
		return err
	}
	switch {
	case wants == firstPlan && planned:
		return ErrPlanned
	case wants == changes && !planned:
		return ErrNoPlan
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

// end cancels, in `tx`, the planning Run going for the Cycle `id`, as something that
// happened, `why`, ends it, and is what stops it: call `stop` only once `tx` has
// committed. Cancelled once, it stays so.
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

// StartPlanning starts a planning Run of the signed-in User's Cycle `id`, upcoming or
// current at `now`, from its first Slot still to come, told what the kitchen holds `now`,
// and is the Run's id. It is ErrNoCycle for a Cycle neither, ErrPlanned for one with a
// Plan, ErrNothingLeft when all its Slots have passed, and ErrBusy while another planning
// Run is going or a Plan Proposal is pending.
func (s *Service) StartPlanning(ctx context.Context, id int64, now time.Time) (int64, error) {
	c, err := s.Find(ctx, id, now)
	if err != nil {
		return 0, err
	}
	return s.start(ctx, c, firstSlot(c), now, "", firstPlan, nil)
}

// firstSlot is where planning `c` starts, the first Slot still to come from it on: its
// Delivery's day's breakfast, no later than its first Slot.
func firstSlot(c Cycle) domain.Slot { return domain.Slot{On: c.Delivery.On, Kind: domain.Breakfast} }

// start starts a planning Run of `c` at `now` from the Slot `from`, or the first still to
// come once that has passed, asking `wants` of `c`'s Plan, told every Planning Note of
// `c`, newest first, `note` first if not "", and is the Run's id. It replaces `c`'s Plan,
// if `c` has one, from that Slot on. `before`, if set, runs in the transaction recording
// the Run, before it is told whether it is `c`'s turn: an error from it starts nothing.
func (s *Service) start(ctx context.Context, c Cycle, from domain.Slot, now time.Time, note string, wants plans, before func(tx pgx.Tx) error) (int64, error) {
	p, err := s.profile.Profile(ctx)
	if err != nil {
		return 0, err
	}
	var slots []domain.Slot
	for _, sl := range c.Slots(p.SlotTimes) {
		if !sl.Before(from) && sl.At(p.SlotTimes).After(now) {
			slots = append(slots, sl)
		}
	}
	if len(slots) == 0 {
		return 0, ErrNothingLeft
	}
	brief, err := s.profile.Brief(ctx)
	if err != nil {
		return 0, err
	}
	k, err := s.kitchenAt(ctx, c, slots[0], p.SlotTimes, now)
	if err != nil {
		return 0, err
	}
	var notes []string
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		notes, err = loadNotes(ctx, tx, c.ID)
		return err
	})
	if err != nil {
		return 0, err
	}
	library, known, held, portions, current, forecast := k.library, k.known, k.held, k.portions, k.plan, k.forecast
	earlier := k.standing.Eaten(portions)
	trailing, _ := domain.PlanDays(earlier, nil, slots[0], slots[0].On)
	if note != "" {
		notes = append([]string{note}, notes...)
	}
	b := planBrief{
		cycle: c.Cycle, from: slots[0], profile: p, current: current, lots: forecast.Lots, stock: forecast.Stock, earlier: earlier, now: s.clock.Now,
		library: library, known: known, checks: s.checks, zone: now.Location(),
		leave: func(ctx context.Context, plan domain.Plan, v domain.Verdict, summary string, drafted []*recipes.Drafted) {
			runID := runs.IDFrom(ctx)
			s.runs.Leaves(ctx, func(ctx context.Context, tx pgx.Tx) error {
				return submit(ctx, tx, runID, plan, p.Targets, v, summary, drafted)
			})
		},
	}
	_, free := current.Replace(slots[0], slots[len(slots)-1], forecast.Lots)
	told := forecast
	told.Lots = free
	prompt := planPrompt(c, p, slots, brief, notes, now, held.Held, told, current, portions, trailing)
	asked, _, _ := strings.Cut(prompt, "\n")
	return s.runs.Start(ctx, planningKind, asked, b.brief(p.Language, planStable(p, library, known), prompt), func(tx pgx.Tx, runID int64) error {
		// The planning lock first, as in Drop, so a reject and a drop never deadlock.
		if err := lockPlanning(ctx, tx); err != nil {
			return err
		}
		if before != nil {
			if err := before(tx); err != nil {
				return err
			}
		}
		if err := turn(ctx, tx, c.ID, runID, wants); err != nil {
			return err
		}
		return saveCycleRun(ctx, tx, runID, c.ID, slots[0])
	})
}

// Approve makes the signed-in User's pending Plan Proposal `id` their Cycle's Plan at
// `now`, accepting its Recipe Drafts into their Recipe Library, unless a rule blocking
// approval refuses it: ErrStartsInThePast once its first Slot has passed, and a
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

// Reject rejects the signed-in User's pending Plan Proposal `id` at `now`. With a
// Planning Note it starts a planning Run of its Cycle, with no Kitchen Check, from its
// first Slot, or the first still to come once that has passed, and is the Run's id; the
// rejection, the `note` and the Run commit together. It is ErrNoProposal when it
// isn't pending.
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
	return s.start(ctx, c, first, now, note, eitherWay, func(tx pgx.Tx) error {
		if err := decide(ctx, tx, id, Rejected, now); err != nil {
			return err
		}
		return saveNote(ctx, tx, cycleID, note, now, &id)
	})
}

// PlanAgain starts a planning Run of the signed-in User's Cycle `id` at `now`, with no
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
		return s.start(ctx, c, last.first, now, "", eitherWay, nil)
	}
	return s.start(ctx, c, last.first, now, note, eitherWay, func(tx pgx.Tx) error {
		return saveNote(ctx, tx, id, note, now, nil)
	})
}

// AskForChanges starts a planning Run of the signed-in User's Cycle `id` at `now`, with
// no Kitchen Check, from its first Slot still to come, told the Planning Note `note`,
// which it keeps on the Cycle, and is the Run's id. The Plan stays as it is until the
// Run's Plan Proposal is approved. It is ErrNoNote for a blank `note`, ErrNoCycle for a
// Cycle neither upcoming nor current, ErrNoPlan for one with no Plan, ErrNothingLeft when
// all its Slots have passed, and ErrBusy while another planning Run is going or a Plan
// Proposal is pending.
func (s *Service) AskForChanges(ctx context.Context, id int64, note string, now time.Time) (int64, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return 0, ErrNoNote
	}
	c, err := s.Find(ctx, id, now)
	if err != nil {
		return 0, err
	}
	return s.start(ctx, c, firstSlot(c), now, note, changes, func(tx pgx.Tx) error {
		return saveNote(ctx, tx, id, note, now, nil)
	})
}

// CycleOfRun is the Cycle the signed-in User's planning Run `runID` plans, while it is
// upcoming or current at `now`, and whether that Run is the `latest` of its Cycle. It is
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

// CycleOfProposal is the id of the Cycle the signed-in User's Plan Proposal `id`,
// whatever its status, plans, and ErrNoProposal when they have no such Plan Proposal.
func (s *Service) CycleOfProposal(ctx context.Context, id int64) (cycleID int64, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		cycleID, err = cycleOfProposal(ctx, tx, id)
		return err
	})
	return cycleID, err
}
