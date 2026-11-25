package domain_test

import (
	"reflect"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// onion and salt are Ingredients to shop for: one counted in pieces, one a Staple.
var (
	onion = domain.Ingredient{ID: 11, Name: "onion", Unit: domain.Pieces, Usual: domain.Pantry}
	salt  = domain.Ingredient{ID: 12, Name: "salt", Unit: domain.Grams, Usual: domain.Pantry, Staple: true}
)

// The Grocery List of a Plan against the kitchen forecast is, per Ingredient, what its
// Prep Sessions need minus what the forecast holds of it in every Storage Location,
// freezer included, listed only above zero, in name order, rounded up to whole units.
// Staples are never lines: those the Plan uses are named apart, once each.
func TestTheGroceryListOfAPlan(t *testing.T) {
	plan := func(sessions ...domain.PrepSession) domain.Plan { return domain.Plan{PrepSessions: sessions} }
	held := func(ing domain.Ingredient, loc domain.StorageLocation, v float64) map[domain.StockKey]domain.Quantity {
		return map[domain.StockKey]domain.Quantity{{Ingredient: ing.ID, Location: loc}: domain.QuantityOf(v)}
	}
	line := func(ing domain.Ingredient, v float64) domain.GroceryLine {
		return domain.GroceryLine{Ingredient: ing, Quantity: domain.QuantityOf(v)}
	}
	tests := []struct {
		name  string
		plan  domain.Plan
		stock map[domain.StockKey]domain.Quantity
		want  domain.GroceryList
	}{
		{"an empty Plan has an empty list", domain.Plan{}, held(breast, domain.Fridge, 500), domain.GroceryList{}},
		{"what every Prep Session needs, with nothing held", plan(setsOut(4, breast, 300), setsOut(5, breast, 200)), nil,
			domain.GroceryList{Lines: []domain.GroceryLine{line(breast, 500)}}},
		{"less what the forecast holds, freezer included",
			plan(setsOut(4, breast, 300), setsOut(5, breast, 200)),
			map[domain.StockKey]domain.Quantity{{Ingredient: breast.ID, Location: domain.Fridge}: 2000, {Ingredient: breast.ID, Location: domain.Freezer}: 1000},
			domain.GroceryList{Lines: []domain.GroceryLine{line(breast, 200)}}},
		{"nothing when it holds enough", plan(setsOut(4, breast, 300)), held(breast, domain.Freezer, 300), domain.GroceryList{}},
		{"in name order", plan(setsOut(4, onion, 1), setsOut(5, breast, 100), setsOut(6, peas, 50)), nil,
			domain.GroceryList{Lines: []domain.GroceryLine{line(breast, 100), line(peas, 50), line(onion, 1)}}},
		{"pieces round up to whole pieces", plan(setsOut(4, onion, 2)), held(onion, domain.Pantry, 0.5),
			domain.GroceryList{Lines: []domain.GroceryLine{line(onion, 2)}}},
		{"and grams to whole grams", plan(setsOut(4, breast, 37.5)), nil,
			domain.GroceryList{Lines: []domain.GroceryLine{line(breast, 38)}}},
		{"a Staple is never a line, and is named once", plan(setsOut(4, salt, 5), setsOut(5, salt, 5), setsOut(6, breast, 100)), nil,
			domain.GroceryList{Lines: []domain.GroceryLine{line(breast, 100)}, Staples: []domain.Ingredient{salt}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.GroceryList(tt.stock); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GroceryList = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Putting a Delivery away places each Grocery List line in its Ingredient's usual Storage
// Location; a freezer split sends that much of it to the freezer and the rest to the
// usual place, never more than the line brings; a split of an Ingredient the list doesn't
// buy puts nothing away.
func TestPuttingADeliveryAway(t *testing.T) {
	line := func(ing domain.Ingredient, v float64) domain.GroceryLine {
		return domain.GroceryLine{Ingredient: ing, Quantity: domain.QuantityOf(v)}
	}
	split := func(ing domain.Ingredient, v float64) domain.RecipeIngredient {
		return domain.RecipeIngredient{Ingredient: ing, Quantity: domain.QuantityOf(v)}
	}
	put := func(ing domain.Ingredient, loc domain.StorageLocation, v float64) domain.PutAwayLine {
		return domain.PutAwayLine{GroceryLine: line(ing, v), Location: loc}
	}
	tests := []struct {
		name   string
		lines  []domain.GroceryLine
		splits []domain.RecipeIngredient
		want   []domain.PutAwayLine
	}{
		{"each line in its usual Storage Location", []domain.GroceryLine{line(breast, 800), line(onion, 2)}, nil,
			[]domain.PutAwayLine{put(breast, domain.Fridge, 800), put(onion, domain.Pantry, 2)}},
		{"a split to the freezer, the rest in the usual place", []domain.GroceryLine{line(breast, 800)}, []domain.RecipeIngredient{split(breast, 400)},
			[]domain.PutAwayLine{put(breast, domain.Fridge, 400), put(breast, domain.Freezer, 400)}},
		{"a split capped at what the line brings", []domain.GroceryLine{line(breast, 300)}, []domain.RecipeIngredient{split(breast, 400)},
			[]domain.PutAwayLine{put(breast, domain.Freezer, 300)}},
		{"a split of an Ingredient not on the list puts nothing away", []domain.GroceryLine{line(onion, 2)}, []domain.RecipeIngredient{split(breast, 400)},
			[]domain.PutAwayLine{put(onion, domain.Pantry, 2)}},
		{"an Ingredient usually kept in the freezer", []domain.GroceryLine{line(peas, 500)}, []domain.RecipeIngredient{split(peas, 200)},
			[]domain.PutAwayLine{put(peas, domain.Freezer, 500)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.PutAway(tt.lines, tt.splits); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PutAway = %+v, want %+v", got, tt.want)
			}
		})
	}
}
