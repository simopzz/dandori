package planning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// ErrNoCycle means there is no Cycle with that id.
var ErrNoCycle = errors.New("no such cycle")

// ErrSlotUnavailable means the Slot isn't one of the Cycle's, or, for
// one between Cycles, a Cycle holds it now.
var ErrSlotUnavailable = errors.New("slot is not the cycle's")

// CycleExceptions is a Cycle's Slots, each with its Exception if it has
// one.
type CycleExceptions struct {
	Cycle Cycle
	Slots []ExceptionSlot
	// MealOut is the Profile's default Meal Out estimate.
	MealOut domain.Nutrition
	// CanPlan is whether a planning Run can start for the Cycle now.
	CanPlan bool
}

// ExceptionSlot is one of a Cycle's Slots and its Exception, or nil when
// the Plan fills it.
type ExceptionSlot struct {
	Slot      domain.Slot
	Exception *domain.Exception
}

// Exceptions loads the Cycle with id and every one of its Slots, with
// their Exceptions.
func (s *Service) Exceptions(ctx context.Context, id int64) (CycleExceptions, error) {
	var ce CycleExceptions
	if err := s.Arrive(ctx); err != nil {
		return ce, err
	}
	p, _, err := s.profile.Get(ctx)
	if err != nil {
		return ce, err
	}
	ce.MealOut = p.MealOut
	err = pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		c, err := findCycle(ctx, tx, id)
		if err != nil {
			return err
		}
		if c == nil {
			return ErrNoCycle
		}
		ce.Cycle = *c
		if ce.CanPlan, err = s.canPlan(ctx, tx, *c); err != nil {
			return err
		}
		exceptions, err := loadExceptions(ctx, tx, c.Start.On, c.LastDay())
		if err != nil {
			return err
		}
		byKey := map[string]domain.Exception{}
		for _, e := range exceptions {
			byKey[slotID(e.Slot)] = e
		}
		for d := c.Start.On; !d.After(c.LastDay()); d = d.AddDate(0, 0, 1) {
			for _, k := range domain.SlotKinds {
				slot := domain.Slot{On: d, Kind: k}
				if !c.Contains(slot) {
					continue
				}
				es := ExceptionSlot{Slot: slot}
				if e, ok := byKey[slotID(slot)]; ok {
					es.Exception = &e
				}
				ce.Slots = append(ce.Slots, es)
			}
		}
		return nil
	})
	return ce, err
}

// LogException logs e for slot, one of the Slots of the Cycle with id, or,
// when e is nil, removes the one it has: ahead, or, once past, as a
// correction of what happened. While no Plan relies on it, it is input to
// the next planning Run. Once the Cycle's Plan does, or, when the Cycle
// has closed, the current one's, which took over what was left of it,
// that Plan needs a new Plan Proposal: what was eaten instead may have
// been meant for a Slot still to come. Either way it replaces the pending
// Plan Proposal and cancels the planning Run going: of the upcoming Cycle
// alone for one of its Slots, else of either, since what the current
// Cycle eats changes what carries over.
func (s *Service) LogException(ctx context.Context, id int64, slot domain.Slot, e *domain.Exception) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return s.write(ctx, func(tx pgx.Tx, cancelled *cancels) error {
		// Serialises with the arrival, move or drop of the Cycle.
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		c, err := findCycle(ctx, tx, id)
		switch {
		case err != nil:
			return err
		case c == nil:
			return ErrNoCycle
		case !c.Contains(slot):
			return ErrSlotUnavailable
		}
		logged, err := loadExceptions(ctx, tx, slot.On, slot.On)
		if err != nil {
			return err
		}
		var was *domain.Exception
		for _, l := range logged {
			if l.Slot.Kind == slot.Kind {
				was = &l
			}
		}
		if sameException(was, e) {
			return nil
		}
		if err := saveException(ctx, tx, slot, e); err != nil {
			return err
		}
		replaced := []int64{c.ID}
		if c.Status != "upcoming" {
			if replaced, err = openCycleIDs(ctx, tx); err != nil {
				return err
			}
		}
		if err := s.replace(ctx, tx, cancelled, replaced, "Replaced by the Exception on "+slot.On.Format(shortDate)+", "+string(slot.Kind)); err != nil {
			return err
		}
		if c.Status == "closed" {
			if c, err = loadCycleWith(ctx, tx, "current"); err != nil || c == nil {
				return err
			}
		}
		relied, err := hasPlan(ctx, tx, c.ID)
		if err != nil || !relied {
			return err
		}
		return setNeedsProposal(ctx, tx, c.ID, exceptionChanged)
	})
}

// between loads the Exceptions from the day first on whose Slots no
// Cycle holds.
func (s *Service) between(ctx context.Context, q db.Querier, first time.Time) ([]ExceptionSlot, error) {
	exceptions, err := loadExceptionsFrom(ctx, q, first)
	if err != nil {
		return nil, err
	}
	cycles, err := loadCyclesFrom(ctx, q, first)
	if err != nil {
		return nil, err
	}
	var between []ExceptionSlot
	for _, e := range exceptions {
		if !slices.ContainsFunc(cycles, func(c Cycle) bool { return c.Contains(e.Slot) }) {
			between = append(between, ExceptionSlot{Slot: e.Slot, Exception: &e})
		}
	}
	return between, nil
}

// RemoveExceptionBetweenCycles removes the Exception of slot, a Slot no
// Cycle holds; one a Cycle holds is changed on its Exceptions page.
func (s *Service) RemoveExceptionBetweenCycles(ctx context.Context, slot domain.Slot) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialises with the opening, move or drop of a Cycle.
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		cycles, err := loadCyclesFrom(ctx, tx, slot.On)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(cycles, func(c Cycle) bool { return c.Contains(slot) }) {
			return ErrSlotUnavailable
		}
		return saveException(ctx, tx, slot, nil)
	})
}

// slotID names s by its day and kind, e.g. "2026-11-07-lunch".
func slotID(s domain.Slot) string { return s.On.Format(time.DateOnly) + "-" + string(s.Kind) }

// sameException reports whether a and b, either nil for none, log their
// Slot the same way.
func sameException(a, b *domain.Exception) bool {
	if a == nil || b == nil {
		return a == b
	}
	same := func(x, y *int) bool { return x == nil && y == nil || x != nil && y != nil && *x == *y }
	ea, eb := a.Estimate, b.Estimate
	return a.Kind == b.Kind && same(ea.Kcal, eb.Kcal) && same(ea.ProteinG, eb.ProteinG) && same(ea.CarbsG, eb.CarbsG) && same(ea.FatG, eb.FatG)
}

// ExceptionsTool lists the Exceptions over a stretch of days as an agent
// tool, for the planner: what it plans around.
func (s *Service) ExceptionsTool() agent.Tool {
	return agent.Tool{
		Name: "list_exceptions",
		Description: "List the Slots logged as a Meal Out or skipped from one day to another, by default the days the next Plan's trailing seven days reach: " +
			"from six days before the upcoming Cycle's first day (or the current one's, when none is upcoming) to its last. " +
			"An excepted Slot gets no Portion and no Prep Session. A Meal Out counts toward the Targets with its estimate; a skipped Slot counts nothing.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {
			"from": {"type": "string", "description": "The first day, YYYY-MM-DD."},
			"to": {"type": "string", "description": "The last day, YYYY-MM-DD."}}}`),
		Run: func(ctx context.Context, _ agent.Trace, input json.RawMessage) (string, error) {
			var in struct{ From, To string }
			if err := json.Unmarshal(input, &in); err != nil {
				return "", fmt.Errorf("the input does not match the schema: %w", err)
			}
			from, to, err := s.exceptionDays(ctx, in.From, in.To)
			if err != nil {
				return "", err
			}
			p, _, err := s.profile.Get(ctx)
			if err != nil {
				return "", err
			}
			exceptions, err := loadExceptions(ctx, s.pool, from, to)
			if err != nil {
				return "", err
			}
			return describeExceptions(from, to, exceptions, p.MealOut), nil
		},
	}
}

// exceptionDays reads the days the Exceptions tool lists, from and to as
// typed, falling back to those the next Plan's trailing seven days reach.
func (s *Service) exceptionDays(ctx context.Context, from, to string) (time.Time, time.Time, error) {
	var first, last time.Time
	if from == "" || to == "" {
		if err := s.Arrive(ctx); err != nil {
			return first, last, err
		}
		c, err := loadCycleWith(ctx, s.pool, "upcoming")
		if err == nil && c == nil {
			c, err = loadCycleWith(ctx, s.pool, "current")
		}
		if err != nil {
			return first, last, err
		}
		if c == nil {
			return first, last, errors.New("no Cycle is open: give from and to")
		}
		first, last = c.Start.On.AddDate(0, 0, -6), c.LastDay()
	}
	for _, d := range []struct {
		typed string
		to    *time.Time
	}{{from, &first}, {to, &last}} {
		if d.typed == "" {
			continue
		}
		t, err := time.Parse(time.DateOnly, d.typed)
		if err != nil {
			return first, last, fmt.Errorf("%q is not a date, YYYY-MM-DD", d.typed)
		}
		*d.to = t
	}
	if last.Before(first) {
		return first, last, errors.New("to must not be before from")
	}
	return first, last, nil
}

// describeExceptions is exceptions, from from to to, as the model reads
// them, each Meal Out with its estimate over mealOut, the default.
func describeExceptions(from, to time.Time, exceptions []domain.Exception, mealOut domain.Nutrition) string {
	days := from.Format(shortDate) + " to " + to.Format(shortDate)
	if len(exceptions) == 0 {
		return "No Exceptions from " + days + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Exceptions from %s:\n", days)
	for _, e := range exceptions {
		if e.Kind == domain.MealOut {
			fmt.Fprintf(&b, "- %s: Meal Out, estimated %s.\n", e.Slot, e.Nutrition(mealOut).Describe())
			continue
		}
		fmt.Fprintf(&b, "- %s: skipped, counts nothing.\n", e.Slot)
	}
	return strings.TrimSpace(b.String())
}
