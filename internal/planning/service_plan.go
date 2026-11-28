package planning

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/kitchen"
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
)

// cycleKitchen is what planning a Cycle, or shopping for it, reads of the kitchen at a
// Slot: the Recipe Library, the known Ingredients, what the kitchen holds and its
// Portions, what every approved Plan Proposal plans together, the Cycle's own Plan,
// whether it has one, its Grocery List's lines once fixed, in name order, and the kitchen
// forecast at that Slot.
type cycleKitchen struct {
	library        []domain.Recipe
	known          []domain.Ingredient
	held           kitchen.Contents
	portions       []domain.Portion
	standing, plan domain.Plan
	planned        bool
	fixed          []domain.GroceryLine
	forecast       domain.Forecast
}

// kitchenAt is the cycleKitchen of the signed-in User's Cycle `c` at `now` by `times`,
// its forecast at the Slot `first`, once the approved Prep Sessions before it have used
// up theirs and an ordered Delivery of `c`'s has brought its fixed lines.
func (s *Service) kitchenAt(ctx context.Context, c Cycle, first domain.Slot, times domain.Times, now time.Time) (k cycleKitchen, err error) {
	if k.library, err = s.recipes.Search(ctx, ""); err != nil {
		return k, err
	}
	if k.known, err = s.ingredients.List(ctx); err != nil {
		return k, err
	}
	if k.held, err = s.kitchen.Contents(ctx, now); err != nil {
		return k, err
	}
	var approved, own []*storedProposal
	var lines map[int64]domain.Quantity
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		if approved, err = loadApproved(ctx, tx, first.On.AddDate(0, 0, -7), now.Location()); err != nil {
			return err
		}
		if own, err = loadPlanOf(ctx, tx, c.ID, now.Location()); err != nil {
			return err
		}
		if lines, err = loadGroceryLines(ctx, tx, c.ID); err != nil {
			return err
		}
		k.portions, err = ledger.LoadPortions(ctx, tx)
		return err
	})
	if err != nil {
		return k, err
	}
	k.fixed = groceryLinesOf(k.known, lines)
	k.standing = approvedPlan(approved, k.library, k.known, k.portions)
	k.plan, k.planned = cyclePlan(c, own, k.library, k.known, k.portions, times), len(own) > 0
	k.forecast = k.standing.Forecast(stockOf(k.held.Held), k.held.Portions, times, now, first).WithOrdered(c.Delivery, k.fixed, k.plan.FreezerSplits)
	return k, nil
}

// groceryLinesOf is the Grocery Lines `fixed` gives by Ingredient id, in the order of
// `known`, name order, which holds their Ingredients.
func groceryLinesOf(known []domain.Ingredient, fixed map[int64]domain.Quantity) []domain.GroceryLine {
	var lines []domain.GroceryLine
	for _, ing := range known {
		if q, ok := fixed[ing.ID]; ok {
			lines = append(lines, domain.GroceryLine{Ingredient: ing, Quantity: q})
		}
	}
	return lines
}

// approvedPlan is what `approved`, oldest first, plan together, their Recipes,
// Ingredients and Portions found among those given: each `approved` replacing, from its
// first Slot to its last, what those before it planned (domain.Plan.With).
func approvedPlan(approved []*storedProposal, library []domain.Recipe, known []domain.Ingredient, portions []domain.Portion) domain.Plan {
	var plan domain.Plan
	for _, sp := range approved {
		plan = plan.With(sp.first, sp.resolve(library, known, portions, nil).Plan)
	}
	return plan
}

// cyclePlan is `c`'s Plan by `times`, what `own`, its approved Plan Proposals oldest
// first, leave in place, as far as `c` lasts: Until the next Cycle's Delivery arrives.
// One approved after `c`'s Delivery arrives keeps the freezer splits before it: the
// Delivery was put away by those.
func cyclePlan(c Cycle, own []*storedProposal, library []domain.Recipe, known []domain.Ingredient, portions []domain.Portion, times domain.Times) domain.Plan {
	var plan domain.Plan
	for _, sp := range own {
		splits := plan.FreezerSplits
		plan = plan.With(sp.first, sp.resolve(library, known, portions, nil).Plan)
		if sp.decided != nil && sp.decided.After(c.Delivery.Arrives()) {
			plan.FreezerSplits = splits
		}
	}
	if !c.Until.IsZero() {
		plan = plan.Until(c.Until, times)
	}
	return plan
}

// stockOf is what `held` holds of each Stock Item.
func stockOf(held []kitchen.Held) map[domain.StockKey]domain.Quantity {
	stock := map[domain.StockKey]domain.Quantity{}
	for _, h := range held {
		stock[domain.StockKey{Ingredient: h.Ingredient.ID, Location: h.Location}] = h.Item.Held
	}
	return stock
}

// PortionSlots is the time of the Slot after `now` each Portion the signed-in User's
// kitchen may hold fills, by its id, in the approved Plans of their Cycles: the earliest,
// for one filling more than one (domain.Plan.Filled). The kitchen feature, which this one
// depends on, reads it in its own transaction, and app wires it: a read, made only of
// store reads.
func PortionSlots(ctx context.Context, q db.Querier, now time.Time) (map[int64]time.Time, error) {
	t, err := loadTimeline(ctx, q, now, nil, nil)
	if err != nil || len(t.cycles) == 0 {
		return nil, err
	}
	slots := map[int64]time.Time{}
	for id, s := range t.plan.Filled(t.madeFrom) {
		if at := s.At(t.times); at.After(now) {
			slots[id] = at
		}
	}
	return slots, nil
}

// Proposal is a Plan Proposal as its Cycle page shows it: its Plan, the Portions without
// a Slot it puts in Slots by id, the planner's summary, the Targets it aimed at, the soft
// rules it breaks, and the Recipe Drafts it prepares, while pending. It has its day table
// too: the days it plans and, for a pending one, the seven days before its first Slot; as
// judged while pending, worked out from today's Recipes once approved. Pending with the
// Cycle's Plan to replace, it has what it changes of that Plan: the Slots it fills with
// other food, and the indexes of its Prep Sessions the Plan has none alike of; and before
// the Delivery, the indexes of its freezer splits the Plan has none alike of, and whether
// it changes the Plan's freezer splits at all.
type Proposal struct {
	ID              int64
	Plan            domain.Plan
	Kept            map[int64]domain.Portion
	Summary         string
	Targets         domain.Targets
	Misses          []domain.Miss
	Drafted         []recipes.Drafted
	Trailing, Days  []domain.Day
	Changed         []domain.Slot
	ChangedSessions []int
	ChangedSplits   []int
	SplitsChanged   bool
}

// Planning is how planning a Cycle stands: its pending Plan Proposal, its Plan (what its
// approved Plan Proposals leave in place, with the last one's summary, Targets and soft
// misses), each nil when it has none, the id of its planning Run going, 0 when none is,
// whether one is going or a Plan Proposal is pending for any Cycle, its latest planning
// Run when that failed, with its Trace, and with a Plan Proposal pending or a Plan, the
// Cycle's Planning Notes, newest first.
type Planning struct {
	Proposal, Plan *Proposal
	Going          int64
	Busy           bool
	Failed         *runs.Run
	Notes          []string
}

// PlanningOf is how planning the signed-in User's Cycle `c` stands, its days in `zone`:
// its Plan as far as `c` lasts, Until the next Cycle's Delivery arrives.
func (s *Service) PlanningOf(ctx context.Context, c Cycle, zone *time.Location) (Planning, error) {
	id := c.ID
	var p Planning
	var pending *storedProposal
	var own, earlier []*storedProposal
	var last lastRun
	var portions []domain.Portion
	var stored []storedDraft
	l, _, err := s.profile.Language(ctx)
	if err != nil {
		return p, err
	}
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		if p.Going, err = goingRun(ctx, tx, id); err != nil {
			return err
		}
		if p.Busy, err = planningBusy(ctx, tx, 0); err != nil {
			return err
		}
		if last, err = loadLastRun(ctx, tx, id, zone); err != nil {
			return err
		}
		if pending, err = loadPending(ctx, tx, id, zone); err != nil {
			return err
		}
		if own, err = loadPlanOf(ctx, tx, id, zone); err != nil || (pending == nil && len(own) == 0) {
			return err
		}
		first := pending
		if len(own) > 0 {
			first = own[0]
		}
		if earlier, err = loadApproved(ctx, tx, first.first.On.AddDate(0, 0, -7), zone); err != nil {
			return err
		}
		if p.Notes, err = loadNotes(ctx, tx, id); err != nil {
			return err
		}
		if pending != nil {
			if stored, err = loadDrafts(ctx, tx, pending.id); err != nil {
				return err
			}
		}
		portions, err = ledger.LoadPortions(ctx, tx)
		return err
	})
	if err != nil {
		return p, err
	}
	if last.status == runs.Failed {
		run, _, err := s.runs.Find(ctx, last.id)
		if err != nil {
			return p, err
		}
		p.Failed = &run
	}
	if pending == nil && len(own) == 0 {
		return p, nil
	}
	library, err := s.recipes.Search(ctx, "")
	if err != nil {
		return p, err
	}
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return p, err
	}
	times, err := s.profile.SlotTimes(ctx)
	if err != nil {
		return p, err
	}
	// Worked out again from today's Recipes, as the Verdict's days were: editing a Recipe
	// changes only what is yet to be recorded.
	plan := cyclePlan(c, own, library, known, portions, times)
	before := approvedPlan(earlier, library, known, portions)
	// Every approved Plan's Planned Portions too: the forecast carry-over a Plan puts
	// in Slots.
	named := slices.Concat(portions, plan.Planned(), before.Planned())
	if pending != nil {
		drafted, err := readDrafts(l, known, stored)
		if err != nil {
			return p, err
		}
		p.Proposal = pending.resolve(library, known, named, drafted)
		if len(own) > 0 {
			p.Proposal.Changed, p.Proposal.ChangedSessions = p.Proposal.Plan.Changes(plan, pending.first, named)
			// Approved once the Delivery has arrived, it keeps the Plan's freezer splits.
			if c.Delivery.Arrives().After(s.clock.Now()) {
				p.Proposal.ChangedSplits, p.Proposal.SplitsChanged = p.Proposal.Plan.SplitChanges(plan)
			}
		}
	}
	if len(own) > 0 {
		first := own[0].first
		proposal := own[len(own)-1].resolve(library, known, portions, nil)
		proposal.Plan, proposal.Kept = plan, keptOf(plan.Kept, named)
		planned := plan.Eaten(named)
		_, proposal.Days = domain.PlanDays(before.Eaten(named), planned, first, lastDay(planned, first.On))
		p.Plan = proposal
	}
	return p, nil
}

// Drafted is the Recipe Draft `id` of the signed-in User's pending Plan Proposal, for
// them speaking in their Language, with the id of the Cycle it plans, or ErrNoProposal
// when that doesn't prepare it.
func (s *Service) Drafted(ctx context.Context, id int64) (rec domain.Recipe, cycleID int64, err error) {
	l, _, err := s.profile.Language(ctx)
	if err != nil {
		return rec, 0, err
	}
	var stored []storedDraft
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		proposalID, err := pendingID(ctx, tx)
		if err != nil {
			return err
		}
		if cycleID, err = cycleOfProposal(ctx, tx, proposalID); err != nil {
			return err
		}
		stored, err = loadDrafts(ctx, tx, proposalID)
		return err
	})
	if err != nil {
		return rec, 0, err
	}
	i := slices.IndexFunc(stored, func(d storedDraft) bool { return d.id == id })
	if i < 0 {
		return rec, 0, ErrNoProposal
	}
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return rec, 0, err
	}
	d, err := recipes.ReadStored(l, known, id, stored[i].doc)
	return d.Recipe, cycleID, err
}

// readDrafts are the Recipe Drafts `stored`, read for a User speaking `l` against their
// Ingredients, `known`.
func readDrafts(l domain.Language, known []domain.Ingredient, stored []storedDraft) ([]recipes.Drafted, error) {
	drafted := make([]recipes.Drafted, len(stored))
	for i, d := range stored {
		var err error
		if drafted[i], err = recipes.ReadStored(l, known, d.id, d.doc); err != nil {
			return nil, err
		}
	}
	return drafted, nil
}

// lastDay is the last day `planned` puts a Portion in a Slot on, `first` when none
// is later.
func lastDay(planned []domain.Eaten, first time.Time) time.Time {
	last := first
	for _, e := range planned {
		if e.At.On.After(last) {
			last = e.At.On
		}
	}
	return last
}

// keptOf are the Portions of `portions` that `kept` puts in Slots, by id.
func keptOf(kept []domain.Kept, portions []domain.Portion) map[int64]domain.Portion {
	out := map[int64]domain.Portion{}
	for _, portion := range portions {
		if slices.ContainsFunc(kept, func(k domain.Kept) bool { return k.Portion == portion.ID }) {
			out[portion.ID] = portion
		}
	}
	return out
}

// resolve is `sp` with its Recipes, Ingredients, Portions and Recipe Drafts found by id
// among those given.
func (sp storedProposal) resolve(library []domain.Recipe, known []domain.Ingredient, portions []domain.Portion, drafted []recipes.Drafted) *Proposal {
	recipes := map[int64]*domain.Recipe{}
	for i := range library {
		recipes[library[i].ID] = &library[i]
	}
	drafts := map[int64]*domain.Recipe{}
	for i := range drafted {
		drafts[drafted[i].ID] = &drafted[i].Recipe
	}
	ingredients := map[int64]*domain.Ingredient{}
	for i := range known {
		ingredients[known[i].ID] = &known[i]
	}
	p := &Proposal{
		ID: sp.id, Summary: sp.summary, Plan: domain.Plan{Kept: sp.kept}, Kept: keptOf(sp.kept, portions),
		Targets: sp.targets, Misses: sp.misses, Trailing: sp.trailing, Days: sp.days, Drafted: drafted,
	}
	for _, ps := range sp.sessions {
		ps.Recipe, ps.Ingredient = cmp.Or(recipes[ps.recipeID], drafts[ps.draftID]), ingredients[ps.ingredientID]
		p.Plan.PrepSessions = append(p.Plan.PrepSessions, ps.PrepSession)
	}
	for _, fs := range sp.splits {
		if ing := ingredients[fs.Ingredient.ID]; ing != nil {
			p.Plan.FreezerSplits = append(p.Plan.FreezerSplits, domain.RecipeIngredient{Ingredient: *ing, Quantity: fs.Quantity})
		}
	}
	return p
}
