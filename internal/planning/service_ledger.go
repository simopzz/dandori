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
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
)

// The Plan Proposal ledger: turn, submit, decide and endPending (in the store) and end
// are the only ways a Plan Proposal's status changes or a planning Run is cancelled.

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

// BringUpToNow is the step that brings the signed-in User's kitchen up to `now`
// (ADR-0021): in one transaction, under the planning lock and then the kitchen lock, it
// records how far the kitchen has reached, takes every Delivery whose window has ended
// by `now` as arrived, and records what the Plan assumed happened since the kitchen was
// last brought up to now. A Cycle a Delivery closes has its pending Plan Proposal
// rejected and its planning Run cancelled, naming the Delivery, and each Delivery is put
// away at its arrival, its Grocery List, fixed then if it wasn't ordered, becoming Stock
// Items where the Plan's freezer splits say (domain.PutAway). Each Cycle's Plan, as far
// as it lasts, has its Prep Sessions use up stock and prepare their Portions, and the
// Portion filling each Slot eaten, at their Slots' times, interleaved with the arrivals
// (domain.Ledger.UpToNow). A Cycle with no Plan brings nothing. Run again for a moment it
// has reached, it changes nothing. Every request runs it, as the beat does.
func (s *Service) BringUpToNow(ctx context.Context, now time.Time) error {
	return s.upToNow(ctx, now, false)
}

// upToNow is BringUpToNow, which records the scheduler's beat at `now` too when `beat`.
func (s *Service) upToNow(ctx context.Context, now time.Time, beat bool) error {
	due, upTo, cycled, err := s.dueAt(ctx, now)
	if err != nil {
		return err
	}
	// What a Plan needs is read only once a Slot of a User with a Cycle has passed: the
	// kitchen reaches no further back than `upTo`. A User without one may have
	// no Profile.
	var times domain.SlotTimes
	if cycled {
		if times, err = s.profile.SlotTimes(ctx); err != nil {
			return err
		}
	}
	var library []domain.Recipe
	var known []domain.Ingredient
	loaded := false
	load := func() (err error) {
		if library, err = s.recipes.Search(ctx, ""); err != nil {
			return err
		}
		known, err = s.ingredients.List(ctx)
		loaded = err == nil
		return err
	}
	if cycled && times.PassBetween(upTo, now) {
		if err := load(); err != nil {
			return err
		}
	}
	var stops []func(context.Context)
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockPlanning(ctx, tx); err != nil {
			return err
		}
		from, err := ledger.SaveUpTo(ctx, tx, now, beat)
		if err != nil {
			return err
		}
		passed := cycled && times.PassBetween(from, now)
		busy, err := busyCycles(ctx, tx)
		if err != nil || (len(busy) == 0 && len(due) == 0 && !passed) {
			return err
		}
		cycles, err := loadCycles(ctx, tx, now.Location())
		if err != nil {
			return err
		}
		arrivals, err := putAwayDue(ctx, tx, cycles, due, now)
		if err != nil {
			return err
		}
		var plan domain.Plan
		if passed && !loaded {
			// The kitchen reached less far than `upTo` said: a first request racing
			// another, before either made its row.
			if err := load(); err != nil {
				return err
			}
		}
		if passed {
			if plan, err = plansBetween(ctx, tx, cycles, library, known, times, from, now); err != nil {
				return err
			}
		}
		if len(arrivals) > 0 || passed {
			err := ledger.Write(ctx, tx, func(l domain.Ledger) ([]domain.Movement, error) {
				madeFrom, err := ledger.LoadMadeFrom(ctx, tx)
				if err != nil {
					return nil, err
				}
				written, made := l.UpToNow(plan, madeFrom, arrivals, times, from, now)
				return written, ledger.SavePrepared(ctx, tx, made, written)
			})
			if err != nil {
				return err
			}
		}
		for i, status := range domain.CycleStatuses(plain(cycles), now) {
			c := cycles[i]
			if status != domain.Closed || !slices.Contains(busy, c.ID) {
				continue
			}
			next := cycles[i+1].Delivery // closed: the next Cycle's Delivery has arrived
			why := cause{ClosedBy: &next}
			if err := endPending(ctx, tx, c.ID, why, now); err != nil {
				return err
			}
			stop, err := s.end(ctx, tx, c.ID, why)
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

// plansBetween is the Plans of `cycles`, read in `q`, that may have a Slot after `from`
// and by `to`, by `times`, together: each Cycle's as its approved Plan Proposals leave
// it, Until the next Cycle's Delivery arrives, its Recipes and Ingredients found among
// `library` and `known`. Days between Cycles are in none.
func plansBetween(ctx context.Context, q db.Querier, cycles []Cycle, library []domain.Recipe, known []domain.Ingredient, times domain.SlotTimes, from, to time.Time) (domain.Plan, error) {
	var plan domain.Plan
	for i, c := range cycles {
		if i+1 < len(cycles) {
			c.Until = cycles[i+1].Delivery.Arrives()
		}
		if c.Delivery.Arrives().After(to) || (!c.Until.IsZero() && !c.Until.After(from)) || !c.Ends.AddDate(0, 0, 1).After(from) {
			continue
		}
		own, err := loadPlanOf(ctx, q, c.ID, to.Location())
		if err != nil {
			return plan, err
		}
		p := cyclePlan(c, own, library, known, nil, times)
		plan.PrepSessions = append(plan.PrepSessions, p.PrepSessions...)
		plan.Kept = append(plan.Kept, p.Kept...)
	}
	return plan, nil
}

// putAwayDue puts away at `now`, in `tx`, each of `cycles`' Deliveries `due` says arrived
// and isn't put away yet, and is their arrivals, for the Ledger. One whose state or fixed
// list changed since `due` was worked out, before the lock, waits for the next request.
func putAwayDue(ctx context.Context, tx pgx.Tx, cycles []Cycle, due map[int64]arrival, now time.Time) ([]domain.Arrival, error) {
	var arrivals []domain.Arrival
	for _, c := range cycles {
		a, ok := due[c.ID]
		if !ok || a.state != c.Delivery.State {
			continue
		}
		same, err := fixedAs(ctx, tx, c, a.list.Lines)
		if err != nil {
			return nil, err
		}
		if !same {
			continue
		}
		done, err := putAway(ctx, tx, c.ID, now, a.list.Lines)
		if err != nil {
			return nil, err
		}
		if done {
			arrivals = append(arrivals, domain.Arrival{At: c.Delivery.Arrives(), Put: domain.PutAway(a.list.Lines, a.splits)})
		}
	}
	return arrivals, nil
}

// fixedAs reports whether the Grocery List of `c`, read in `tx`, is still fixed as
// `lines` once its Delivery is ordered: undone and marked again, it may not be.
func fixedAs(ctx context.Context, tx pgx.Tx, c Cycle, lines []domain.GroceryLine) (bool, error) {
	if c.Delivery.State != domain.Ordered {
		return true, nil
	}
	fixed, err := loadGroceryLines(ctx, tx, c.ID)
	if err != nil || len(fixed) != len(lines) {
		return false, err
	}
	for _, l := range lines {
		if fixed[l.Ingredient.ID] != l.Quantity {
			return false, nil
		}
	}
	return true, nil
}

// arrival is what a Delivery due to be put away brings: its Grocery List, as it was
// read in `state`, and its Plan's freezer splits.
type arrival struct {
	state  domain.DeliveryState
	list   domain.GroceryList
	splits []domain.RecipeIngredient
}

// dueAt is what each of the signed-in User's Deliveries arrived by `now` and not put
// away yet brings, by its Cycle's id, nothing for a Cycle without a Plan, how far their
// kitchen has been brought up to now (ledger.SaveUpTo), and whether they have any Cycle.
func (s *Service) dueAt(ctx context.Context, now time.Time) (_ map[int64]arrival, upTo time.Time, cycled bool, err error) {
	var cycles []Cycle
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		if upTo, err = ledger.LoadUpTo(ctx, tx, now); err != nil {
			return err
		}
		cycles, err = loadCycles(ctx, tx, now.Location())
		return err
	})
	if err != nil {
		return nil, upTo, false, err
	}
	due := map[int64]arrival{}
	for i, c := range cycles {
		if c.putAway || c.Delivery.Arrives().After(now) {
			continue
		}
		if i+1 < len(cycles) {
			c.Until = cycles[i+1].Delivery.Arrives()
		}
		list, plan, err := s.groceryList(ctx, c, now)
		if err != nil && !errors.Is(err, ErrNoPlan) {
			return nil, upTo, true, err
		}
		due[c.ID] = arrival{state: c.Delivery.State, list: list, splits: plan.FreezerSplits}
	}
	return due, upTo, len(cycles) > 0, nil
}

// Beat brings every User's kitchen up to `at`, the scheduler's beat, and records the beat
// for each. One User's failure is logged and doesn't stop the others.
func (s *Service) Beat(ctx context.Context, at time.Time, log *slog.Logger) {
	users, err := db.UserIDs(ctx, s.pool)
	if err != nil {
		log.ErrorContext(ctx, "kitchen beat: list the Users", "err", err)
		return
	}
	for _, id := range users {
		if err := s.upToNow(db.WithUser(ctx, id), at, true); err != nil {
			log.ErrorContext(ctx, "kitchen beat", "user", id, "err", err)
		}
	}
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
// still to come once that has passed, and is the Run's id. A Planning Note is kept on
// the Cycle and told to the Run; a blank one is none. It is ErrNoFailure unless that Run
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
