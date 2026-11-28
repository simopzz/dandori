package planning

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/kitchen"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
)

// planningKind is the kind of Run that plans a Cycle into a Plan Proposal.
const planningKind runs.Kind = "planning"

// planEffort is how hard the model thinks planning: the Plan is long, and thinking shares
// its token budget.
const planEffort = agent.EffortMedium

// DefaultChecks is how many times a planning Run may call check_plan unless the Service
// is told otherwise.
const DefaultChecks = 6

// attempts is how many submitted Plans breaking a hard rule fail a planning Run.
const attempts = 3

// brokeHardRules is a planning Run failing after attempts Plans that break a hard rule.
const brokeHardRules agent.Reason = "broke_hard_rules"

// lowCareBrief is the Planning Brief the planner follows when the User's is empty.
const lowCareBrief = "Low-care Recipes, about 15 active minutes."

// languageNames name each Language for the model.
var languageNames = map[domain.Language]string{domain.English: "English", domain.Italian: "Italian"}

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
- Eat a Recipe's Portions only at the kinds of Slot it is for, and prepare only Recipes whose appliances the user owns.
- Frozen food is thawed the evening before the Slot or Prep Session that needs it, after the previous day's dinner: use a frozen Portion, or freezer stock of an Ingredient without Use from frozen, only where that evening is still to come, and eat a Portion you freeze on a later day than it is prepared. An Ingredient marked Use from frozen (frozen peas) is used straight from the freezer, with no thaw. A Prep Session uses stock outside the freezer first, then freezer stock marked Use from frozen, and thaws what it still needs from the freezer.
- When a fresh Ingredient the Delivery brings is used late in the Cycle, name a freezer split: how much of it goes in the freezer when the Delivery is put away (of 800 g chicken breast, 400 g).
- Until the user marks the Delivery ordered, it brings whatever the Plan needs. Once the prompt says it is ordered, it brings only what the prompt lists: no Prep Session may need more of an Ingredient than the kitchen will hold with it, Staples aside, which it always holds. Its freezer splits may still change until it is put away.
- Put the Portions without a Slot in Slots before scheduling new Prep Sessions, and use first the Stock Items that would spoil first, judged from the Ingredient and its Storage Location.
- Prefer Recipes with few active minutes: the user spends little time cooking.
- Draft new Recipes with draft_recipe as the Planning Brief wishes, about one a Cycle when it doesn't say: dishes the Recipe Library lacks, reusing the known Ingredients where they fit. Each gets a negative id, which a Prep Session prepares as its recipe_id. Approving the Plan adds the drafts it prepares to the user's Recipe Library.
- For each new Ingredient in draft_recipe, choose its usual Storage Location and whether it is used from frozen independently. Set Use from frozen only when it is usually kept in the freezer and used straight from frozen; a whole chicken kept in the freezer needs thawing first.
- Follow the user's Planning Brief as far as these rules allow.

Write every food term in the user's Language, named in the prompt.

Call check_plan with a whole draft Plan: it stores nothing, gives the draft an id and lists every hard rule it breaks. You have a few calls; use them well. Then call submit_plan with the id of a draft that keeps every hard rule. A draft breaking one comes back with the rules it breaks; after %[1]d such submits the Run fails. Once a Plan Proposal is stored, end your turn with a summary of the Plan for the user, in their Language: two or three sentences on what it cooks and why. Give no nutrition figures and no day-by-day list: Dandori shows both.`, attempts, domain.MinSize, domain.MaxSize)

// planStable is the planning Run's stable block, what varies per User but rarely per Run,
// so it is cached: the Profile `p`, the Recipe Library and the `known` Ingredients.
func planStable(p domain.Profile, library []domain.Recipe, known []domain.Ingredient) string {
	var b strings.Builder
	t := p.Targets
	b.WriteString("Profile:\n")
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
			strings.Join(domain.Codes(r.SlotKinds), ", "), listOrNone(domain.Codes(r.Appliances)), strings.Join(ings, ", "))
	}

	b.WriteString("\nIngredients, which a Prep Session may set out as they are:\n")
	if len(known) == 0 {
		b.WriteString("(none yet)\n")
	}
	for _, ing := range known {
		staple := ""
		if ing.Staple {
			staple = ", a Staple, which the kitchen always holds"
		}
		useFromFrozen := ""
		if ing.UseFromFrozen {
			useFromFrozen = ", use directly from frozen"
		}
		fmt.Fprintf(&b, "- Ingredient %d: %s, in %s, usually in the %s%s%s, per %s %s\n", ing.ID, ing.Name, ing.Unit, ing.Usual, useFromFrozen, staple, per(ing.Unit), describeNutrition(ing.Nutrition, false, false))
	}
	return strings.TrimSpace(b.String())
}

// planPrompt is what a planning Run of `c`'s Slots `planned`, from the first on, is told
// after its stable block: the Cycle and its Slots, today, the User's Language, the
// Planning Brief or its low-care default, the Planning Notes newest first, the kitchen
// forecast `f` at the first of the Stock Items `held` at `now`, the Portions without a
// Slot being those it may put in Slots, what an ordered Delivery brings, that a current
// Cycle gets no more, what `current`, the Cycle's Plan, eats from then on, which it
// replaces, `portions` naming the Portions without a Slot it puts in Slots, and what the
// `trailing` seven days before from eat as approved Plans fill them.
func planPrompt(c Cycle, p domain.Profile, planned []domain.Slot, brief string, notes []string, now time.Time, held []kitchen.Held, f domain.Forecast, current domain.Plan, portions []domain.Portion, trailing []domain.Day) string {
	slots, from := c.Slots(p.SlotTimes), planned[0]
	var b strings.Builder
	fmt.Fprintf(&b, "Plan the Cycle whose Delivery comes on %s, %s to %s, ending on %s.\n",
		c.Delivery.On.Format(longDate), c.Delivery.From, c.Delivery.To, c.Ends.Format(longDate))
	fmt.Fprintf(&b, "Its Slots run from %s to %s. Plan every Slot from %s on.\n", slots[0], slots[len(slots)-1], from)
	fmt.Fprintf(&b, "Today is %s. The user's Language is %s.\n", now.Format(longDate), languageNames[p.Language])

	brief = cmp.Or(brief, lowCareBrief)
	fmt.Fprintf(&b, "\nThe user's Planning Brief, their lasting wishes for how their meals are planned:\n<brief>\n%s\n</brief>\n", brief)
	if len(notes) > 0 {
		b.WriteString("\nThe user's Planning Notes on this Cycle, newest first. Keep to every one: a newer note overrides an older one only where the two contradict each other.\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "<note>\n%s\n</note>\n", n)
		}
	}

	fmt.Fprintf(&b, "\nWhat the kitchen will hold at %s, the first Slot to plan: what it holds now, as approved Plans leave it by then.\nPortions without a Slot:\n", from)
	if len(f.Lots) == 0 {
		b.WriteString("(none)\n")
	}
	for _, lot := range f.Lots {
		for _, id := range lot.IDs {
			fmt.Fprintf(&b, "- Portion %d: %s, %s, each %s\n", id, lot.Name, lot.Location, describeNutrition(lot.Nutrition, false, false))
		}
	}
	b.WriteString("Stock Items:\n")
	arriving := map[domain.StockKey]domain.Quantity{}
	for _, l := range f.Arriving {
		arriving[l.Key()] += l.Quantity
	}
	told := 0
	for _, h := range held {
		key := domain.StockKey{Ingredient: h.Ingredient.ID, Location: h.Location}
		left := f.Stock[key] - arriving[key]
		if left <= 0 {
			continue
		}
		told++
		fmt.Fprintf(&b, "- %g %s %s (Ingredient %d), %s\n", left.Number(), h.Ingredient.Unit, h.Ingredient.Name, h.Ingredient.ID, h.Location)
	}
	if told == 0 {
		b.WriteString("(none)\n")
	}
	if c.Delivery.State == domain.Ordered {
		b.WriteString("Its Delivery is ordered, so it brings only these, put away where they go, besides the Stock Items above:\n")
		if len(f.Arriving) == 0 {
			b.WriteString("(nothing)\n")
		}
		for _, l := range f.Arriving {
			fmt.Fprintf(&b, "- %g %s %s (Ingredient %d), %s\n", l.Quantity.Number(), l.Ingredient.Unit, l.Ingredient.Name, l.Ingredient.ID, l.Location)
		}
	}
	if !c.Delivery.Arrives().After(now) {
		b.WriteString("This Cycle is current: its Delivery has arrived and none comes before it ends, so no Prep Session may need more of an Ingredient than the kitchen will hold then, Staples aside, which it always holds. Its Delivery is put away too, so name no freezer split.\n")
	}
	if eats := eatenFrom(current, planned, portions); len(eats) > 0 {
		fmt.Fprintf(&b, "\nThe Cycle has a Plan, which this Plan Proposal replaces from %s on, to change what the user's Planning Notes ask. Before then the Plan stays, and so do its Prep Sessions: the Portions they prepared and don't eat by then are among the Portions without a Slot above, with negative ids, to put in Slots again. What the Plan eats from %s on:\n", from, from)
		for _, e := range eats {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}

	b.WriteString("\nWhat the seven days before the first Slot to plan eat, as approved Plans fill them. They are past: shown, never made up for.\n")
	for _, d := range trailing {
		eats := "no Plan"
		if d.Filled > 0 {
			eats = describeDay(d.Nutrition, nil)
		}
		fmt.Fprintf(&b, "- %s: %s\n", d.On.Format(domain.PlannerDay), eats)
	}
	return strings.TrimSpace(b.String())
}

// eatenFrom is what `plan` puts in each of `slots`, in order, as the planner reads it,
// `portions` naming the Portions without a Slot it puts in them: none for a Slot it
// leaves empty.
func eatenFrom(plan domain.Plan, slots []domain.Slot, portions []domain.Portion) []string {
	var eats []string
	planned := plan.PlannedSlots()
	for _, sl := range slots {
		for _, slot := range planned {
			if !slot.At.Equal(sl) {
				continue
			}
			from := slot.From
			switch {
			case from != nil && from.Recipe != nil:
				eats = append(eats, fmt.Sprintf("%s: %s at %g×, prepared at %s", sl, from.Name(), from.Size, from.At))
			case from != nil:
				eats = append(eats, fmt.Sprintf("%s: %g %s %s, set out at %s", sl, from.Quantity.Number(), from.Ingredient.Unit, from.Name(), from.At))
			default:
				id := slot.Portion
				if slot.Planned != nil {
					id = domain.PlannedID(slot.Planned.ID) // the planner names it so, among the Portions without a Slot
				}
				if i := slices.IndexFunc(portions, func(p domain.Portion) bool { return p.ID == id }); i >= 0 {
					eats = append(eats, fmt.Sprintf("%s: Portion %d, %s", sl, id, portions[i].Name))
				}
			}
		}
	}
	return eats
}

// longDate is how the planner is told a day in full.
const longDate = "Monday 2 January 2006"

// describeDay is `n`'s values of what the Targets aim at, as the planner reads them,
// those missed marked, if `missed` says.
func describeDay(n domain.PortionNutrition, missed func(domain.Target) (missed, under bool)) string {
	parts := make([]string, len(domain.Aimed))
	for i, target := range domain.Aimed {
		a, floor := n.Of(target)
		name := string(target) // a Target's code is a plain English word
		switch {
		case !a.Known:
			parts[i] = name + " unknown"
			continue
		case target == domain.TargetKcal:
			parts[i] = fmt.Sprintf("%.0f kcal", a.Value)
		default:
			parts[i] = fmt.Sprintf("%.0f g %s", a.Value, name)
		}
		if floor {
			parts[i] = "at least " + parts[i]
		}
		if off, _ := missOf(missed, target); off {
			parts[i] += " (missed)"
		}
	}
	return strings.Join(parts, ", ")
}

// dayMissed reports which Targets the day `d` misses as a soft rule does: by more
// than 25%.
func dayMissed(t domain.Targets, d domain.Day) func(domain.Target) (missed, under bool) {
	return func(target domain.Target) (bool, bool) {
		m, off := t.DayOff(target, d)
		return off, m.Under
	}
}

// averageMissed reports which Targets the average `avg` misses, and whether under them.
func averageMissed(t domain.Targets, avg domain.PortionNutrition) func(domain.Target) (missed, under bool) {
	return func(target domain.Target) (bool, bool) {
		a, _ := avg.Of(target)
		lo, _ := t.Bounds(target)
		return t.Missed(target, avg), math.Round(a.Value) < lo
	}
}

// missOf is whether `missed` says `target` is missed, and if so whether `under` it;
// nothing is missed when `missed` is nil.
func missOf(missed func(domain.Target) (bool, bool), target domain.Target) (off, under bool) {
	if missed == nil {
		return false, false
	}
	return missed(target)
}

// describeNutrition is `n` as the planner reads it, a floor said "at least".
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

// per is the quantity of `unit` an Ingredient's values are given for.
func per(unit domain.Unit) string {
	if unit == domain.Pieces {
		return "piece:"
	}
	return "100 " + string(unit) + ":"
}

// listOrNone is `items` joined, or "none".
func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

// parenthesised is `note` in brackets, or "" for none.
func parenthesised(note string) string {
	if note == "" {
		return ""
	}
	return "(" + note + ")"
}

// planBrief is what one planning Run judges drafts by, domain.Judge's inputs: the Cycle,
// the first Slot it plans, the Profile, the Cycle's Plan it replaces, if any, the Portion
// Lots without a Slot, what the kitchen holds of each Stock Item, what approved Plans put
// in Slots before, and the clock. It has the Recipe Library and Ingredients the drafts'
// ids name, the zone their days are in, how many check_plan calls it has, and how it
// leaves its Plan Proposal, stored only as the Run is stored succeeded; then the
// Run's state.
type planBrief struct {
	cycle   domain.Cycle
	from    domain.Slot
	profile domain.Profile
	current domain.Plan
	lots    []domain.PortionLot
	stock   map[domain.StockKey]domain.Quantity
	earlier []domain.Eaten
	now     func() time.Time
	library []domain.Recipe
	known   []domain.Ingredient
	zone    *time.Location
	checks  int
	leave   func(ctx context.Context, plan domain.Plan, v domain.Verdict, summary string, drafted []*recipes.Drafted)

	drafted []*recipes.Drafted // the Recipe Drafts, the first id -1, the next -2
	drafts  []domain.Plan
	checked int
	broken  []string // each submit breaking a hard rule, what it broke
	chosen  *domain.Plan
	verdict domain.Verdict // the Verdict on chosen, stored with it
}

// brief is the planning Run's Brief, for a User speaking `l`, told `stable`
// then `prompt`.
func (pb *planBrief) brief(l domain.Language, stable, prompt string) agent.Brief {
	return agent.Brief{
		System: plannerSystem,
		Stable: stable,
		Prompt: prompt,
		Effort: planEffort,
		Ended: func(ctx context.Context, said string) error {
			if pb.chosen == nil {
				return &agent.Failure{Reason: agent.NoResult, Detail: said}
			}
			pb.leave(ctx, *pb.chosen, pb.verdict, said, pb.drafted)
			return nil
		},
		Tools: []agent.Tool{
			{
				Name:        "check_plan",
				Description: fmt.Sprintf("Check a whole draft Plan, from the first Slot to plan to the Cycle's last. Stores nothing: gives the draft an id for submit_plan, lists every hard rule it breaks, and gives each day's nutrition, the Targets' results and the soft rules it breaks. %d calls per Run.", pb.checks),
				InputSchema: draftSchema,
				Run:         pb.check,
			},
			{
				Name:        "submit_plan",
				Description: fmt.Sprintf("Submit a draft checked with check_plan, by its id. One that keeps every hard rule is stored as the Plan Proposal for the user; one that breaks one comes back with what it breaks. After %d of those the Run fails.", attempts),
				InputSchema: json.RawMessage(`{"type": "object", "properties": {"draft_id": {"type": "integer"}}, "required": ["draft_id"]}`),
				Run:         pb.submit,
			},
			{
				Name:        "draft_recipe",
				Description: "Draft a new Recipe, written as the user's Recipe Library holds them, and get its id, a negative number, to prepare it by as a recipe_id in check_plan. Drafting a name drafted before replaces that draft, keeping its id.",
				InputSchema: recipes.DraftSchema,
				Run:         func(_ context.Context, input json.RawMessage) (string, error) { return pb.draft(l, input) },
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
	v := pb.judge(plan)
	left := fmt.Sprintf("%d of %d check_plan calls left.", pb.checks-pb.checked, pb.checks)
	if len(v.Broken) == 0 {
		return fmt.Sprintf("Draft %d keeps every hard rule. %s\n%s", id, left, pb.results(v)), nil
	}
	return fmt.Sprintf("Draft %d breaks these hard rules:\n%s\n%s\n%s", id, listViolations(v.Broken), left, pb.results(v)), nil
}

// judge is the Verdict on `plan` now.
func (pb *planBrief) judge(plan domain.Plan) domain.Verdict {
	return domain.Judge(plan, pb.cycle, pb.from, pb.profile, pb.current, pb.lots, pb.stock, pb.earlier, pb.now())
}

// results are the Verdict `v` as the planner reads it: each day it plans, the Targets'
// results over the days it fills whole, the soft rules it breaks and the Portions it
// makes for after the Cycle.
func (pb *planBrief) results(v domain.Verdict) string {
	days, misses := v.Days, v.Misses
	judged := domain.Judged(days)
	t := pb.profile.Targets
	lo, hi := t.Bounds(domain.TargetKcal)
	var b strings.Builder
	fmt.Fprintf(&b, "Each day it plans, against the Targets (%.0f–%.0f kcal, at least %d g protein, at least %d g fibre, at most %d g sugar):\n",
		lo, hi, t.Protein, t.Fibre, t.Sugar)
	for _, d := range days {
		on := d.On.Format(domain.PlannerDay)
		if d.Partly {
			on += ", partly planned, so not judged"
		}
		fmt.Fprintf(&b, "- %s: %s\n", on, describeDay(d.Nutrition, dayMissed(t, d)))
	}
	if n := len(judged); n > 0 {
		days := "days"
		if n == 1 {
			days = "day"
		}
		avg := domain.Average(judged)
		fmt.Fprintf(&b, "Average over the %d %s judged: %s\n", n, days, describeDay(avg, averageMissed(t, avg)))
	}
	if len(misses) == 0 {
		b.WriteString("It breaks no soft rule.")
	} else {
		b.WriteString("Soft rules it breaks, which don't stop it being stored but which the user weighs:\n")
		for _, m := range misses {
			fmt.Fprintf(&b, "- %s\n", m.Describe(t))
		}
	}
	if len(v.MadeForAfter) > 0 {
		n, made := 0, make([]string, len(v.MadeForAfter))
		for i, m := range v.MadeForAfter {
			n += m.Portions
			made[i] = fmt.Sprintf("%s: %d, in the %s", m.Name, m.Portions, m.Location)
		}
		portions := "Portions"
		if n == 1 {
			portions = "Portion"
		}
		fmt.Fprintf(&b, "\nInformation, nothing to fix: %d %s made for after the Cycle: %s", n, portions, strings.Join(made, "; "))
	}
	return strings.TrimSpace(b.String())
}

// submit chooses the draft named as the Plan Proposal if it keeps every hard rule, and
// fails the Run after attempts that break one.
func (pb *planBrief) submit(_ context.Context, input json.RawMessage) (string, error) {
	if pb.chosen != nil {
		return "", fmt.Errorf("a Plan Proposal is already stored: end your turn with a short summary of it, in %s", languageNames[pb.profile.Language])
	}
	var in struct {
		DraftID int `json:"draft_id"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.DraftID < 1 || in.DraftID > len(pb.drafts) {
		return "", fmt.Errorf("there is no draft %d: check a draft with check_plan first", in.DraftID)
	}
	plan := pb.drafts[in.DraftID-1]
	v := pb.judge(plan)
	vs := v.Broken
	if len(vs) == 0 {
		pb.chosen, pb.verdict = &plan, v
		// The Language again where the summary is asked for: told only up front, it can
		// come back in English.
		return fmt.Sprintf("Draft %d is stored as the Plan Proposal for the user. End your turn with a short summary of it, in %s.", in.DraftID, languageNames[pb.profile.Language]), nil
	}
	pb.broken = append(pb.broken, fmt.Sprintf("Draft %d:\n%s", in.DraftID, listViolations(vs)))
	if len(pb.broken) >= attempts {
		return "", &agent.Failure{Reason: brokeHardRules, Detail: strings.Join(pb.broken, "\n")}
	}
	return "", fmt.Errorf("draft %d breaks these hard rules (submit %d of %d):\n%s", in.DraftID, len(pb.broken), attempts, listViolations(vs))
}

// draft reads `input` as a Recipe Draft for a User speaking `l`, replacing the one of the
// same name if any, and gives its id, or says what to fix.
func (pb *planBrief) draft(l domain.Language, input json.RawMessage) (string, error) {
	d, err := recipes.ReadDraft(l, pb.known, pb.library, input)
	if err != nil {
		return "", err
	}
	name := domain.NormaliseRecipeName(d.Recipe.Name)
	i := slices.IndexFunc(pb.drafted, func(e *recipes.Drafted) bool { return domain.NormaliseRecipeName(e.Recipe.Name) == name })
	if i < 0 {
		pb.drafted, i = append(pb.drafted, &d), len(pb.drafted)
	} else {
		*pb.drafted[i] = d
	}
	return fmt.Sprintf("Drafted Recipe %d: %s. Prepare it in check_plan with recipe_id %d.", draftID(i), d.Recipe.Name, draftID(i)), nil
}

// draftID is the id the planner prepares the Recipe Draft drafted at `i` by: negative, so
// it is never a Recipe's.
func draftID(i int) int64 { return -int64(i + 1) }

// listViolations is `vs` one a line, as the planner reads them.
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
    },
    "freezer_splits": {
      "type": "array",
      "description": "How much of an Ingredient the Delivery brings goes in the freezer when it is put away.",
      "items": {
        "type": "object",
        "properties": {
          "ingredient_id": {"type": "integer"},
          "quantity": {"type": "number", "exclusiveMinimum": 0, "description": "In the Ingredient's unit."}
        },
        "required": ["ingredient_id", "quantity"]
      }
    }
  },
  "required": ["prep_sessions"]
}`, enumOf(domain.SlotKinds), domain.MinSize, domain.MaxSize))

// enumOf is `kinds` as a JSON array of their codes, for a schema's enum.
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
	FreezerSplits []struct {
		IngredientID int64   `json:"ingredient_id"`
		Quantity     float64 `json:"quantity"`
	} `json:"freezer_splits"`
}

// read is the draft Plan `input`, its ids resolved, or why it can't be read: an id that
// names nothing, a Slot that isn't one, a Prep Session of both or neither a Recipe and an
// Ingredient, a freezer split of nothing, and what domain.Plan.Unreadable finds.
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
	for i, drafted := range pb.drafted {
		recipes[draftID(i)] = &drafted.Recipe
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
	for i, fs := range d.FreezerSplits {
		ing := ingredients[fs.IngredientID]
		switch {
		case ing == nil:
			problems = append(problems, fmt.Sprintf("freezer_splits.%d: there is no Ingredient %d", i, fs.IngredientID))
		case fs.Quantity <= 0:
			problems = append(problems, fmt.Sprintf("freezer_splits.%d: give the quantity, above 0", i))
		default:
			plan.FreezerSplits = append(plan.FreezerSplits, domain.RecipeIngredient{Ingredient: *ing, Quantity: domain.QuantityOf(fs.Quantity)})
		}
	}
	return plan, append(problems, plan.Unreadable()...)
}
