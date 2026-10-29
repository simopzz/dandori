package planning

import (
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
// why.
type CannotPlanError struct {
	Reason string
	RunID  int64
}

func (e *CannotPlanError) Error() string        { return ErrCannotPlan.Error() + ": " + e.Reason }
func (e *CannotPlanError) Is(target error) bool { return target == ErrCannotPlan }

// plannable checks the Cycle c can get an initial Plan, planned from the
// Slot from on: it is open, has no Plan and no Plan Proposal pending, it
// has Slots left to plan, and no planning Run is going for it. A Cycle with
// a Plan changes through a Plan Proposal of its own.
func plannable(ctx context.Context, q db.Querier, c Cycle, from domain.Slot) error {
	if c.Status == "closed" {
		return &CannotPlanError{Reason: "the next Delivery has arrived, so it is closed"}
	}
	if planned, err := hasPlan(ctx, q, c.ID); err != nil {
		return err
	} else if planned {
		return &CannotPlanError{Reason: "it already has a Plan"}
	}
	if waiting, err := hasPending(ctx, q, c.ID); err != nil {
		return err
	} else if waiting {
		return &CannotPlanError{Reason: "a Plan Proposal for it is pending: approve or reject it"}
	}
	if !c.Contains(from) {
		return &CannotPlanError{Reason: "it has no Slots left to plan"}
	}
	if run, status, err := latestPlanningRun(ctx, q, c.ID); err != nil {
		return err
	} else if status == "running" {
		return &CannotPlanError{Reason: "it is being planned", RunID: run}
	}
	return nil
}

// canPlan reports whether a planning Run can start for c now.
func (s *Service) canPlan(ctx context.Context, q db.Querier, c Cycle) (bool, error) {
	err := plannable(ctx, q, c, domain.FirstUnprepared(c.Start, s.today()))
	if errors.Is(err, ErrCannotPlan) {
		return false, nil
	}
	return err == nil, err
}

// StartPlanning starts a planning Run for the Cycle with id, returning the
// Run's id. The Run submits the Cycle's initial Plan Proposal.
func (s *Service) StartPlanning(ctx context.Context, id int64) (int64, error) {
	if err := s.Arrive(ctx); err != nil {
		return 0, err
	}
	var run int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		run, err = s.startPlanningIn(ctx, tx, id, 0)
		return err
	})
	return run, err
}

// startPlanningIn starts a planning Run for the Cycle with id as part of
// tx, after the rejection of the Plan Proposal with id after when it isn't
// 0, and returns the Run's id.
func (s *Service) startPlanningIn(ctx context.Context, tx pgx.Tx, id, after int64) (int64, error) {
	// Serialises with another start, and with the planner submitting.
	if err := lockCycle(ctx, tx, id); err != nil {
		return 0, err
	}
	c, err := findCycle(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	if c == nil {
		return 0, ErrNoCycle
	}
	// What is prepared today is taken as done: a Cycle whose first day has
	// come is planned from tomorrow.
	from := domain.FirstUnprepared(c.Start, s.today())
	if err := plannable(ctx, tx, *c, from); err != nil {
		return 0, err
	}
	notes, err := loadNotes(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	run, err := s.runs.Start(ctx, planningKind, s.planningJob(*c, from, notes))
	if err != nil {
		return 0, err
	}
	return run, insertPlanningRun(ctx, tx, run, id, after)
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
- Put every Portion without a Slot in a Slot (kept) before scheduling new Prep Sessions, and use Stock Items first, perishable ones and those near expiry early in the Cycle. A Stock Item past its expiry date is never used.
- Design Prep Sessions around whole packs of the Known Catalog, so little is left over.
- Keep the Targets over every trailing seven days, the Untracked Allowance included, and never use an Ingredient the user excludes or is allergic to.
- When the Recipe Library has nothing that fits, draft a new Recipe with draft_recipe; the user accepts it into the Recipe Library.

Write every food term in %[1]s.

Read the Profile, the Exceptions, the kitchen, the Recipe Library and the Known Catalog with your tools, then call submit_plan once with the whole Plan. If it breaks rules, you are told which: fix them and submit again. After %[2]d Plans that break rules the Run fails. Once a Plan is stored, end your turn with a short summary of it for the user.`

// planningJob is the planning Run of c from the Slot from on, told notes,
// why the user rejected earlier Plan Proposals for it, newest first.
func (s *Service) planningJob(c Cycle, from domain.Slot, notes []string) agent.Job {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Plan the Cycle delivered on %s. Its Slots run from %s to %s.\n",
		describeDelivery(c.Delivery, longDate), c.Start, lastSlot(c.Cycle))
	fmt.Fprintf(&prompt, "Plan every Slot from %s on. Today is %s.\n", from, s.today().Format(longDate))
	fmt.Fprintf(&prompt, "Its trailing seven days reach from %s to %s: list the Exceptions over those days.\n",
		c.Start.On.AddDate(0, 0, -6).Format(dateLayout), c.LastDay().Format(dateLayout))
	if len(notes) > 0 {
		prompt.WriteString("\nThe user rejected earlier Plan Proposals for this Cycle with these notes, newest first. Take them into account:\n")
		for _, n := range notes {
			fmt.Fprintf(&prompt, "- %s\n", n)
		}
	}

	submitted := false
	return agent.Job{
		System: fmt.Sprintf(plannerSystem, s.language.Name(), s.attempts),
		Prompt: strings.TrimSpace(prompt.String()),
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

// kitchenTool lists what the kitchen holds that a Plan of the Cycle with
// id from the Slot from on can use.
func (s *Service) kitchenTool(id int64, from domain.Slot) agent.Tool {
	return agent.Tool{
		Name:        "list_kitchen",
		Description: "List what the kitchen holds: the Portions without a Slot the Plan may put in Slots (kept), by id, and the Stock Items, each with its Storage Location and expiry date.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {}}`),
		Run: func(ctx context.Context, _ agent.Trace, _ json.RawMessage) (string, error) {
			kept, err := loadKept(ctx, s.pool, id, from, domain.FirstUnprepared(from, s.today()))
			if err != nil {
				return "", err
			}
			items, err := s.stock.Items(ctx)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			b.WriteString("Portions without a Slot:\n")
			if len(kept) == 0 {
				b.WriteString("(none)\n")
			}
			for _, k := range kept {
				fmt.Fprintf(&b, "- Portion %d: %s, %s\n", k.ID, k.Name, k.Nutrition.Describe())
			}
			b.WriteString("\nStock Items:\n")
			if len(items) == 0 {
				b.WriteString("(none)\n")
			}
			for _, it := range items {
				fmt.Fprintf(&b, "- %g %s %s in the %s", it.Quantity, it.Ingredient.Unit, it.Ingredient.Name, it.Location)
				switch {
				case it.Expired(s.today()):
					fmt.Fprintf(&b, ", expired on %s: never use it", it.ExpiresOn.Format(longDate))
				case it.ExpiresOn != nil:
					fmt.Fprintf(&b, ", expires on %s", it.ExpiresOn.Format(longDate))
				}
				b.WriteString("\n")
			}
			return strings.TrimSpace(b.String()), nil
		},
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
	if err := s.Arrive(ctx); err != nil {
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
		// The user may have planned it another way meanwhile, or it closed.
		if err := stillPlannable(ctx, tx, *c, from, run); err != nil {
			return agent.Stop(err)
		}
		ch, err := s.checkProposal(ctx, tx, storedProposal{CycleID: id, RunID: run, Plan: raw})
		if unreadable := (*UnreadableError)(nil); errors.As(err, &unreadable) {
			problems = append(problems, "the Plan can't be read: "+unreadable.Reason)
			return nil
		}
		if err != nil {
			return err
		}
		for _, v := range domain.ValidatePlan(ch.check) {
			problems = append(problems, v.String())
		}
		if len(problems) > 0 {
			return nil
		}
		return insertProposal(ctx, tx, id, run, raw)
	})
	return problems, err
}

// stillPlannable checks the Cycle c can still get an initial Plan from
// the planning Run with run, going for it.
func stillPlannable(ctx context.Context, q db.Querier, c Cycle, from domain.Slot, run int64) error {
	err := plannable(ctx, q, c, from)
	if going := (*CannotPlanError)(nil); errors.As(err, &going) && going.RunID == run {
		return nil
	}
	return err
}

// Planning is how planning a Cycle without a Plan stands.
type Planning struct {
	// CanPlan is whether a planning Run can start for it now.
	CanPlan bool
	// Run is its last planning Run, with its Trace; nil when it had none.
	Run *runs.Run
}

// planning loads how planning c stands.
func (s *Service) planning(ctx context.Context, q db.Querier, c Cycle) (Planning, error) {
	var p Planning
	var err error
	if p.CanPlan, err = s.canPlan(ctx, q, c); err != nil {
		return p, err
	}
	id, status, err := latestPlanningRun(ctx, q, c.ID)
	if err != nil || status == "" {
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
