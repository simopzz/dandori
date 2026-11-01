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

// cartFillSystem is the Cart Fill Run's instructions; %[1]s is the name of
// the Content Language.
const cartFillSystem = `You fill the user's Esselunga cart with their Grocery List, for a Delivery they check out and pay for themselves on Esselunga. You never check out or pay.

1. Call look_up_product for each product on the Grocery List, to see whether Esselunga has it in stock now. Look up several in one turn.
2. For each one available, call set_cart_quantity with the packs the Grocery List buys. It sets the quantity rather than adding to it, so filling the cart again never doubles a product.
3. Leave a product out of stock, or one Esselunga no longer sells, out of the cart: once you end your turn, the planner replans around it in a substitution Plan Proposal for the user to approve.
4. If the store fails on a product, you may try it again once; otherwise move on.

End your turn with a short summary for the user, under these headings: Added (with the packs), Substituted (out of stock, sent to the planner), Skipped (no Product Mapping) and Failed (what the store refused: filling the cart again retries it). Say which products come from a proposed Product Mapping, so the user checks them in the cart. Write the summary in %[1]s; Catalog Product names are Esselunga's own, in Italian: keep them as they are.`

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

// cartFill is how a Cart Fill Run stands: what Esselunga showed of each
// product, and what is in the cart.
type cartFill struct {
	lines []*cartLine
	// seen is whether each product looked up, by code, is available; inCart
	// whether it was set in the cart.
	seen   map[string]bool
	inCart map[string]bool
}

func (f *cartFill) line(code string) (*cartLine, error) {
	i := slices.IndexFunc(f.lines, func(l *cartLine) bool { return l.code == code })
	if i < 0 {
		return nil, fmt.Errorf("product %s is not on the Grocery List", code)
	}
	return f.lines[i], nil
}

// cartFillJob is the Cart Fill Run of c, whose Grocery List is list.
func (s *Service) cartFillJob(c Cycle, list domain.GroceryList) agent.Job {
	fill := &cartFill{lines: cartLines(list), seen: map[string]bool{}, inCart: map[string]bool{}}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "The Grocery List for the Delivery on %s:\n", describeDelivery(c.Delivery, longDate))
	for _, l := range fill.lines {
		fmt.Fprintf(&prompt, "- %d × %s (product %s), for %s", l.packs, l.name, l.code, strings.Join(l.ingredients, ", "))
		if l.proposed {
			prompt.WriteString(", from a proposed Product Mapping")
		}
		prompt.WriteString("\n")
	}
	var unmapped []string
	for _, l := range list {
		if l.Mapping == nil {
			unmapped = append(unmapped, l.Ingredient.Name)
		}
	}
	if len(unmapped) > 0 {
		fmt.Fprintf(&prompt, "\nWithout a Product Mapping, skipped: %s\n", strings.Join(unmapped, ", "))
	}
	return agent.Job{
		System: fmt.Sprintf(cartFillSystem, s.language.Name()),
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

// storeFailure is how a Cart Fill tool reports a store error: one the agent
// can't work around ends the Run.
func storeFailure(err error) error {
	if errors.Is(err, esselunga.ErrLoggedOut) || errors.Is(err, esselunga.ErrNoStore) {
		return agent.Stop(err)
	}
	return err
}

func (s *Service) cartLookUpTool(fill *cartFill) agent.Tool {
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
				return "", storeFailure(err)
			}
			fill.seen[l.code] = p.Available
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

func (s *Service) setCartQuantityTool(fill *cartFill) agent.Tool {
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
				return "", storeFailure(err)
			}
			fill.inCart[l.code] = true
			return fmt.Sprintf("The cart holds %d × %s.", l.packs, l.name), nil
		},
	}
}

// finishCartFill ends the Cart Fill Run of the Cycle with id: it has the
// planner replan around what Esselunga is out of, and fails the Run if a
// product is neither in the cart nor out of stock, so it is filled again.
func (s *Service) finishCartFill(ctx context.Context, tr agent.Trace, id int64, fill *cartFill) error {
	var missing, unfilled []*cartLine
	for _, l := range fill.lines {
		available, seen := fill.seen[l.code]
		switch {
		case seen && !available:
			missing = append(missing, l)
		case !fill.inCart[l.code]:
			unfilled = append(unfilled, l)
		}
	}
	if len(missing) > 0 {
		if err := s.substitute(ctx, tr, id, missing); err != nil {
			return err
		}
	}
	if len(unfilled) > 0 {
		var names []string
		for _, l := range unfilled {
			names = append(names, l.name)
		}
		return fmt.Errorf("these are not in the cart: %s. Fill the cart again to retry them", strings.Join(names, ", "))
	}
	return nil
}

// substitute has a planning Run replan the Cycle with id around missing,
// what Esselunga is out of. Like an Exception on the upcoming Cycle, it
// replaces that Cycle's pending Plan Proposal and cancels its planning Run
// going; while the current Cycle's turn comes first, it marks the Plan as
// needing a new Plan Proposal instead.
func (s *Service) substitute(ctx context.Context, tr agent.Trace, id int64, missing []*cartLine) error {
	var ingredients []string
	var describe strings.Builder
	for _, l := range missing {
		ingredients = append(ingredients, l.ingredients...)
		fmt.Fprintf(&describe, "- %s, bought as %s\n", strings.Join(l.ingredients, ", "), l.name)
	}
	out := strings.Join(ingredients, ", ")
	var started runs.Recorded
	var waits string
	err := s.write(ctx, func(tx pgx.Tx, cancelled *cancels) error {
		if err := lockCycles(ctx, tx); err != nil {
			return err
		}
		c, err := findCycle(ctx, tx, id)
		if err != nil || c == nil || c.Status != "upcoming" {
			// Arrived or dropped meanwhile: there is no Plan to change.
			return err
		}
		if err := s.end(ctx, tx, cancelled, []int64{id}, endReplaced, "Replaced by the Cart Fill substitution for "+out); err != nil {
			return err
		}
		started, err = s.startPlanningIn(ctx, tx, id, askAny, 0, "", strings.TrimSpace(describe.String()))
		var cannot *CannotPlanError
		if errors.As(err, &cannot) {
			waits = cannot.Reason
			return setNeedsProposal(ctx, tx, id, "Esselunga is out of "+out)
		}
		return err
	})
	if err != nil {
		return err
	}
	if started.ID == 0 {
		if waits != "" {
			tr.Emit(agent.Event{Kind: agent.EventStep, Summary: fmt.Sprintf("Esselunga is out of %s, but the planner can't replan now, since %s: the Plan needs a new Plan Proposal.", out, waits)})
		}
		return nil
	}
	s.runs.Launch(started)
	tr.Emit(agent.Event{Kind: agent.EventStep, Summary: fmt.Sprintf("Esselunga is out of %s: planning Run %d replans around it", out, started.ID), Detail: runURL(started.ID)})
	return nil
}

// CartFill is how filling the cart for a Cycle stands.
type CartFill struct {
	// CanFill is whether Cart Fill can start now.
	CanFill bool
	// Run is its last Cart Fill Run, with its Trace; nil when it had none.
	Run *runs.Run
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
	id, found, err := latestCartFillRun(ctx, q, c.ID)
	if err != nil || !found {
		return f, err
	}
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
