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

// ErrSlotUnavailable means the Slot can't be logged ahead: it is past, it
// isn't one of the Cycle's Slots, or the Cycle has closed.
var ErrSlotUnavailable = errors.New("slot can't be logged ahead")

// CycleExceptions is a Cycle's Slots, each with its Exception if it has
// one.
type CycleExceptions struct {
	Cycle Cycle
	Slots []ExceptionSlot
	// MealOut is the Profile's default Meal Out estimate.
	MealOut domain.Nutrition
}

// ExceptionSlot is one of a Cycle's Slots and its Exception, or nil when
// the Plan fills it.
type ExceptionSlot struct {
	Slot      domain.Slot
	Exception *domain.Exception
	// ToCome is whether the Slot can still be logged ahead.
	ToCome bool
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
				es := ExceptionSlot{Slot: slot, ToCome: c.Status != "closed" && slot.ToCome(s.now(), p.SlotTimes)}
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

// LogException logs e for slot, one of the Slots still to come of the
// current or upcoming Cycle with id, or, when e is nil, removes the one it
// has. While no Plan relies on it, it is input to the next planning Run;
// once the Cycle's Plan does, the Plan needs a new Plan Proposal, and the
// one pending is rejected.
func (s *Service) LogException(ctx context.Context, id int64, slot domain.Slot, e *domain.Exception) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	p, _, err := s.profile.Get(ctx)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
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
		case c.Status == "closed" || !c.Contains(slot) || !slot.ToCome(s.now(), p.SlotTimes):
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
		relied, err := hasPlan(ctx, tx, c.ID)
		if err != nil || !relied {
			return err
		}
		if err := setNeedsProposal(ctx, tx, c.ID, exceptionChanged); err != nil {
			return err
		}
		return rejectPending(ctx, tx, c.ID, exceptionChanged+": "+slot.String()+" is now "+describeSlotState(e)+".")
	})
}

// between loads the Exceptions from the day first on whose Slots no
// Cycle holds, as Slots at times.
func (s *Service) between(ctx context.Context, q db.Querier, first time.Time, times domain.SlotTimes) ([]ExceptionSlot, error) {
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
			between = append(between, ExceptionSlot{Slot: e.Slot, Exception: &e, ToCome: e.Slot.ToCome(s.now(), times)})
		}
	}
	return between, nil
}

// RemoveExceptionBetweenCycles removes the Exception of slot, a Slot still
// to come that no Cycle holds; one a Cycle holds is changed on its
// Exceptions page.
func (s *Service) RemoveExceptionBetweenCycles(ctx context.Context, slot domain.Slot) error {
	if err := s.Arrive(ctx); err != nil {
		return err
	}
	p, _, err := s.profile.Get(ctx)
	if err != nil {
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
		if !slot.ToCome(s.now(), p.SlotTimes) || slices.ContainsFunc(cycles, func(c Cycle) bool { return c.Contains(slot) }) {
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

// describeSlotState is what fills a Slot with Exception e, or with none.
func describeSlotState(e *domain.Exception) string {
	if e == nil {
		return "from the Plan"
	}
	return e.Kind.Describe()
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
			n := e.Nutrition(mealOut)
			fmt.Fprintf(&b, "- %s: Meal Out, estimated %d kcal, %d g protein, %d g carbs, %d g fat.\n", e.Slot, n.Kcal, n.ProteinG, n.CarbsG, n.FatG)
			continue
		}
		fmt.Fprintf(&b, "- %s: skipped, counts nothing.\n", e.Slot)
	}
	return strings.TrimSpace(b.String())
}
