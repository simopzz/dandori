package planning

import (
	"cmp"
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
	"github.com/simopzz/dandori/internal/kitchen"
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/profile"
	"github.com/simopzz/dandori/internal/runs"
)

// Kind is the kind of Run that plans a Cycle into a Plan Proposal.
const Kind = "planning"

// planEffort is how hard the model thinks planning: the Plan is long, and thinking shares
// its token budget.
const planEffort = agent.EffortMedium

// DefaultChecks is how many times a planning Run may call check_plan unless the Service
// is told otherwise.
const DefaultChecks = 6

// attempts is how many submitted Plans breaking a hard rule fail a planning Run.
const attempts = 3

// BrokeHardRules is a planning Run failing after attempts Plans that break a hard rule.
const BrokeHardRules agent.Reason = "broke_hard_rules"

// ErrBusy is planning while another planning Run is going, or a Plan Proposal pending,
// for any of the User's Cycles.
var ErrBusy = errors.New("another planning Run is going or a Plan Proposal is pending")

// lowCareBrief is the Planning Brief the planner follows when the User's is empty.
const lowCareBrief = "Low-care Recipes, about 15 active minutes."

// languageNames name each Language for the model.
var languageNames = map[domain.Language]string{domain.English: "English", domain.Italian: "Italian"}

// StartPlanning starts a planning Run of the signed-in User's Cycle id, upcoming at now,
// from its first Slot, told what the kitchen holds now, and is the Run's id. It is
// ErrNoCycle for a Cycle not upcoming, and ErrBusy while another planning Run is going or
// a Plan Proposal is pending.
func (s *Service) StartPlanning(ctx context.Context, id int64, now time.Time) (int64, error) {
	c, err := s.Upcoming(ctx, id, now)
	if err != nil {
		return 0, err
	}
	p, err := s.profile.Planner(ctx)
	if err != nil {
		return 0, err
	}
	library, err := s.recipes.Search(ctx, "")
	if err != nil {
		return 0, err
	}
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return 0, err
	}
	held, err := s.kitchen.Contents(ctx, now)
	if err != nil {
		return 0, err
	}
	slots := c.Slots(p.Times)
	first := slots[0]
	b := planBrief{
		slots: slots, library: library, known: known, checks: s.checks,
		unslotted: unslotted(held.Portions), zone: now.Location(), save: s.saveProposal,
	}
	prompt := planPrompt(c, p, now, library, known, held)
	asked, _, _ := strings.Cut(prompt, "\n")
	return s.runs.Start(ctx, Kind, asked, b.brief(p.Language, prompt), func(tx pgx.Tx, runID int64) error {
		if err := lockOpening(ctx, tx); err != nil {
			return err
		}
		if busy, err := planningBusy(ctx, tx, runID); err != nil || busy {
			return cmp.Or(err, ErrBusy)
		}
		return savePlanningRun(ctx, tx, runID, c.ID, first)
	})
}

// unslotted are the Portions lots hold, each on its own: none has a Slot until a Plan
// is approved.
func unslotted(lots []domain.PortionLot) []domain.Portion {
	var portions []domain.Portion
	for _, lot := range lots {
		for _, id := range lot.IDs {
			portions = append(portions, domain.Portion{ID: id, Name: lot.Name, Nutrition: lot.Nutrition})
		}
	}
	return portions
}

// plannerSystem is the planning Run's system prompt, the same for every User and Cycle,
// so it is cached; %[1]d is how many Plans breaking a hard rule fail the Run.
var plannerSystem = fmt.Sprintf(`You plan a Cycle of the user's meals: the stretch one supermarket Delivery has to last. You check drafts of its Plan, then submit one, which the user approves or rejects.

A Plan is Prep Sessions, each preparing food at one Slot (an eating occasion: breakfast, lunch, snack or dinner on a day) and yielding Portions, each eaten at a Slot or kept for later.
- Fill every Slot from the first one you plan to the Cycle's last with exactly one Portion.
- A Prep Session prepares a Recipe (recipe_id) or one Ingredient set out as it is (ingredient_id, with its quantity for each Portion in the Ingredient's unit), never both.
- A Prep Session of a Recipe chooses its Portion size, between %[2]g and %[3]g of the Recipe's Portion as written, the same for all its Portions; its Ingredients are scaled to the Portions it yields at that size. A "serves 4" Recipe at 1.25 makes bigger dinners.
- A dinner Prep Session yields three Portions by default: one eaten then, one at lunch two days later, and a third two to three days after that, typically frozen. Other Prep Sessions yield what their planned eating needs, often one Portion eaten straight away.
- A Portion is eaten at or after the Slot it is prepared at. One given no Slot is kept, and carries over past the Cycle.
- Give each Portion a Storage Location (pantry, fridge or freezer) that fits what it is and how long it must keep.
- Put the Portions without a Slot in Slots before scheduling new Prep Sessions, and use first the Stock Items that would spoil first, judged from the Ingredient and its Storage Location.
- Prefer Recipes with few active minutes: the user spends little time cooking.
- Follow the user's Planning Brief as far as these rules allow.

Write every food term in the user's Language, named in the prompt.

Call check_plan with a whole draft Plan: it stores nothing, gives the draft an id and lists every hard rule it breaks. You have a few calls; use them well. Then call submit_plan with the id of a draft that keeps every hard rule. A draft breaking one comes back with the rules it breaks; after %[1]d such submits the Run fails. Once a Plan Proposal is stored, end your turn with a short summary of the Plan for the user, in their Language.`, attempts, domain.MinSize, domain.MaxSize)

// planPrompt is what a planning Run of c is told: the User's Language, the Cycle and its
// Slots, today, the Profile p as the planner reads it, the Recipe Library, the known
// Ingredients, the Planning Brief or its low-care default, and what the kitchen holds,
// k, at now.
func planPrompt(c Cycle, p profile.Planner, now time.Time, library []domain.Recipe, known []domain.Ingredient, k kitchen.Contents) string {
	slots := c.Slots(p.Times)
	var b strings.Builder
	fmt.Fprintf(&b, "Plan the Cycle whose Delivery comes on %s, %s to %s, ending on %s.\n",
		c.Delivery.On.Format(longDate), c.Delivery.From, c.Delivery.To, c.Ends.Format(longDate))
	fmt.Fprintf(&b, "Its Slots run from %s to %s. Plan every Slot from %s on.\n", slots[0], slots[len(slots)-1], slots[0])
	fmt.Fprintf(&b, "Today is %s. The user's Language is %s.\n", now.Format(longDate), languageNames[p.Language])

	t := p.Targets
	b.WriteString("\nProfile:\n")
	fmt.Fprintf(&b, "- Targets a day: %d kcal (within %d%%), at least %d g protein, at least %d g fibre, at most %d g sugar.\n",
		t.Kcal, t.Tolerance, t.Protein, t.Fibre, t.Sugar)
	fmt.Fprintf(&b, "- A Meal Out is estimated at %s.\n", describeNutrition(p.MealOut, false, false))
	fmt.Fprintf(&b, "- Excludes: %s.\n", listOrNone(p.Exclusions))
	fmt.Fprintf(&b, "- Allergic to: %s.\n", listOrNone(p.Allergies))
	var owned []string
	for _, k := range domain.ApplianceKinds {
		if note, ok := p.Appliances[k]; ok {
			owned = append(owned, strings.TrimSpace(string(k)+" "+parenthesised(note)))
		}
	}
	fmt.Fprintf(&b, "- Owns: %s.\n", listOrNone(owned))

	b.WriteString("\nRecipe Library:\n")
	if len(library) == 0 {
		b.WriteString("(none yet)\n")
	}
	for _, r := range library {
		n := r.NutritionPerPortion()
		var ings []string
		for _, ri := range r.Ingredients {
			ings = append(ings, fmt.Sprintf("%g %s %s (Ingredient %d)", ri.Quantity.Number(), ri.Ingredient.Unit, ri.Ingredient.Name, ri.Ingredient.ID))
		}
		fmt.Fprintf(&b, "- Recipe %d: %s, %d Portions as written, each %s; %d active of %d total minutes; for %s; needs %s. Ingredients: %s.\n",
			r.ID, r.Name, r.Portions, describeNutrition(n.Nutrition, n.FibreFloor, n.SugarFloor), r.ActiveMinutes, r.TotalMinutes,
			strings.Join(codes(r.SlotKinds), ", "), listOrNone(codes(r.Appliances)), strings.Join(ings, ", "))
	}

	b.WriteString("\nIngredients, which a Prep Session may set out as they are:\n")
	if len(known) == 0 {
		b.WriteString("(none yet)\n")
	}
	for _, ing := range known {
		fmt.Fprintf(&b, "- Ingredient %d: %s, in %s, usually in the %s, per %s %s\n", ing.ID, ing.Name, ing.Unit, ing.Usual, per(ing.Unit), describeNutrition(ing.Nutrition, false, false))
	}

	brief := cmp.Or(p.Brief, lowCareBrief)
	fmt.Fprintf(&b, "\nThe user's Planning Brief, their lasting wishes for how their meals are planned:\n<brief>\n%s\n</brief>\n", brief)

	b.WriteString("\nWhat the kitchen holds now.\nPortions without a Slot:\n")
	if len(k.Portions) == 0 {
		b.WriteString("(none)\n")
	}
	for _, lot := range k.Portions {
		for _, id := range lot.IDs {
			fmt.Fprintf(&b, "- Portion %d: %s, %s, each %s\n", id, lot.Name, lot.Location, describeNutrition(lot.Nutrition, false, false))
		}
	}
	b.WriteString("Stock Items:\n")
	if len(k.Held) == 0 {
		b.WriteString("(none)\n")
	}
	for _, h := range k.Held {
		fmt.Fprintf(&b, "- %g %s %s (Ingredient %d), %s\n", h.Item.Held.Number(), h.Ingredient.Unit, h.Ingredient.Name, h.Ingredient.ID, h.Location)
	}
	return strings.TrimSpace(b.String())
}

// longDate is how the planner is told a day.
const longDate = "Monday 2 January 2006"

// describeNutrition is n as the planner reads it, a floor said "at least".
func describeNutrition(n domain.Nutrition, fibreFloor, sugarFloor bool) string {
	amount := func(a domain.Amount, unit, what string, floor bool) string {
		switch {
		case !a.Known:
			return what + " unknown"
		case floor:
			return fmt.Sprintf("at least %g%s %s", a.Value, unit, what)
		}
		return fmt.Sprintf("%g%s %s", a.Value, unit, what)
	}
	return strings.Join([]string{
		amount(n.Kcal, "", "kcal", false), amount(n.Protein, " g", "protein", false),
		amount(n.Carbs, " g", "carbs", false), amount(n.Fat, " g", "fat", false),
		amount(n.Fibre, " g", "fibre", fibreFloor), amount(n.Sugar, " g", "sugar", sugarFloor),
	}, ", ")
}

// per is the quantity of unit an Ingredient's values are given for.
func per(unit domain.Unit) string {
	if unit == domain.Pieces {
		return "piece:"
	}
	return "100 " + string(unit) + ":"
}

// listOrNone is items joined, or "none".
func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

// parenthesised is note in brackets, or "" for none.
func parenthesised(note string) string {
	if note == "" {
		return ""
	}
	return "(" + note + ")"
}

// codes are kinds by their codes.
func codes[K ~string](kinds []K) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// planBrief is what one planning Run checks drafts against: the Slots it plans, the
// Recipe Library and Ingredients their ids name, the Portions without a Slot, how many
// check_plan calls it has, the zone its days are in, and how to store its Plan Proposal.
type planBrief struct {
	slots     []domain.Slot
	library   []domain.Recipe
	known     []domain.Ingredient
	unslotted []domain.Portion
	checks    int
	zone      *time.Location
	save      func(ctx context.Context, runID int64, plan domain.Plan, summary string) error

	drafts  []domain.Plan
	checked int
	broken  []string // each submit breaking a hard rule, what it broke
	chosen  *domain.Plan
}

// brief is the planning Run's Brief, for a User speaking l, told prompt.
func (pb *planBrief) brief(l domain.Language, prompt string) agent.Brief {
	return agent.Brief{
		System: plannerSystem,
		Prompt: prompt,
		Effort: planEffort,
		Ended: func(ctx context.Context, said string) error {
			if pb.chosen == nil {
				return &agent.Failure{Reason: agent.NoResult, Detail: said}
			}
			return pb.save(ctx, runs.IDFrom(ctx), *pb.chosen, said)
		},
		Tools: []agent.Tool{
			{
				Name:        "check_plan",
				Description: fmt.Sprintf("Check a whole draft Plan, from the first Slot to plan to the Cycle's last. Stores nothing: gives the draft an id for submit_plan and lists every hard rule it breaks. %d calls per Run.", pb.checks),
				InputSchema: draftSchema,
				Run:         pb.check,
			},
			{
				Name:        "submit_plan",
				Description: fmt.Sprintf("Submit a draft checked with check_plan, by its id. One that keeps every hard rule is stored as the Plan Proposal for the user; one that breaks one comes back with what it breaks. After %d of those the Run fails.", attempts),
				InputSchema: json.RawMessage(`{"type": "object", "properties": {"draft_id": {"type": "integer"}}, "required": ["draft_id"]}`),
				Run:         pb.submit,
			},
		},
	}
}

// check reads a draft Plan, stores it for submit_plan under the next id, and says which
// hard rules it breaks.
func (pb *planBrief) check(_ context.Context, input json.RawMessage) (string, error) {
	if pb.checked >= pb.checks {
		return "", fmt.Errorf("the check_plan budget of %d calls is spent: submit the best draft you have with submit_plan", pb.checks)
	}
	pb.checked++
	plan, problems := pb.read(input)
	if len(problems) > 0 {
		return "", errors.New("the draft can't be read; fix these and check it again:\n- " + strings.Join(problems, "\n- "))
	}
	pb.drafts = append(pb.drafts, plan)
	id := len(pb.drafts)
	vs := domain.HardRules(plan, pb.slots, pb.unslotted)
	left := fmt.Sprintf("%d of %d check_plan calls left.", pb.checks-pb.checked, pb.checks)
	if len(vs) == 0 {
		return fmt.Sprintf("Draft %d keeps every hard rule. %s", id, left), nil
	}
	return fmt.Sprintf("Draft %d breaks these hard rules:\n%s\n%s", id, listViolations(vs), left), nil
}

// submit chooses the draft named as the Plan Proposal if it keeps every hard rule, and
// fails the Run after attempts that break one.
func (pb *planBrief) submit(_ context.Context, input json.RawMessage) (string, error) {
	if pb.chosen != nil {
		return "", errors.New("a Plan Proposal is already stored: end your turn with a short summary of it")
	}
	var in struct {
		DraftID int `json:"draft_id"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.DraftID < 1 || in.DraftID > len(pb.drafts) {
		return "", fmt.Errorf("there is no draft %d: check a draft with check_plan first", in.DraftID)
	}
	plan := pb.drafts[in.DraftID-1]
	vs := domain.HardRules(plan, pb.slots, pb.unslotted)
	if len(vs) == 0 {
		pb.chosen = &plan
		return fmt.Sprintf("Draft %d is stored as the Plan Proposal for the user. End your turn with a short summary of it.", in.DraftID), nil
	}
	pb.broken = append(pb.broken, fmt.Sprintf("Draft %d:\n%s", in.DraftID, listViolations(vs)))
	if len(pb.broken) >= attempts {
		return "", &agent.Failure{Reason: BrokeHardRules, Detail: strings.Join(pb.broken, "\n")}
	}
	return "", fmt.Errorf("draft %d breaks these hard rules (submit %d of %d):\n%s", in.DraftID, len(pb.broken), attempts, listViolations(vs))
}

// listViolations is vs one a line, as the planner reads them.
func listViolations(vs []domain.Violation) string {
	lines := make([]string, len(vs))
	for i, v := range vs {
		lines[i] = "- " + v.String()
	}
	return strings.Join(lines, "\n")
}

// draftSchema is a draft Plan as check_plan takes it.
var draftSchema = json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "prep_sessions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "on": {"type": "string", "description": "The day it is prepared, YYYY-MM-DD."},
          "kind": {"type": "string", "enum": %[1]s, "description": "The Slot it is prepared at."},
          "recipe_id": {"type": "integer", "description": "The Recipe it prepares."},
          "size": {"type": "number", "minimum": %[2]g, "maximum": %[3]g, "description": "Its Portion size against the Recipe's as written; only with recipe_id."},
          "ingredient_id": {"type": "integer", "description": "The Ingredient it sets out as it is, when it prepares no Recipe."},
          "quantity": {"type": "number", "exclusiveMinimum": 0, "description": "How much of the Ingredient each Portion is, in its unit; only with ingredient_id."},
          "portions": {
            "type": "array",
            "minItems": 1,
            "items": {
              "type": "object",
              "properties": {
                "location": {"type": "string", "enum": ["pantry", "fridge", "freezer"]},
                "eat_on": {"type": "string", "description": "The day it is eaten, YYYY-MM-DD; leave out with eat_kind to keep it without a Slot."},
                "eat_kind": {"type": "string", "enum": %[1]s}
              },
              "required": ["location"]
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
          "kind": {"type": "string", "enum": %[1]s}
        },
        "required": ["portion_id", "on", "kind"]
      }
    }
  },
  "required": ["prep_sessions"]
}`, enumOf(domain.SlotKinds), domain.MinSize, domain.MaxSize))

// enumOf is kinds as a JSON array of their codes, for a schema's enum.
func enumOf[K ~string](kinds []K) []byte {
	b, err := json.Marshal(kinds)
	if err != nil {
		panic(err) // a slice of strings always marshals
	}
	return b
}

// draft is a draft Plan as the model gives it.
type draft struct {
	PrepSessions []struct {
		On           string          `json:"on"`
		Kind         domain.SlotKind `json:"kind"`
		RecipeID     int64           `json:"recipe_id"`
		Size         float64         `json:"size"`
		IngredientID int64           `json:"ingredient_id"`
		Quantity     float64         `json:"quantity"`
		Portions     []struct {
			Location domain.StorageLocation `json:"location"`
			EatOn    string                 `json:"eat_on"`
			EatKind  domain.SlotKind        `json:"eat_kind"`
		} `json:"portions"`
	} `json:"prep_sessions"`
	Kept []struct {
		PortionID int64           `json:"portion_id"`
		On        string          `json:"on"`
		Kind      domain.SlotKind `json:"kind"`
	} `json:"kept"`
}

// read is the draft Plan input, its ids resolved, or why it can't be read: an id that
// names nothing, a Slot that isn't one, a Prep Session of both or neither a Recipe and an
// Ingredient, and what domain.Plan.Unreadable finds.
func (pb *planBrief) read(input json.RawMessage) (domain.Plan, []string) {
	var d draft
	if err := json.Unmarshal(input, &d); err != nil {
		return domain.Plan{}, []string{"the draft does not match the schema: " + err.Error()}
	}
	var problems []string
	slot := func(on string, kind domain.SlotKind, where string) domain.Slot {
		day, err := time.ParseInLocation(time.DateOnly, on, pb.zone)
		if err != nil || !slices.Contains(domain.SlotKinds, kind) {
			problems = append(problems, fmt.Sprintf("%s: %q %q is not a day and a kind of Slot", where, on, kind))
		}
		return domain.Slot{On: day, Kind: kind}
	}
	recipes := map[int64]*domain.Recipe{}
	for i := range pb.library {
		recipes[pb.library[i].ID] = &pb.library[i]
	}
	ingredients := map[int64]*domain.Ingredient{}
	for i := range pb.known {
		ingredients[pb.known[i].ID] = &pb.known[i]
	}
	var plan domain.Plan
	for i, in := range d.PrepSessions {
		where := fmt.Sprintf("prep_sessions.%d", i)
		ps := domain.PrepSession{At: slot(in.On, in.Kind, where), Size: in.Size, Quantity: domain.QuantityOf(in.Quantity)}
		switch {
		case in.RecipeID != 0 && in.IngredientID != 0, in.RecipeID == 0 && in.IngredientID == 0:
			problems = append(problems, where+": give recipe_id or ingredient_id, not both")
			continue
		case in.RecipeID != 0:
			if ps.Recipe = recipes[in.RecipeID]; ps.Recipe == nil {
				problems = append(problems, fmt.Sprintf("%s: there is no Recipe %d", where, in.RecipeID))
				continue
			}
		default:
			if ps.Ingredient = ingredients[in.IngredientID]; ps.Ingredient == nil {
				problems = append(problems, fmt.Sprintf("%s: there is no Ingredient %d", where, in.IngredientID))
				continue
			}
			if ps.Quantity <= 0 {
				problems = append(problems, where+": give the quantity of each Portion, above 0")
			}
		}
		for j, p := range in.Portions {
			pp := domain.PlannedPortion{Location: p.Location}
			if !slices.Contains(domain.StorageLocations, p.Location) {
				problems = append(problems, fmt.Sprintf("%s.portions.%d: %q is not a Storage Location", where, j, p.Location))
			}
			if p.EatOn != "" || p.EatKind != "" {
				s := slot(p.EatOn, p.EatKind, fmt.Sprintf("%s.portions.%d", where, j))
				pp.EatAt = &s
			}
			ps.Portions = append(ps.Portions, pp)
		}
		plan.PrepSessions = append(plan.PrepSessions, ps)
	}
	for i, k := range d.Kept {
		plan.Kept = append(plan.Kept, domain.Kept{Portion: k.PortionID, At: slot(k.On, k.Kind, fmt.Sprintf("kept.%d", i))})
	}
	return plan, append(problems, plan.Unreadable()...)
}

// saveProposal stores plan as the Plan Proposal of the signed-in User's planning Run
// runID, with the planner's summary.
func (s *Service) saveProposal(ctx context.Context, runID int64, plan domain.Plan, summary string) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveProposal(ctx, tx, runID, plan, summary) })
}

// Proposal is a pending Plan Proposal as its Cycle page shows it: its Plan, the
// Portions without a Slot it puts in Slots by id, and the planner's summary.
type Proposal struct {
	Plan    domain.Plan
	Kept    map[int64]domain.Portion
	Summary string
}

// Planning is how planning a Cycle stands: its pending Plan Proposal, nil when it has
// none, and the id of its planning Run going, 0 when none is.
type Planning struct {
	Proposal *Proposal
	Going    int64
}

// PlanningOf is how planning the signed-in User's Cycle id stands, its days in zone.
func (s *Service) PlanningOf(ctx context.Context, id int64, zone *time.Location) (Planning, error) {
	var p Planning
	var stored *storedProposal
	var portions []domain.Portion
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) (err error) {
		if p.Going, err = goingRun(ctx, tx, id); err != nil {
			return err
		}
		if stored, err = loadPending(ctx, tx, id, zone); err != nil || stored == nil {
			return err
		}
		portions, err = ledger.LoadPortions(ctx, tx)
		return err
	})
	if err != nil || stored == nil {
		return p, err
	}
	library, err := s.recipes.Search(ctx, "")
	if err != nil {
		return p, err
	}
	known, err := s.ingredients.List(ctx)
	if err != nil {
		return p, err
	}
	p.Proposal = stored.resolve(library, known, portions)
	return p, nil
}

// resolve is sp with its Recipes, Ingredients and Portions found by id among those given.
func (sp storedProposal) resolve(library []domain.Recipe, known []domain.Ingredient, portions []domain.Portion) *Proposal {
	recipes := map[int64]*domain.Recipe{}
	for i := range library {
		recipes[library[i].ID] = &library[i]
	}
	ingredients := map[int64]*domain.Ingredient{}
	for i := range known {
		ingredients[known[i].ID] = &known[i]
	}
	p := &Proposal{Summary: sp.summary, Plan: domain.Plan{Kept: sp.kept}, Kept: map[int64]domain.Portion{}}
	for _, ps := range sp.sessions {
		ps.Recipe, ps.Ingredient = recipes[ps.recipeID], ingredients[ps.ingredientID]
		p.Plan.PrepSessions = append(p.Plan.PrepSessions, ps.PrepSession)
	}
	for _, portion := range portions {
		if slices.ContainsFunc(sp.kept, func(k domain.Kept) bool { return k.Portion == portion.ID }) {
			p.Kept[portion.ID] = portion
		}
	}
	return p
}
