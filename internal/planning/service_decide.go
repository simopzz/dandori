package planning

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/recipes"
)

// Deciding on a Plan Proposal: the User approves or rejects it, through the ways
// service_run.go names.

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

var (
	// ErrNoProposal is deciding on a Plan Proposal the User has none pending of.
	ErrNoProposal = errors.New("no such pending Plan Proposal")
	// ErrStartsInThePast is approving a Plan Proposal whose first Slot has passed.
	ErrStartsInThePast = errors.New("the Plan Proposal's first Slot has passed")
)

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

// CycleOfProposal is the id of the Cycle the signed-in User's Plan Proposal `id`,
// whatever its status, plans, and ErrNoProposal when they have no such Plan Proposal.
func (s *Service) CycleOfProposal(ctx context.Context, id int64) (cycleID int64, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		cycleID, err = cycleOfProposal(ctx, tx, id)
		return err
	})
	return cycleID, err
}
