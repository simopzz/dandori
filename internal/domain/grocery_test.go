package domain_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

func TestGroceryListRoundsWhatThePlanNeedsUpToWholePacks(t *testing.T) {
	delivery := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	oats := domain.Ingredient{Name: "rolled oats", Unit: domain.Grams}
	milk := domain.Ingredient{Name: "milk", Unit: domain.Millilitres}
	eggs := domain.Ingredient{Name: "egg", Unit: domain.Pieces}
	porridge := &domain.Recipe{Name: "Porridge", Portions: 2, Ingredients: []domain.RecipeIngredient{
		{Ingredient: oats, Quantity: 160},
		{Ingredient: milk, Quantity: 500},
	}}
	session := func(r *domain.Recipe, portions int, ingredients ...domain.RecipeIngredient) domain.PrepSession {
		return domain.PrepSession{Recipe: r, Ingredients: ingredients, Portions: make([]domain.PlannedPortion, portions)}
	}
	product := func(name string, pack float64, unit domain.Unit, aisle string) domain.CatalogProduct {
		return domain.CatalogProduct{Code: name, Name: name, PackQuantity: pack, Unit: unit, Aisle: aisle}
	}
	oatsPack := domain.ProductMapping{Product: product("Fiocchi d'avena 500 g", 500, domain.Grams, "Dispensa › Cereali")}
	milkPack := domain.ProductMapping{Product: product("Latte 1 l", 1000, domain.Millilitres, "Latte e latticini › Latte")}
	eggBox := domain.ProductMapping{Product: product("Uova x6", 6, domain.Pieces, "Latte e latticini › Uova"), Proposed: true}
	everyMapping := map[string]domain.ProductMapping{"rolled oats": oatsPack, "milk": milkPack, "egg": eggBox}
	item := func(ing domain.Ingredient, quantity float64, expires *time.Time) domain.StockItem {
		return domain.StockItem{Ingredient: ing, Quantity: quantity, Location: domain.Pantry, ExpiresOn: expires}
	}
	before := delivery.AddDate(0, 0, -1)

	type line struct {
		ingredient              string
		needed, bought, surplus float64
		packs                   int
		product                 string
		proposed                bool
	}
	tests := []struct {
		name     string
		plan     domain.Plan
		stock    []domain.StockItem
		mappings map[string]domain.ProductMapping
		want     []line
	}{
		{
			name:     "a Recipe scaled to its Prep Session's Portions, rounded up to whole packs",
			plan:     domain.Plan{PrepSessions: []domain.PrepSession{session(porridge, 3)}},
			mappings: everyMapping,
			want: []line{
				{ingredient: "milk", needed: 750, packs: 1, bought: 1000, surplus: 250, product: "Latte 1 l"},
				{ingredient: "rolled oats", needed: 240, packs: 1, bought: 500, surplus: 260, product: "Fiocchi d'avena 500 g"},
			},
		},
		{
			name: "needs of several Prep Sessions add up before rounding",
			plan: domain.Plan{PrepSessions: []domain.PrepSession{
				session(porridge, 2), session(porridge, 2), session(nil, 1, domain.RecipeIngredient{Ingredient: oats, Quantity: 200}),
			}},
			mappings: everyMapping,
			want: []line{
				{ingredient: "milk", needed: 1000, packs: 1, bought: 1000, surplus: 0, product: "Latte 1 l"},
				{ingredient: "rolled oats", needed: 520, packs: 2, bought: 1000, surplus: 480, product: "Fiocchi d'avena 500 g"},
			},
		},
		{
			name:     "a proposed Product Mapping is used, and says so",
			plan:     domain.Plan{PrepSessions: []domain.PrepSession{session(nil, 1, domain.RecipeIngredient{Ingredient: eggs, Quantity: 8})}},
			mappings: everyMapping,
			want:     []line{{ingredient: "egg", needed: 8, packs: 2, bought: 12, surplus: 4, product: "Uova x6", proposed: true}},
		},
		{
			name:     "stock partly covering an Ingredient leaves the rest to buy",
			plan:     domain.Plan{PrepSessions: []domain.PrepSession{session(porridge, 2)}},
			stock:    []domain.StockItem{item(oats, 100, nil), item(oats, 20, nil), item(milk, 600, nil)},
			mappings: everyMapping,
			want:     []line{{ingredient: "rolled oats", needed: 40, packs: 1, bought: 500, surplus: 460, product: "Fiocchi d'avena 500 g"}},
		},
		{
			name:     "stock expired by the Delivery day doesn't count",
			plan:     domain.Plan{PrepSessions: []domain.PrepSession{session(porridge, 2)}},
			stock:    []domain.StockItem{item(oats, 500, &before), item(milk, 500, &delivery)},
			mappings: everyMapping,
			want:     []line{{ingredient: "rolled oats", needed: 160, packs: 1, bought: 500, surplus: 340, product: "Fiocchi d'avena 500 g"}},
		},
		{
			name:     "an unmapped Ingredient is listed at what is needed",
			plan:     domain.Plan{PrepSessions: []domain.PrepSession{session(porridge, 2)}},
			mappings: map[string]domain.ProductMapping{"rolled oats": oatsPack},
			want: []line{
				{ingredient: "milk", needed: 500, bought: 500},
				{ingredient: "rolled oats", needed: 160, packs: 1, bought: 500, surplus: 340, product: "Fiocchi d'avena 500 g"},
			},
		},
		{
			name: "no Plan, nothing to buy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.NewGroceryList(tt.plan, tt.stock, tt.mappings, delivery)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				product, proposed := "", false
				if g.Mapping != nil {
					product, proposed = g.Mapping.Product.Name, g.Mapping.Proposed
				}
				gl := line{
					ingredient: g.Ingredient.Name, needed: g.Needed, bought: g.Bought(), surplus: g.Surplus(),
					packs: g.Packs, product: product, proposed: proposed,
				}
				if gl != w {
					t.Errorf("line %d = %+v, want %+v", i, gl, w)
				}
			}
		})
	}
}

func TestGroceryListGroupsLinesByAisleWithUnmappedOnesLast(t *testing.T) {
	line := func(ingredient, aisle string, mapped bool) domain.GroceryLine {
		l := domain.GroceryLine{Ingredient: domain.Ingredient{Name: ingredient}, Needed: 1}
		if mapped {
			l.Mapping = &domain.ProductMapping{Product: domain.CatalogProduct{PackQuantity: 1, Aisle: aisle}}
			l.Packs = 1
		}
		return l
	}
	list := domain.GroceryList{
		line("basil", "", false),
		line("chickpeas", "Dispensa › Legumi", true),
		line("milk", "Latte e latticini", true),
		line("parsley", "", true),
		line("rice", "Dispensa › Riso", true),
		line("yoghurt", "Latte e latticini", true),
	}
	var got []string
	for _, a := range list.ByAisle() {
		var names []string
		for _, l := range a.Lines {
			names = append(names, l.Ingredient.Name)
		}
		got = append(got, fmt.Sprintf("%s [%v]: %s", a.Aisle, a.Unmapped, strings.Join(names, ", ")))
	}
	want := []string{
		"Dispensa › Legumi [false]: chickpeas",
		"Dispensa › Riso [false]: rice",
		"Latte e latticini [false]: milk, yoghurt",
		" [false]: parsley",
		" [true]: basil",
	}
	if !slices.Equal(got, want) {
		t.Errorf("ByAisle =\n%q\nwant\n%q", got, want)
	}
}

func TestGroceryListBecomesStockItemsOfWhatItBrings(t *testing.T) {
	oats := domain.Ingredient{Name: "rolled oats", Unit: domain.Grams}
	peas := domain.Ingredient{Name: "peas", Unit: domain.Grams}
	milk := domain.Ingredient{Name: "milk", Unit: domain.Millilitres}
	basil := domain.Ingredient{Name: "basil", Unit: domain.Grams}
	mapped := func(ing domain.Ingredient, needed, pack float64, packs int, aisle string) domain.GroceryLine {
		return domain.GroceryLine{Ingredient: ing, Needed: needed, Packs: packs,
			Mapping: &domain.ProductMapping{Product: domain.CatalogProduct{PackQuantity: pack, Aisle: aisle}}}
	}
	list := domain.GroceryList{
		{Ingredient: basil, Needed: 20},
		mapped(milk, 750, 1000, 1, "Latte, burro e panna › Latte fresco"),
		mapped(peas, 300, 450, 1, "Surgelati › Verdure"),
		mapped(oats, 520, 500, 2, "Dispensa › Cereali"),
	}
	want := []domain.StockItem{
		{Ingredient: basil, Quantity: 20, Location: domain.Pantry},
		{Ingredient: milk, Quantity: 1000, Location: domain.Fridge},
		{Ingredient: peas, Quantity: 450, Location: domain.Freezer},
		{Ingredient: oats, Quantity: 1000, Location: domain.Pantry},
	}
	got := list.StockItems()
	if len(got) != len(want) {
		t.Fatalf("StockItems = %+v, want %+v", got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Ingredient.Name != w.Ingredient.Name || g.Quantity != w.Quantity || g.Location != w.Location || g.ExpiresOn != nil {
			t.Errorf("StockItems[%d] = %+v, want %+v", i, g, w)
		}
	}
}

func TestAisleDecidesTheStorageLocation(t *testing.T) {
	tests := []struct {
		aisle string
		want  domain.StorageLocation
	}{
		{"Surgelati › Pesce", domain.Freezer},
		{"Gelati e surgelati", domain.Freezer},
		{"Frutta e verdura › Insalate", domain.Fridge},
		{"Latte, burro e panna", domain.Fridge},
		{"Yogurt e dessert", domain.Fridge},
		{"Carne › Pollo", domain.Fridge},
		{"Pesce e sushi", domain.Fridge},
		{"Salumi e formaggi", domain.Fridge},
		{"Uova", domain.Fridge},
		{"Gastronomia e piatti pronti freschi", domain.Fridge},
		{"Dispensa › Pasta", domain.Pantry},
		{"Bevande", domain.Pantry},
		{"", domain.Pantry},
	}
	for _, tt := range tests {
		t.Run(tt.aisle, func(t *testing.T) {
			if got := domain.AisleLocation(tt.aisle); got != tt.want {
				t.Errorf("AisleLocation(%q) = %s, want %s", tt.aisle, got, tt.want)
			}
		})
	}
}
