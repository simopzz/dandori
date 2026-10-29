package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/esselunga"
)

const (
	// seedKind is the kind of Run that seeds the Known Catalog from the
	// user's Esselunga order history.
	seedKind = "catalog seed"
	// refreshKind is the kind of Run that brings the Known Catalog's
	// Catalog Products up to date with what Esselunga shows now.
	refreshKind = "catalog refresh"
	// proposalKind is the kind of Run that proposes Product Mappings for
	// Ingredients without one.
	proposalKind = "product mapping proposal"
)

// brief is what a catalog Run is told when it starts: the Known Catalog as
// it is, and the Ingredients still without a Product Mapping.
type brief struct {
	products []Product
	unmapped []domain.Ingredient
}

func (s *Service) brief(ctx context.Context) (brief, error) {
	var b brief
	var err error
	if b.products, err = loadProducts(ctx, s.pool); err != nil {
		return brief{}, err
	}
	mappings, err := loadMappings(ctx, s.pool)
	if err != nil {
		return brief{}, err
	}
	for _, m := range mappings {
		if m.ProductID == 0 {
			b.unmapped = append(b.unmapped, m.Ingredient)
		}
	}
	return b, nil
}

// String is the brief as the model is told it.
func (b brief) String() string {
	var w strings.Builder
	w.WriteString(b.catalog())
	w.WriteString("\nIngredients without a Product Mapping:\n")
	if len(b.unmapped) == 0 {
		w.WriteString("(none)\n")
	}
	for _, ing := range b.unmapped {
		fmt.Fprintf(&w, "- %s (%s)\n", ing.Name, ing.Unit)
	}
	return w.String()
}

// catalog is the Known Catalog as the model is told it.
func (b brief) catalog() string {
	var w strings.Builder
	w.WriteString("Known Catalog:\n")
	if len(b.products) == 0 {
		w.WriteString("(empty)\n")
	}
	for _, p := range b.products {
		fmt.Fprintf(&w, "- %s: %s (%s %s)\n", p.Code, p.Name, formatAmount(p.PackQuantity), p.Unit)
	}
	return w.String()
}

// proposeRules is how the agent proposes Product Mappings; %[1]s is the
// name of the Content Language.
const proposeRules = `For each Ingredient without a Product Mapping, if a Catalog Product is clearly that food and is sold in the Ingredient's unit, call propose_product_mapping. Propose nothing for an Ingredient no product fits: the user maps it by hand.

Ingredient names are in %[1]s; Catalog Product names are Esselunga's own, in Italian.`

// labelRules is how the agent reads a pack and its nutrition label into a
// Catalog Product's pack quantity and nutrition, whichever Run saves it.
const labelRules = `- pack_quantity is how much one pack holds in that unit: "6 x 125 g" is 750 g, "1,5 L" is 1500 ml, "6 x 150 cL" is 9000 ml. For canned food use the net weight, not the drained weight.
- nutrition is per 100 g or per 100 ml, or per piece for unit piece, read from the nutrition label: kcal, protein, carbohydrate and fat. Leave it out when the product has no label, or for unit piece when the label doesn't give a piece's weight. Never estimate it.`

// seedSystem is the seed Run's instructions; %[1]d is how many orders it
// reads, %[2]s the label rules and %[3]s the proposal rules.
const seedSystem = `You seed the Known Catalog: the Esselunga products the user buys, with pack sizes and nutrition the meal planner plans with.

1. Call read_order_history to see the products of the user's last %[1]d orders.
2. Call look_up_product for each food product; skip what isn't food (cleaning products, toiletries). Look up several in one turn.
3. Call save_catalog_product for each food product you looked up, giving its pack and nutrition in one unit:
- unit is g for solids, ml for liquids, piece for what is sold by count (eggs).
%[2]s
- For a product sold by weight, pack_quantity is the approximate weight the order history gives.
4. Where several Catalog Products fit an Ingredient, propose the one the order history shows the user buys most. %[3]s

End your turn with one short sentence saying what you saved.`

// refreshSystem is the refresh Run's instructions; %[1]s is the label
// rules and %[2]s the name of the Content Language.
const refreshSystem = `You refresh the Known Catalog: you bring each Catalog Product up to date with what Esselunga shows for it now, so the meal planner plans with its current pack size, nutrition, price and availability.

1. Call look_up_product for each Catalog Product. Look up several in one turn.
2. Call save_catalog_product for each product you looked up, giving its pack and nutrition in one unit, read again from what Esselunga shows now:
- Keep the unit it is saved in unless Esselunga now sells it differently.
%[1]s
- For a product sold by weight, keep the pack_quantity it is saved with.
3. A product Esselunga no longer has stays in the Known Catalog as it is: don't save it.

End your turn with a short summary: what changed (pack sizes, nutrition, price, availability), and each product Esselunga no longer has. Write your summary in %[2]s; Catalog Product names are Esselunga's own, in Italian: keep them as they are.`

// proposalSystem is the proposal Run's instructions; %[1]s is the proposal
// rules.
const proposalSystem = `You propose Product Mappings: the Catalog Product to buy for each of the user's Ingredients, from their Known Catalog.

%[1]s

End your turn with one short sentence saying what you proposed.`

func (s *Service) seedJob(b brief) agent.Job {
	looked := map[string]lookedUp{}
	return agent.Job{
		System: fmt.Sprintf(seedSystem, s.cfg.Orders, labelRules, fmt.Sprintf(proposeRules, s.cfg.Language.Name())),
		Prompt: b.String(),
		Start: func(ctx context.Context, tr agent.Trace) error {
			return s.cfg.Store.CheckSession(ctx, tr)
		},
		Tools: []agent.Tool{
			s.readOrderHistory(),
			s.lookUpProduct(looked),
			s.saveCatalogProduct(looked),
			s.proposeProductMapping(),
		},
	}
}

func (s *Service) refreshJob(b brief) agent.Job {
	looked := map[string]lookedUp{}
	return agent.Job{
		System: fmt.Sprintf(refreshSystem, labelRules, s.cfg.Language.Name()),
		Prompt: b.catalog(),
		Start: func(ctx context.Context, tr agent.Trace) error {
			return s.cfg.Store.CheckSession(ctx, tr)
		},
		Tools: []agent.Tool{
			s.lookUpProduct(looked),
			s.saveCatalogProduct(looked),
		},
	}
}

func (s *Service) proposalJob(b brief) agent.Job {
	return agent.Job{
		System: fmt.Sprintf(proposalSystem, fmt.Sprintf(proposeRules, s.cfg.Language.Name())),
		Prompt: b.String(),
		Tools:  []agent.Tool{s.proposeProductMapping()},
	}
}

// storeFailure is how a tool reports a store error: one the agent can't
// work around ends the Run.
func storeFailure(err error) error {
	if errors.Is(err, esselunga.ErrLoggedOut) || errors.Is(err, esselunga.ErrNoStore) {
		return agent.Stop(err)
	}
	return err
}

// orderedProduct is one product of the order history, as the model sees it.
type orderedProduct struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	TimesBought int     `json:"times_bought"`
	LastPrice   float64 `json:"last_price"`
	Weight      string  `json:"weight,omitempty"`
}

func (s *Service) readOrderHistory() agent.Tool {
	return agent.Tool{
		Name:        "read_order_history",
		Description: "The products of the user's latest Esselunga orders, each once, most recent first. weight is the approximate weight of one of a product sold by weight.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {}}`),
		Run: func(ctx context.Context, tr agent.Trace, _ json.RawMessage) (string, error) {
			orders, err := s.cfg.Store.OrderHistory(ctx, tr, s.cfg.Orders)
			if err != nil {
				return "", storeFailure(err)
			}
			var products []orderedProduct
			index := map[string]int{} // of each product in products, by code
			for _, o := range orders {
				lines, err := s.cfg.Store.OrderLines(ctx, tr, o.ID)
				if err != nil {
					return "", storeFailure(err)
				}
				for _, l := range lines {
					if i, ok := index[l.Code]; ok {
						products[i].TimesBought++
						continue
					}
					index[l.Code] = len(products)
					products = append(products, orderedProduct{Code: l.Code, Name: l.Name, TimesBought: 1, LastPrice: l.Price, Weight: l.Weight})
				}
			}
			out, err := json.Marshal(map[string]any{"orders": len(orders), "products": products})
			return string(out), err
		},
	}
}

// lookedUp is a product as the store showed it, and when.
type lookedUp struct {
	esselunga.Product
	at time.Time
}

func (s *Service) lookUpProduct(looked map[string]lookedUp) agent.Tool {
	return agent.Tool{
		Name:        "look_up_product",
		Description: "What Esselunga shows for a product: its name, pack quantity, aisle, price (per kg when sold_by_weight), availability and nutrition label.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {"code": {"type": "string"}}, "required": ["code"]}`),
		Run: func(ctx context.Context, tr agent.Trace, input json.RawMessage) (string, error) {
			var in struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return "", err
			}
			p, err := s.cfg.Store.LookUpProduct(ctx, tr, in.Code)
			if err != nil {
				return "", storeFailure(err)
			}
			looked[p.Code] = lookedUp{Product: p, at: s.cfg.Now()}
			out, err := json.Marshal(map[string]any{
				"code": p.Code, "name": p.Name, "brand": p.Brand, "quantity": p.Quantity, "sold_by_weight": p.SoldByWeight,
				"aisle": p.Aisle, "price": p.Price, "available": p.Available,
				"nutrition_per": p.NutritionPer, "nutrition": p.Nutrition,
			})
			return string(out), err
		},
	}
}

var saveCatalogProductSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "code": {"type": "string"},
    "pack_quantity": {"type": "number", "exclusiveMinimum": 0},
    "unit": {"type": "string", "enum": ["g", "ml", "piece"]},
    "nutrition": {
      "type": "object",
      "description": "Per 100 g, per 100 ml or per piece, as unit says. Leave out when the product has no nutrition label.",
      "properties": {
        "kcal": {"type": "number"},
        "protein_g": {"type": "number"},
        "carbs_g": {"type": "number"},
        "fat_g": {"type": "number"}
      },
      "required": ["kcal", "protein_g", "carbs_g", "fat_g"]
    }
  },
  "required": ["code", "pack_quantity", "unit"]
}`)

// nutrition is a nutrition label as the model saves it.
type nutrition struct {
	Kcal     *float64 `json:"kcal"`
	ProteinG *float64 `json:"protein_g"`
	CarbsG   *float64 `json:"carbs_g"`
	FatG     *float64 `json:"fat_g"`
}

// UnmarshalJSON requires every value, so one the model left out is not
// taken as 0.
func (n *nutrition) UnmarshalJSON(data []byte) error {
	type values nutrition
	var v values
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.Kcal == nil || v.ProteinG == nil || v.CarbsG == nil || v.FatG == nil {
		return errors.New("nutrition needs kcal, protein_g, carbs_g and fat_g")
	}
	*n = nutrition(v)
	return nil
}

func (s *Service) saveCatalogProduct(looked map[string]lookedUp) agent.Tool {
	return agent.Tool{
		Name:        "save_catalog_product",
		Description: "Save a product you looked up to the Known Catalog, with its pack and nutrition in one unit. Its name, aisle, price and availability are what Esselunga showed; the price of a product sold by weight is worked out for the pack. Replaces the product if it is already there.",
		InputSchema: saveCatalogProductSchema,
		Run: func(ctx context.Context, _ agent.Trace, input json.RawMessage) (string, error) {
			var in struct {
				Code         string      `json:"code"`
				PackQuantity float64     `json:"pack_quantity"`
				Unit         domain.Unit `json:"unit"`
				Nutrition    *nutrition  `json:"nutrition"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return "", fmt.Errorf("the product does not match the schema: %w", err)
			}
			seen, ok := looked[in.Code]
			if !ok {
				return "", fmt.Errorf("look up product %s before saving it", in.Code)
			}
			p := domain.CatalogProduct{
				Code: seen.Code, Name: seen.Name,
				PackQuantity: in.PackQuantity, Unit: in.Unit,
				Aisle: seen.Aisle, Price: seen.Price,
				Available: seen.Available, AvailabilitySeen: seen.at,
			}
			if in.Nutrition != nil {
				p.Nutrition = &domain.IngredientNutrition{Kcal: *in.Nutrition.Kcal, ProteinG: *in.Nutrition.ProteinG, CarbsG: *in.Nutrition.CarbsG, FatG: *in.Nutrition.FatG}
			}
			if seen.SoldByWeight {
				if p.Unit != domain.Grams {
					return "", fmt.Errorf("%s is sold by weight: save it in g, at the weight of one", seen.Name)
				}
				// Its price is per kilogram; a pack is the weight of one.
				p.Price = math.Round(seen.Price*p.PackQuantity/10) / 100
			}
			if invalid := p.Validate(); len(invalid) > 0 {
				var problems []string
				for _, k := range slices.Sorted(maps.Keys(invalid)) {
					problems = append(problems, k+": "+invalid[k])
				}
				return "", errors.New("fix these and save again:\n" + strings.Join(problems, "\n"))
			}
			// Saved in another unit, it would no longer be sold in its
			// Ingredients' units.
			mapped, err := mappedIngredients(ctx, s.pool, p.Code)
			if err != nil {
				return "", fmt.Errorf("load its Product Mappings: %w", err)
			}
			for _, ing := range mapped {
				if domain.MappingProblem(ing, p) != "" {
					return "", fmt.Errorf("%s is the Product Mapping of %s, measured in %s: save it in %s", p.Name, ing.Name, ing.Unit, ing.Unit)
				}
			}
			if err := saveProduct(ctx, s.pool, p); err != nil {
				return "", fmt.Errorf("store the product: %w", err)
			}
			return "Saved " + p.Name + " to the Known Catalog.", nil
		},
	}
}

func (s *Service) proposeProductMapping() agent.Tool {
	return agent.Tool{
		Name:        "propose_product_mapping",
		Description: "Propose a Catalog Product as the one to buy for an Ingredient without a Product Mapping. The user confirms or changes it.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {"ingredient": {"type": "string"}, "code": {"type": "string"}}, "required": ["ingredient", "code"]}`),
		Run: func(ctx context.Context, _ agent.Trace, input json.RawMessage) (string, error) {
			var in struct {
				Ingredient string `json:"ingredient"`
				Code       string `json:"code"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return "", err
			}
			ingID, ing, found, err := loadIngredient(ctx, s.pool, 0, domain.IngredientName(in.Ingredient))
			if err != nil {
				return "", err
			}
			if !found {
				return "", fmt.Errorf("there is no Ingredient named %q", in.Ingredient)
			}
			p, found, err := loadProduct(ctx, s.pool, 0, in.Code)
			if err != nil {
				return "", err
			}
			if !found {
				return "", fmt.Errorf("the Known Catalog has no product %s", in.Code)
			}
			if problem := domain.MappingProblem(ing, p.CatalogProduct); problem != "" {
				return "", errors.New(problem)
			}
			proposed, err := proposeMapping(ctx, s.pool, ingID, p.ID)
			if err != nil {
				return "", fmt.Errorf("store the proposal: %w", err)
			}
			if !proposed {
				return ing.Name + " already has a Product Mapping; it is left as it is.", nil
			}
			return fmt.Sprintf("Proposed %s for %s.", p.Name, ing.Name), nil
		},
	}
}

// Tool lists the Known Catalog as an agent tool, for the planner: what can
// be bought, in which packs, and which Catalog Product each Ingredient is
// bought as.
func (s *Service) Tool() agent.Tool {
	return agent.Tool{
		Name: "list_catalog",
		Description: "List the Known Catalog: each Catalog Product with its pack size and whether it was last seen available, and each Ingredient's Product Mapping, the Catalog Product it is bought as. " +
			"Design Prep Sessions around whole packs, so little is left over.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {}}`),
		Run: func(ctx context.Context, _ agent.Trace, _ json.RawMessage) (string, error) {
			products, err := loadProducts(ctx, s.pool)
			if err != nil {
				return "", err
			}
			mappings, err := loadMappings(ctx, s.pool)
			if err != nil {
				return "", err
			}
			return describeCatalog(products, mappings), nil
		},
	}
}

// describeCatalog is the Known Catalog and the Product Mappings as the
// planner reads them.
func describeCatalog(products []Product, mappings []Mapping) string {
	var w strings.Builder
	w.WriteString("Known Catalog:\n")
	if len(products) == 0 {
		w.WriteString("(empty)\n")
	}
	for _, p := range products {
		available := "available"
		if !p.Available {
			available = "unavailable"
		}
		fmt.Fprintf(&w, "- %s: %s, a pack of %s %s, %s\n", p.Code, p.Name, formatAmount(p.PackQuantity), p.Unit, available)
	}
	w.WriteString("\nProduct Mappings:\n")
	mapped := 0
	for _, m := range mappings {
		if m.ProductID == 0 || m.Proposed {
			continue
		}
		mapped++
		fmt.Fprintf(&w, "- %s is bought as %s\n", m.Ingredient.Name, m.ProductName)
	}
	if mapped == 0 {
		w.WriteString("(none)\n")
	}
	return w.String()
}
