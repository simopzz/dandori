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
	"github.com/simopzz/dandori/internal/esselunga"
	"github.com/simopzz/dandori/internal/runs"
)

// cartFillKind is the kind of Run that adds a Cycle's Grocery List to the
// Esselunga cart.
const cartFillKind = "cart fill"

// ErrCannotFill means Cart Fill can't start for the Cycle now; a
// CannotFillError says why.
var ErrCannotFill = errors.New("cart can't be filled")

// CannotFillError means Cart Fill can't start for the Cycle now. Reason
// says why, for the user; RunID is the Cart Fill Run already going, if
// that is why.
type CannotFillError struct {
	Reason string
	RunID  int64
}

func (e *CannotFillError) Error() string        { return ErrCannotFill.Error() + ": " + e.Reason }
func (e *CannotFillError) Is(target error) bool { return target == ErrCannotFill }

// canFill checks Cart Fill can start for c now, whose Grocery List is
// list: c is upcoming, its Delivery tentative, it has a Plan whose list
// buys a Catalog Product, and no Cart Fill is going, for any Cycle, since
// they share one cart.
func (s *Service) canFill(ctx context.Context, q db.Querier, c Cycle, list domain.GroceryList) error {
	switch {
	case c.Status != "upcoming":
		return &CannotFillError{Reason: "its Delivery has arrived"}
	case c.Delivery.Ordered:
		return &CannotFillError{Reason: "its Delivery is ordered, so its Grocery List is fixed"}
	}
	if planned, err := hasPlan(ctx, q, c.ID); err != nil {
		return err
	} else if !planned {
		return &CannotFillError{Reason: "it has no Plan yet"}
	}
	if !slices.ContainsFunc(list, func(l domain.GroceryLine) bool { return l.Mapping != nil }) {
		return &CannotFillError{Reason: "its Grocery List has no line with a Product Mapping"}
	}
	going, err := s.runs.Going(ctx, q, cartFillKind)
	if err != nil {
		return err
	}
	if len(going) > 0 {
		return &CannotFillError{Reason: "Cart Fill is already going", RunID: going[0]}
	}
	return nil
}

// StartCartFill starts a Cart Fill Run adding the Grocery List of the
// Cycle with id to the Esselunga cart, and returns the Run's id.
func (s *Service) StartCartFill(ctx context.Context, id int64) (int64, error) {
	var started runs.Recorded
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialises with changes to the Cycle's Delivery and Plan, and with
		// another start.
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		c, err := findCycle(ctx, tx, id)
		if err != nil {
			return err
		}
		if c == nil {
			return ErrNoCycle
		}
		list, _, err := s.groceryListOf(ctx, tx, *c)
		if err != nil {
			return err
		}
		if err := s.canFill(ctx, tx, *c, list); err != nil {
			return err
		}
		if started, err = s.runs.Record(ctx, tx, cartFillKind, s.cartFillJob(*c, list)); err != nil {
			return err
		}
		return insertCartFillRun(ctx, tx, started.ID, id)
	})
	if err != nil {
		return 0, err
	}
	s.runs.Launch(started)
	return started.ID, nil
}

// cartFillSystem is the Cart Fill Run's instructions.
const cartFillSystem = `You fill the user's Esselunga cart with their Grocery List, for a Delivery they check out and pay for themselves on Esselunga. You never check out or pay.

1. Call look_up_product for each product on the Grocery List, to see whether Esselunga has it in stock now. Look up several in one turn.
2. For each one available, call set_cart_quantity with the packs the Grocery List buys. It sets the quantity rather than adding to it, so filling the cart again never doubles a product.
3. Leave a product out of stock, or one Esselunga no longer sells, out of the cart: once you end your turn, the planner replans around it in a substitution Plan Proposal for the user to approve.
4. If the store fails on a product, you may try it again once; otherwise move on.

End your turn once every product is looked up and those available are in the cart. Cart Fill then sums up for the user what it added, substituted, skipped and failed, so end with a single short line.`

// cartLine is one product Cart Fill puts in the cart: the packs the
// Grocery List buys of it, for its Ingredients, usually one.
type cartLine struct {
	code, name  string
	packs       int
	ingredients []string
	proposed    bool
}

// cartLines are the products list buys, by code, in its order.
func cartLines(list domain.GroceryList) []*cartLine {
	var lines []*cartLine
	for _, l := range list {
		if l.Mapping == nil {
			continue
		}
		p := l.Mapping.Product
		i := slices.IndexFunc(lines, func(c *cartLine) bool { return c.code == p.Code })
		if i < 0 {
			lines = append(lines, &cartLine{code: p.Code, name: p.Name})
			i = len(lines) - 1
		}
		lines[i].packs += l.Packs
		lines[i].ingredients = append(lines[i].ingredients, l.Ingredient.Name)
		lines[i].proposed = lines[i].proposed || l.Mapping.Proposed
	}
	return lines
}

// fillState is how a Cart Fill Run stands: what Esselunga showed of each
// product, and what is in the cart.
type fillState struct {
	lines []*cartLine
	// skipped are the Ingredients without a Product Mapping.
	skipped []string
	// seen is whether each product looked up, by code, is available;
	// unlisted whether Esselunga no longer sells it; inCart whether it was
	// set in the cart.
	seen     map[string]bool
	unlisted map[string]bool
	inCart   map[string]bool
}

func newFillState(list domain.GroceryList) *fillState {
	f := &fillState{lines: cartLines(list), seen: map[string]bool{}, unlisted: map[string]bool{}, inCart: map[string]bool{}}
	for _, l := range list {
		if l.Mapping == nil {
			f.skipped = append(f.skipped, l.Ingredient.Name)
		}
	}
	return f
}

func (f *fillState) line(code string) (*cartLine, error) {
	i := slices.IndexFunc(f.lines, func(l *cartLine) bool { return l.code == code })
	if i < 0 {
		return nil, fmt.Errorf("product %s is not on the Grocery List", code)
	}
	return f.lines[i], nil
}

// missing is whether Esselunga showed l's product unavailable.
func (f *fillState) missing(l *cartLine) bool {
	available, seen := f.seen[l.code]
	return seen && !available
}

// summary sums up for the user what the Run added, substituted, skipped
// and failed, in Markdown; substituted says what became of the
// substitution.
func (f *fillState) summary(substituted string) string {
	var added, out, failed []string
	for _, l := range f.lines {
		uses := strings.Join(l.ingredients, ", ")
		switch {
		case f.inCart[l.code]:
			item := fmt.Sprintf("%d × %s, for %s", l.packs, l.name, uses)
			if l.proposed {
				item += " (from a proposed Product Mapping: check it in the cart)"
			}
			added = append(added, item)
		case f.unlisted[l.code]:
			out = append(out, fmt.Sprintf("%s, for %s: Esselunga no longer sells it", l.name, uses))
		case f.missing(l):
			out = append(out, fmt.Sprintf("%s, for %s: out of stock", l.name, uses))
		default:
			failed = append(failed, fmt.Sprintf("%s, for %s", l.name, uses))
		}
	}
	var b strings.Builder
	section := func(heading string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&b, "**%s**\n\n", heading)
		for _, item := range items {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}
	section("Added", added)
	section("Substituted: "+substituted, out)
	section("Skipped, without a Product Mapping", f.skipped)
	section("Failed: filling the cart again retries them", failed)
	return strings.TrimSpace(b.String())
}

// cartFillJob is the Cart Fill Run of c, whose Grocery List is list.
func (s *Service) cartFillJob(c Cycle, list domain.GroceryList) agent.Job {
	fill := newFillState(list)
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "The Grocery List for the Delivery on %s:\n", describeDelivery(c.Delivery, longDate))
	for _, l := range fill.lines {
		fmt.Fprintf(&prompt, "- %d × %s (product %s), for %s\n", l.packs, l.name, l.code, strings.Join(l.ingredients, ", "))
	}
	return agent.Job{
		System: cartFillSystem,
		Prompt: strings.TrimSpace(prompt.String()),
		Start: func(ctx context.Context, tr agent.Trace) error {
			return s.store.CheckSession(ctx, tr)
		},
		Tools: []agent.Tool{s.cartLookUpTool(fill), s.setCartQuantityTool(fill)},
		Finish: func(ctx context.Context, tr agent.Trace) error {
			return s.finishCartFill(ctx, tr, c.ID, fill)
		},
	}
}

func (s *Service) cartLookUpTool(fill *fillState) agent.Tool {
	return agent.Tool{
		Name:        "look_up_product",
		Description: "Whether Esselunga has a product on the Grocery List in stock now. What it shows is recorded in the Known Catalog.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {"code": {"type": "string"}}, "required": ["code"]}`),
		Run: func(ctx context.Context, tr agent.Trace, input json.RawMessage) (string, error) {
			var in struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return "", err
			}
			l, err := fill.line(in.Code)
			if err != nil {
				return "", err
			}
			p, err := s.store.LookUpProduct(ctx, tr, l.code)
			unlisted := errors.Is(err, esselunga.ErrNoSuchProduct)
			if err != nil && !unlisted {
				return "", esselunga.ToolFailure(err)
			}
			fill.seen[l.code] = p.Available
			fill.unlisted[l.code] = unlisted
			if err := s.catalog.SeeAvailability(ctx, l.code, p.Available); err != nil {
				return "", fmt.Errorf("record its availability: %w", err)
			}
			switch {
			case unlisted:
				return fmt.Sprintf("Esselunga no longer sells %s: leave it out of the cart.", l.name), nil
			case !p.Available:
				return fmt.Sprintf("%s is out of stock: leave it out of the cart.", l.name), nil
			}
			return fmt.Sprintf("%s is available: put %d in the cart.", l.name, l.packs), nil
		},
	}
}

func (s *Service) setCartQuantityTool(fill *fillState) agent.Tool {
	return agent.Tool{
		Name:        "set_cart_quantity",
		Description: "Put the packs the Grocery List buys of a product you looked up and found available in the Esselunga cart, whatever the cart held of it.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {"code": {"type": "string"}, "quantity": {"type": "integer", "minimum": 1}}, "required": ["code", "quantity"]}`),
		Run: func(ctx context.Context, tr agent.Trace, input json.RawMessage) (string, error) {
			var in struct {
				Code     string `json:"code"`
				Quantity int    `json:"quantity"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return "", err
			}
			l, err := fill.line(in.Code)
			if err != nil {
				return "", err
			}
			if available, seen := fill.seen[l.code]; !seen {
				return "", fmt.Errorf("look up product %s before putting it in the cart", l.code)
			} else if !available {
				return "", fmt.Errorf("%s is not available: leave it out of the cart", l.name)
			}
			if in.Quantity != l.packs {
				return "", fmt.Errorf("the Grocery List buys %d of %s, not %d", l.packs, l.name, in.Quantity)
			}
			if err := s.store.SetCartQuantity(ctx, tr, l.code, l.packs); err != nil {
				return "", esselunga.ToolFailure(err)
			}
			fill.inCart[l.code] = true
			return fmt.Sprintf("The cart holds %d × %s.", l.packs, l.name), nil
		},
	}
}

// finishCartFill ends the Cart Fill Run of the Cycle with id: it has the
// planner replan around what Esselunga is out of, sums up the Run for the
// user, and fails it if a product is neither in the cart nor out of stock,
// so it is filled again.
func (s *Service) finishCartFill(ctx context.Context, tr agent.Trace, id int64, fill *fillState) error {
	var missing []*cartLine
	var unfilled []string
	for _, l := range fill.lines {
		switch {
		case fill.missing(l):
			missing = append(missing, l)
		case !fill.inCart[l.code]:
			unfilled = append(unfilled, l.name)
		}
	}
	var substituted string
	if len(missing) > 0 {
		var err error
		if substituted, err = s.substitute(ctx, id, missing); err != nil {
			return err
		}
	}
	summary := fill.summary(substituted)
	tr.Emit(agent.Event{Kind: agent.EventStep, Summary: summary})
	if err := setCartFillSummary(ctx, s.pool, runs.IDFrom(ctx), summary); err != nil {
		return err
	}
	if len(unfilled) > 0 {
		return fmt.Errorf("these are not in the cart: %s. Fill the cart again to retry them", strings.Join(unfilled, ", "))
	}
	return nil
}

// substitute has a planning Run replan the Cycle with id around missing,
// what Esselunga is out of, and says what became of it. Like an Exception
// on the upcoming Cycle, it replaces that Cycle's pending Plan Proposal and
// cancels its planning Run going; while the current Cycle's turn comes
// first, it marks the Plan as needing a new Plan Proposal instead. It
// changes nothing once the Delivery is ordered, or when the same
// substitution is already under way, as when Cart Fill runs again.
func (s *Service) substitute(ctx context.Context, id int64, missing []*cartLine) (string, error) {
	var ingredients []string
	var describe strings.Builder
	for _, l := range missing {
		ingredients = append(ingredients, l.ingredients...)
		fmt.Fprintf(&describe, "- %s, bought as %s\n", strings.Join(l.ingredients, ", "), l.name)
	}
	out := strings.Join(ingredients, ", ")
	described := strings.TrimSpace(describe.String())
	var started runs.Recorded
	var outcome string
	err := s.write(ctx, func(tx pgx.Tx, cancelled *cancels) error {
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		c, err := findCycle(ctx, tx, id)
		switch {
		case err != nil:
			return err
		case c == nil || c.Status != "upcoming":
			outcome = "its Delivery has arrived, so its Plan stays as it is"
			return nil
		case c.Delivery.Ordered:
			outcome = "its Delivery is ordered, so its Plan stays as it is"
			return nil
		}
		going, err := s.runs.Going(ctx, tx, planningKind)
		if err != nil {
			return err
		}
		if run, pending, err := substitutionUnderWay(ctx, tx, id, described, going); err != nil {
			return err
		} else if pending {
			outcome = fmt.Sprintf("their substitution Plan Proposal, from [planning Run %d](%s), is pending: approve or reject it", run, runURL(run))
			return nil
		} else if run != 0 {
			outcome = fmt.Sprintf("[planning Run %d](%s) is already replanning around them", run, runURL(run))
			return nil
		}
		// The upcoming Cycle's own Plan Proposal and planning Run make way;
		// the current Cycle's don't.
		var cannot *CannotPlanError
		flag := func() error {
			outcome = fmt.Sprintf("the planner can't replan now, since %s: the Plan needs a new Plan Proposal", cannot.Reason)
			return setNeedsProposal(ctx, tx, id, "Esselunga is out of "+out)
		}
		from := domain.FirstUnprepared(c.Start, s.today())
		if err := s.turn(ctx, tx, *c, from, askAny); errors.As(err, &cannot) && cannot.WaitsTurn {
			return flag()
		} else if err != nil && !errors.As(err, &cannot) {
			return err
		}
		if err := s.end(ctx, tx, cancelled, []int64{id}, endReplaced, "Replaced by the Cart Fill substitution for "+out); err != nil {
			return err
		}
		started, err = s.startPlanningIn(ctx, tx, id, askAny, 0, "", described)
		if errors.As(err, &cannot) {
			return flag()
		}
		return err
	})
	if err != nil {
		return "", err
	}
	if started.ID != 0 {
		s.runs.Launch(started)
		outcome = fmt.Sprintf("[planning Run %d](%s) replans around them in a substitution Plan Proposal", started.ID, runURL(started.ID))
	}
	return outcome, nil
}

// CartFill is how filling the cart for a Cycle stands.
type CartFill struct {
	// CanFill is whether Cart Fill can start now.
	CanFill bool
	// Run is its last Cart Fill Run, with its Trace; nil when it had none.
	Run *runs.Run
	// Summary is what Run added, substituted, skipped and failed, in
	// Markdown, once it finished.
	Summary string
}

// cartFill loads how filling the cart for c, whose Grocery List is list,
// stands.
func (s *Service) cartFill(ctx context.Context, q db.Querier, c Cycle, list domain.GroceryList) (CartFill, error) {
	var f CartFill
	err := s.canFill(ctx, q, c, list)
	if err != nil && !errors.Is(err, ErrCannotFill) {
		return f, err
	}
	f.CanFill = err == nil
	id, summary, found, err := latestCartFillRun(ctx, q, c.ID)
	if err != nil || !found {
		return f, err
	}
	f.Summary = summary
	run, found, err := s.runs.Get(ctx, id)
	if err != nil || !found {
		return f, err
	}
	if run.Events, err = s.runs.Trace(ctx, id, 0); err != nil {
		return f, err
	}
	f.Run = &run
	return f, nil
}
