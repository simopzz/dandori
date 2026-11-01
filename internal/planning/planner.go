package planning

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/runs"
)

// planningKind is the kind of Run that plans a Cycle: it submits its
// initial Plan Proposal.
const planningKind = "planning"

// ErrCannotPlan means the Cycle can't be planned now; a CannotPlanError
// says why.
var ErrCannotPlan = errors.New("cycle can't be planned")

// CannotPlanError means the Cycle can't be planned now. Reason says why,
// for the user; RunID is the planning Run already going for it, if that is
// why. WaitsTurn is whether another Cycle's pending Plan Proposal or
// planning Run is why: planning waits its turn.
type CannotPlanError struct {
	Reason    string
	RunID     int64
	WaitsTurn bool
}

func (e *CannotPlanError) Error() string        { return ErrCannotPlan.Error() + ": " + e.Reason }
func (e *CannotPlanError) Is(target error) bool { return target == ErrCannotPlan }

// ErrNoteRequired means changes to a Plan were asked for without a
// Planning Note saying what to change.
var ErrNoteRequired = errors.New("asking for changes needs a planning note")

// canPlan reports whether a planning Run can start for c now, for what ask
// says, and waits, why not when another Cycle's turn comes first.
func (s *Service) canPlan(ctx context.Context, q db.Querier, c Cycle, ask asking) (can bool, waits string, err error) {
	err = s.turn(ctx, q, c, domain.FirstUnprepared(c.Start, s.today()), ask)
	if errors.Is(err, ErrCannotPlan) {
		return false, waitsTurn(err), nil
	}
	return err == nil, "", err
}

// StartPlanning starts a planning Run for the Cycle with id, returning the
// Run's id. The Run submits the Cycle's initial Plan Proposal.
func (s *Service) StartPlanning(ctx context.Context, id int64) (int64, error) {
	var started runs.Recorded
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		started, err = s.startPlanningIn(ctx, tx, id, askPlan, 0, "", "")
		return err
	})
	if err != nil {
		return 0, err
	}
	s.runs.Launch(started)
	return started.ID, nil
}

// AskForChanges starts a planning Run for changes to the Plan of the
// Cycle with id, told note, the Planning Note saying what to change, and
// returns the Run's id. It plans from the first Slot not yet prepared; the
// Plan stays as it is until its Plan Proposal is approved.
func (s *Service) AskForChanges(ctx context.Context, id int64, note string) (int64, error) {
	if note == "" {
		return 0, ErrNoteRequired
	}
	var started runs.Recorded
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		started, err = s.startPlanningIn(ctx, tx, id, askChanges, 0, note, "")
		return err
	})
	if err != nil {
		return 0, err
	}
	s.runs.Launch(started)
	return started.ID, nil
}

// startPlanningIn records a planning Run for the Cycle with id, for what
// ask says, as part of tx, after the rejection of the Plan Proposal with
// id after when it isn't 0, for the caller to launch once tx commits.
// note, when given, joins the Cycle's Planning Notes once the Run can
// start. missing, when given, is what Esselunga is out of, which Cart Fill
// asks it to replan around: its Plan Proposal is a substitution.
func (s *Service) startPlanningIn(ctx context.Context, tx pgx.Tx, id int64, ask asking, after int64, note, missing string) (runs.Recorded, error) {
	// Serialises with another start on either Cycle, with the planner
	// submitting, and with what replaces Plan Proposals.
	if err := lockCycles(ctx, tx); err != nil {
		return runs.Recorded{}, err
	}
	c, err := findCycle(ctx, tx, id)
	if err != nil {
		return runs.Recorded{}, err
	}
	if c == nil {
		return runs.Recorded{}, ErrNoCycle
	}
	// What is prepared today is taken as done: a Cycle whose first day has
	// come is planned from tomorrow.
	from := domain.FirstUnprepared(c.Start, s.today())
	if err := s.turn(ctx, tx, *c, from, ask); err != nil {
		return runs.Recorded{}, err
	}
	if note != "" {
		if err := insertNote(ctx, tx, id, note); err != nil {
			return runs.Recorded{}, err
		}
	}
	notes, err := loadNotes(ctx, tx, id)
	if err != nil {
		return runs.Recorded{}, err
	}
	brief, err := s.profile.Brief(ctx)
	if err != nil {
		return runs.Recorded{}, err
	}
	replacing, err := s.describeReplaced(ctx, tx, *c, from)
	if err != nil {
		return runs.Recorded{}, err
	}
	run, err := s.runs.Record(ctx, tx, planningKind, s.planningJob(*c, from, brief, notes, replacing, missing))
	if err != nil {
		return runs.Recorded{}, err
	}
	return run, insertPlanningRun(ctx, tx, run.ID, id, after, missing)
}

// lastSlot is the Cycle's last Slot.
func lastSlot(c domain.Cycle) domain.Slot {
	u := c.Until()
	if i := slices.Index(domain.SlotKinds, u.Kind); i > 0 {
		return domain.Slot{On: u.On, Kind: domain.SlotKinds[i-1]}
	}
	return domain.Slot{On: u.On.AddDate(0, 0, -1), Kind: domain.SlotKinds[len(domain.SlotKinds)-1]}
}

// plannerSystem is the planning Run's instructions; %[1]s is the name of
// the Content Language and %[2]d how many Plans breaking the rules it may
// submit.
const plannerSystem = `You plan a Cycle of the user's meals: the stretch one supermarket Delivery has to last. You submit its Plan, which the user approves or rejects.

A Plan is Prep Sessions, each preparing food at one Slot (an eating occasion: breakfast, lunch, snack or dinner on a day) and yielding Portions, each eaten at a Slot.
- Fill every Slot from the first one you plan to the Cycle's last with exactly one Portion, except a Slot with an Exception: it gets no Portion and no Prep Session. A Meal Out counts toward the Targets with its estimate; a skipped Slot counts nothing.
- A Prep Session at dinner yields three Portions by default: one eaten then, one at lunch two days later, and a third two to three days after that, typically frozen. Other Prep Sessions, such as a quick breakfast or a snack, yield what their planned eating needs, often one Portion eaten straight away. Portions of one Prep Session are never eaten within two days of each other.
- A Prep Session prepares a Recipe (recipe_id), a Recipe Draft you drafted (draft_id), or Ingredients eaten as they are (ingredients, by their exact names, quantities in the Ingredient's unit for all its Portions together), never more than one of them. A Recipe is scaled to the Portions the Prep Session yields.
- Never yield more Portions than there is eating planned for. A Portion whose eating falls after the Cycle is still given its eat_on and eat_kind: it stays without a Slot and carries over to the next Cycle.
- Give each Portion a Storage Location (pantry, fridge or freezer) that fits what it is and how long it must keep.
- Put every Portion without a Slot in a Slot (kept) before scheduling new Prep Sessions, and use Stock Items first, those that would spoil first, judged from the Ingredient and its Storage Location, early in the Cycle.
- Design Prep Sessions around whole packs of the Known Catalog, so little is left over.
- Keep the Targets over every trailing seven days, the Untracked Allowance included, and never use an Ingredient the user excludes or is allergic to.
- When the Recipe Library has nothing that fits, draft a new Recipe with draft_recipe; the user accepts it into the Recipe Library.

Write every food term in %[1]s.

Read the Profile, the Exceptions, the kitchen, the Recipe Library and the Known Catalog with your tools, then call submit_plan once with the whole Plan. If it breaks rules, you are told which: fix them and submit again. After %[2]d Plans that break rules the Run fails. Once a Plan is stored, end your turn with a short summary of it for the user.`

// describeReplaced is what of c's Plan a Plan Proposal from the Slot from
// on replaces, for the planner: empty while c has no Plan.
func (s *Service) describeReplaced(ctx context.Context, q db.Querier, c Cycle, from domain.Slot) (string, error) {
	if planned, err := hasPlan(ctx, q, c.ID); err != nil || !planned {
		return "", err
	}
	plan, _, err := loadPlan(ctx, q, c.ID)
	if err != nil {
		return "", err
	}
	r, err := loadReplacement(ctx, q, c, from, s.today())
	if err != nil {
		return "", err
	}
	// Where each Portion freed is eaten now.
	slotOf := map[int64]domain.Slot{}
	for _, ps := range plan.PrepSessions {
		for _, p := range ps.Portions {
			if p.EatAt != nil {
				slotOf[p.ID] = *p.EatAt
			}
		}
	}
	for _, k := range plan.Kept {
		slotOf[k.PortionID] = k.Slot
	}
	freed := slices.Clone(r.Kept)
	freed = slices.DeleteFunc(freed, func(k domain.KeptPortion) bool { return !slices.Contains(r.Freed, k.ID) })
	slices.SortFunc(freed, func(a, b domain.KeptPortion) int {
		switch {
		case slotOf[a.ID].Before(slotOf[b.ID]):
			return -1
		case slotOf[b.ID].Before(slotOf[a.ID]):
			return 1
		}
		return cmp.Compare(a.ID, b.ID)
	})

	var b strings.Builder
	fmt.Fprintf(&b, "The Cycle already has a Plan. Yours replaces it from %s on; what comes before stays.\n", from)
	if len(freed) > 0 {
		b.WriteString("From then on, these Portions already prepared fill its Slots; they are yours to put in Slots again (list_kitchen lists them):\n")
		for _, k := range freed {
			fmt.Fprintf(&b, "- %s: Portion %d, %s\n", slotOf[k.ID], k.ID, k.Name)
		}
	}
	var dropped []string
	for _, ps := range plan.PrepSessions {
		if !slices.Contains(r.Dropped, ps.ID) {
			continue
		}
		var eaten []string
		for _, p := range ps.Portions {
			if p.EatAt == nil {
				eaten = append(eaten, "without a Slot")
			} else {
				eaten = append(eaten, p.EatAt.String())
			}
		}
		dropped = append(dropped, fmt.Sprintf("- %s at %s, eaten at %s", ps.Name(), ps.At, strings.Join(eaten, ", ")))
	}
	if len(dropped) > 0 {
		b.WriteString("And these Prep Sessions, which yours replaces:\n" + strings.Join(dropped, "\n") + "\n")
	}
	b.WriteString("Change only what the notes ask for, and keep the rest of it as it is where the rules allow.")
	return b.String(), nil
}

// planningJob is the planning Run of c from the Slot from on, told the
// Planning Brief, notes, the Planning Notes of c, newest first,
// replacing, what of c's Plan it replaces, if c has one, and missing, what
// Esselunga is out of, if Cart Fill found anything.
func (s *Service) planningJob(c Cycle, from domain.Slot, brief string, notes []string, replacing, missing string) agent.Job {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Plan the Cycle delivered on %s. Its Slots run from %s to %s.\n",
		describeDelivery(c.Delivery, longDate), c.Start, lastSlot(c.Cycle))
	fmt.Fprintf(&prompt, "Plan every Slot from %s on. Today is %s.\n", from, s.today().Format(longDate))
	fmt.Fprintf(&prompt, "Its trailing seven days reach from %s to %s: list the Exceptions over those days.\n",
		c.Start.On.AddDate(0, 0, -6).Format(dateLayout), c.LastDay().Format(dateLayout))
	if brief != "" {
		fmt.Fprintf(&prompt, "\nThe user's Planning Brief, their lasting wishes for how their meals are planned. Follow it as far as the rules of the Plan allow; where one of this Cycle's notes disagrees with it, the note wins:\n%s\n", brief)
	}
	if len(notes) > 0 {
		prompt.WriteString("\nRejecting this Cycle's Plan Proposals or asking for changes to its Plan, the user gave these notes, newest first. Take them into account; where two disagree, the newer one wins:\n")
		for _, n := range notes {
			fmt.Fprintf(&prompt, "- %s\n", n)
		}
	}
	if replacing != "" {
		prompt.WriteString("\n" + replacing + "\n")
	}
	if missing != "" {
		prompt.WriteString("\nFilling the cart for its Delivery, Esselunga is out of these, so the Delivery can't bring them. Plan around them with Catalog Products available or what the kitchen holds, and change only what that needs:\n" + missing + "\n")
	}

	submitted := false
	return agent.Job{
		System: fmt.Sprintf(plannerSystem, s.language.Name(), s.attempts),
		Prompt: strings.TrimSpace(prompt.String()),
		// The Plan is long and thinking shares its token budget: at the
		// default effort the model can think the whole budget away.
		Effort: "medium",
		Tools: []agent.Tool{
			s.profile.Tool(),
			s.ExceptionsTool(),
			s.kitchenTool(c.ID, from),
			s.recipes.SearchTool(),
			s.catalog.Tool(),
			s.recipes.DraftTool(),
			s.submitTool(c.ID, from, &submitted),
		},
		Finish: func(context.Context, agent.Trace) error {
			if !submitted {
				return errors.New("the planner ended without submitting a Plan")
			}
			return nil
		},
	}
}

// kitchenTool lists what the kitchen will hold that a Plan of the Cycle
// with id from the Slot from on can use: the Portions it may put in Slots,
// the Stock Items left at its first Slot not yet prepared once the Prep
// Sessions before it have used theirs up, and, once its Delivery is ordered
// and until it arrives, what it brings: the Plan makes do with that.
func (s *Service) kitchenTool(id int64, from domain.Slot) agent.Tool {
	return agent.Tool{
		Name:        "list_kitchen",
		Description: "List what the kitchen will hold when the Plan starts: the Portions without a Slot the Plan may put in Slots (kept), by id, the Stock Items left once the Prep Sessions before it have used theirs up, each with its Storage Location, and what an ordered Delivery still to arrive brings.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {}}`),
		Run: func(ctx context.Context, _ agent.Trace, _ json.RawMessage) (string, error) {
			c, err := loadCycle(ctx, s.pool, id)
			if err != nil {
				return "", err
			}
			r, err := loadReplacement(ctx, s.pool, c, from, s.today())
			if err != nil {
				return "", err
			}
			items, err := s.stockAt(ctx, s.pool, r.Prepared)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			b.WriteString("Portions without a Slot:\n")
			if len(r.Kept) == 0 {
				b.WriteString("(none)\n")
			}
			for _, k := range r.Kept {
				fmt.Fprintf(&b, "- Portion %d: %s, %s\n", k.ID, k.Name, k.Nutrition.Describe())
			}
			b.WriteString("\nStock Items:\n")
			s.describeStock(&b, items)
			if c.Status == "upcoming" && c.Delivery.Ordered {
				list, _, err := s.groceryListOf(ctx, s.pool, c)
				if err != nil {
					return "", err
				}
				fmt.Fprintf(&b, "\nOrdered, arriving with the Delivery on %s: buy nothing more for it.\n", c.Delivery.On.Format(longDate))
				s.describeStock(&b, list.StockItems())
			}
			return strings.TrimSpace(b.String()), nil
		},
	}
}

// describeStock writes items to b, one a line, for the planner.
func (s *Service) describeStock(b *strings.Builder, items []domain.StockItem) {
	if len(items) == 0 {
		b.WriteString("(none)\n")
	}
	for _, it := range items {
		fmt.Fprintf(b, "- %g %s %s in the %s\n", it.Quantity, it.Ingredient.Unit, it.Ingredient.Name, it.Location)
	}
}

var submitPlanSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "prep_sessions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "on": {"type": "string", "description": "The day it is prepared, YYYY-MM-DD."},
          "kind": {"type": "string", "enum": ["breakfast", "lunch", "snack", "dinner"], "description": "The Slot it is prepared at."},
          "recipe_id": {"type": "integer", "description": "The Recipe it prepares."},
          "draft_id": {"type": "integer", "description": "The Recipe Draft it prepares."},
          "ingredients": {
            "type": "array",
            "description": "Ingredients set out as they are, for all its Portions together, when it prepares no Recipe.",
            "items": {
              "type": "object",
              "properties": {"name": {"type": "string"}, "quantity": {"type": "number", "exclusiveMinimum": 0}},
              "required": ["name", "quantity"]
            }
          },
          "portions": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "location": {"type": "string", "enum": ["pantry", "fridge", "freezer"]},
                "eat_on": {"type": "string", "description": "The day it is eaten, YYYY-MM-DD."},
                "eat_kind": {"type": "string", "enum": ["breakfast", "lunch", "snack", "dinner"]}
              },
              "required": ["location", "eat_on", "eat_kind"]
            }
          }
        },
        "required": ["on", "kind", "portions"]
      }
    },
    "kept": {
      "type": "array",
      "description": "Portions without a Slot put in Slots.",
      "items": {
        "type": "object",
        "properties": {
          "portion_id": {"type": "integer"},
          "on": {"type": "string", "description": "YYYY-MM-DD"},
          "kind": {"type": "string", "enum": ["breakfast", "lunch", "snack", "dinner"]}
        },
        "required": ["portion_id", "on", "kind"]
      }
    }
  },
  "required": ["prep_sessions"]
}`)

// submitTool submits the Plan of the Cycle with id from the Slot from on.
// A Plan that keeps every rule is stored as its initial Plan Proposal and
// sets submitted; one that breaks them goes back to the model, until it
// has submitted as many as the Service allows.
func (s *Service) submitTool(id int64, from domain.Slot, submitted *bool) agent.Tool {
	failed := 0
	return agent.Tool{
		Name:        "submit_plan",
		Description: fmt.Sprintf("Submit the Plan. It is checked against the rules of the Plan: one that keeps them is stored as a Plan Proposal for the user; one that breaks them comes back with the rules it breaks. After %d that break rules the Run fails.", s.attempts),
		InputSchema: submitPlanSchema,
		Run: func(ctx context.Context, tr agent.Trace, input json.RawMessage) (string, error) {
			if *submitted {
				return "", errors.New("a Plan Proposal is already stored: end your turn")
			}
			problems, err := s.propose(ctx, id, from, runs.IDFrom(ctx), input)
			if err != nil {
				return "", err
			}
			if len(problems) == 0 {
				*submitted = true
				return "The Plan keeps every rule and is stored as a Plan Proposal for the user to approve. End your turn with a short summary of it.", nil
			}
			failed++
			list := "- " + strings.Join(problems, "\n- ")
			if failed >= s.attempts {
				return "", agent.Stop(fmt.Errorf("the planner's Plan still broke the rules after %d attempts:\n%s", failed, list))
			}
			tr.Emit(agent.Event{
				Kind:    agent.EventRetry,
				Summary: fmt.Sprintf("The Plan breaks the rules: attempt %d of %d", failed+1, s.attempts),
				Detail:  list,
			})
			return "", fmt.Errorf("the Plan breaks these rules; fix them and submit the whole Plan again (attempt %d of %d):\n%s", failed+1, s.attempts, list)
		},
	}
}

// propose checks input, the Plan the planner submitted in the Run with
// run, as a Plan Proposal for the Cycle with id from the Slot from on, and
// stores it if it keeps every rule. It returns the problems with it: why
// it can't be read, or the rules it breaks.
func (s *Service) propose(ctx context.Context, id int64, from domain.Slot, run int64, input json.RawMessage) ([]string, error) {
	var plan map[string]json.RawMessage
	if err := json.Unmarshal(input, &plan); err != nil {
		return []string{"the Plan does not match the schema: " + err.Error()}, nil
	}
	// The Plan starts where the Run was told to plan from.
	start, err := json.Marshal(map[string]any{"on": from.On.Format(dateLayout), "kind": from.Kind})
	if err != nil {
		return nil, err
	}
	plan["from"] = start
	raw, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	var problems []string
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockCycle(ctx, tx, id); err != nil {
			return err
		}
		c, err := findCycle(ctx, tx, id)
		if err != nil {
			return err
		}
		if c == nil {
			return agent.Stop(errors.New("the Cycle was dropped"))
		}
		problems, err = s.submit(ctx, tx, *c, from, run, raw)
		return err
	})
	return problems, err
}

// Planning is how planning a Cycle stands.
type Planning struct {
	// CanPlan is whether a planning Run can start for its initial Plan
	// now; Waits why not, when another Cycle's turn comes first.
	CanPlan bool
	Waits   string
	// CanAsk is whether changes to its Plan can be asked for now.
	CanAsk bool
	// From is the first Slot a planning Run starting now plans from.
	From domain.Slot
	// Run is its last planning Run, with its Trace; nil when it had none.
	Run *runs.Run
}

// planning loads how planning c stands.
func (s *Service) planning(ctx context.Context, q db.Querier, c Cycle) (Planning, error) {
	p := Planning{From: domain.FirstUnprepared(c.Start, s.today())}
	var err error
	if p.CanPlan, p.Waits, err = s.canPlan(ctx, q, c, askPlan); err != nil {
		return p, err
	}
	if p.CanAsk, _, err = s.canPlan(ctx, q, c, askChanges); err != nil {
		return p, err
	}
	id, found, err := latestPlanningRun(ctx, q, c.ID)
	if err != nil || !found {
		return p, err
	}
	run, found, err := s.runs.Get(ctx, id)
	if err != nil || !found {
		return p, err
	}
	if run.Events, err = s.runs.Trace(ctx, id, 0); err != nil {
		return p, err
	}
	p.Run = &run
	return p, nil
}
