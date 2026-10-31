package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

func TestPrepSessionsUseUpTheStockItemsOfTheirIngredients(t *testing.T) {
	chicken := domain.Ingredient{Name: "chicken breast", Unit: domain.Grams}
	rice := domain.Ingredient{Name: "basmati rice", Unit: domain.Grams}
	lemon := domain.Ingredient{Name: "lemon", Unit: domain.Pieces}
	// Lemon chicken serves 4: scaled to the 2 Portions a Prep Session
	// yields, it uses half of each Ingredient.
	lemonChicken := &domain.Recipe{Name: "Lemon chicken", Portions: 4, Ingredients: []domain.RecipeIngredient{
		{Ingredient: chicken, Quantity: 800},
		{Ingredient: rice, Quantity: 320},
	}}
	nov2, nov3, nov4, nov5 := day("2026-11-02"), day("2026-11-03"), day("2026-11-04"), day("2026-11-05")
	cooks := func(on time.Time, portions int) domain.PrepSession {
		return domain.PrepSession{At: domain.Slot{On: on, Kind: domain.Dinner}, Recipe: lemonChicken, Portions: make([]domain.PlannedPortion, portions)}
	}
	setsOut := func(on time.Time, ing domain.Ingredient, quantity float64) domain.PrepSession {
		return domain.PrepSession{At: domain.Slot{On: on, Kind: domain.Snack}, Ingredients: []domain.RecipeIngredient{{Ingredient: ing, Quantity: quantity}}, Portions: make([]domain.PlannedPortion, 1)}
	}
	item := func(ing domain.Ingredient, quantity float64, loc domain.StorageLocation, expires *time.Time) domain.StockItem {
		return domain.StockItem{Ingredient: ing, Quantity: quantity, Location: loc, ExpiresOn: expires}
	}
	took := func(from int, s domain.StockItem, quantity, leaves float64) domain.Taking {
		s.Quantity = quantity
		return domain.Taking{StockItem: s, From: from, Leaves: leaves}
	}
	chickenFridge := item(chicken, 640, domain.Fridge, &nov4)
	riceJar := item(rice, 1000, domain.Pantry, nil)
	tests := []struct {
		name      string
		sessions  []domain.PrepSession
		stock     []domain.StockItem
		left      []domain.StockItem
		taken     [][]domain.Taking
		shortfall []map[string]float64
	}{
		{
			name:     "a Recipe scaled to its Portions",
			sessions: []domain.PrepSession{cooks(nov2, 2)},
			stock:    []domain.StockItem{chickenFridge, riceJar},
			left:     []domain.StockItem{item(chicken, 240, domain.Fridge, &nov4), item(rice, 840, domain.Pantry, nil)},
			taken:    [][]domain.Taking{{took(0, chickenFridge, 400, 240), took(1, riceJar, 160, 840)}},
		},
		{
			name:     "a Prep Session with no Recipe uses what it sets out",
			sessions: []domain.PrepSession{setsOut(nov2, lemon, 1)},
			stock:    []domain.StockItem{item(lemon, 3, domain.Pantry, nil)},
			left:     []domain.StockItem{item(lemon, 2, domain.Pantry, nil)},
			taken:    [][]domain.Taking{{took(0, item(lemon, 3, domain.Pantry, nil), 1, 2)}},
		},
		{
			name:     "one Ingredient split across Stock Items, earliest expiry first, none last, then oldest",
			sessions: []domain.PrepSession{setsOut(nov2, chicken, 450)},
			stock: []domain.StockItem{
				item(chicken, 300, domain.Freezer, nil),
				item(chicken, 100, domain.Fridge, &nov5),
				item(chicken, 150, domain.Freezer, nil),
				item(chicken, 100, domain.Fridge, &nov3),
			},
			left: []domain.StockItem{item(chicken, 50, domain.Freezer, nil), item(chicken, 150, domain.Freezer, nil)},
			taken: [][]domain.Taking{{
				took(3, item(chicken, 100, domain.Fridge, &nov3), 100, 0),
				took(1, item(chicken, 100, domain.Fridge, &nov5), 100, 0),
				took(0, item(chicken, 300, domain.Freezer, nil), 250, 50),
			}},
		},
		{
			name:     "Stock Items expired on its day are skipped and left as they are",
			sessions: []domain.PrepSession{setsOut(nov4, chicken, 100)},
			stock:    []domain.StockItem{item(chicken, 200, domain.Fridge, &nov3), item(chicken, 200, domain.Freezer, nil)},
			left:     []domain.StockItem{item(chicken, 200, domain.Fridge, &nov3), item(chicken, 100, domain.Freezer, nil)},
			taken:    [][]domain.Taking{{took(1, item(chicken, 200, domain.Freezer, nil), 100, 100)}},
		},
		{
			name:     "a Stock Item used up exactly leaves the kitchen",
			sessions: []domain.PrepSession{cooks(nov2, 3)},
			stock:    []domain.StockItem{item(chicken, 600, domain.Fridge, nil), item(rice, 240, domain.Pantry, nil)},
			left:     nil,
			taken: [][]domain.Taking{{
				took(0, item(chicken, 600, domain.Fridge, nil), 600, 0),
				took(1, item(rice, 240, domain.Pantry, nil), 240, 0),
			}},
		},
		{
			name:      "stock that runs short is used up as far as it goes",
			sessions:  []domain.PrepSession{cooks(nov2, 2)},
			stock:     []domain.StockItem{item(chicken, 150, domain.Fridge, nil), item(chicken, 100, domain.Fridge, &nov2)},
			left:      nil,
			taken:     [][]domain.Taking{{took(1, item(chicken, 100, domain.Fridge, &nov2), 100, 0), took(0, item(chicken, 150, domain.Fridge, nil), 150, 0)}},
			shortfall: []map[string]float64{{"chicken breast": 150, "basmati rice": 160}},
		},
		{
			name: "several Prep Sessions, each from what the ones before it left",
			sessions: []domain.PrepSession{
				cooks(nov2, 2),
				setsOut(nov3, chicken, 200),
				cooks(nov4, 1),
			},
			stock: []domain.StockItem{chickenFridge, riceJar},
			left:  []domain.StockItem{item(rice, 760, domain.Pantry, nil)},
			taken: [][]domain.Taking{
				{took(0, chickenFridge, 400, 240), took(1, riceJar, 160, 840)},
				{took(0, chickenFridge, 200, 40)},
				// On the 4th the chicken still holds: it expires that day.
				{took(0, chickenFridge, 40, 0), took(1, riceJar, 80, 760)},
			},
			shortfall: []map[string]float64{nil, nil, {"chicken breast": 160}},
		},
		{
			name:     "a Prep Session from before planning has no day and uses nothing",
			sessions: []domain.PrepSession{{Recipe: lemonChicken, Portions: make([]domain.PlannedPortion, 2)}},
			stock:    []domain.StockItem{chickenFridge},
			left:     []domain.StockItem{chickenFridge},
			taken:    [][]domain.Taking{nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left, uses := domain.UseUp(tt.sessions, tt.stock)
			if !reflect.DeepEqual(left, tt.left) {
				t.Errorf("left = %+v, want %+v", left, tt.left)
			}
			if len(uses) != len(tt.sessions) {
				t.Fatalf("got %d uses for %d Prep Sessions", len(uses), len(tt.sessions))
			}
			for i, use := range uses {
				if !reflect.DeepEqual(use.Taken, tt.taken[i]) {
					t.Errorf("Prep Session %d took %+v, want %+v", i, use.Taken, tt.taken[i])
				}
				var want map[string]float64
				if tt.shortfall != nil {
					want = tt.shortfall[i]
				}
				if !reflect.DeepEqual(use.Shortfall, want) {
					t.Errorf("Prep Session %d Shortfall = %v, want %v", i, use.Shortfall, want)
				}
			}
		})
	}
}
