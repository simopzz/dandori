package domain_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// The kitchen forecast at a Slot is what the kitchen holds now plus what the Plan brings
// at the Slots still to come before it: Prep Sessions using up stock and preparing
// Portions, and Portions without a Slot eaten. What the Plan brings before now, or from
// that Slot on, changes nothing.
func TestTheKitchenForecastAtASlot(t *testing.T) {
	now, at := at(3, "08:00"), slot(5, domain.Breakfast)
	lasagne := func(ids ...int64) []domain.PortionLot {
		return []domain.PortionLot{{Name: "lasagne", Location: domain.Freezer, IDs: ids}}
	}
	kept := func(portion int64, s domain.Slot) domain.Plan {
		return domain.Plan{Kept: []domain.Kept{{Portion: portion, At: s}}}
	}
	eats := func(s domain.Slot) domain.PlannedPortion {
		return domain.PlannedPortion{Location: domain.Fridge, EatAt: &s}
	}
	// chiliIn is the Portions of chili prepared in `location`, by PlannedID.
	chiliIn := func(location domain.StorageLocation, ids ...int64) domain.PortionLot {
		return domain.PortionLot{Name: "Chili", Location: location, Nutrition: chili.NutritionPerPortion().Nutrition, IDs: ids}
	}
	// sets is a Prep Session at kind on day setting out quantity of ing, eaten there.
	sets := func(day int, kind domain.SlotKind, ing domain.Ingredient, quantity float64) domain.Plan {
		ps := setsOut(day, ing, quantity)
		ps.At, ps.Portions = slot(day, kind), []domain.PlannedPortion{eats(slot(day, kind))}
		return domain.Plan{PrepSessions: []domain.PrepSession{ps}}
	}
	cooks := domain.Plan{PrepSessions: []domain.PrepSession{{At: slot(4, domain.Dinner), Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{
		{ID: 1, Location: domain.Fridge, EatAt: in(slot(4, domain.Dinner))}, {ID: 2, Location: domain.Fridge, EatAt: in(slot(5, domain.Dinner))}, {ID: 3, Location: domain.Freezer},
	}}}}
	// cooksEarlier prepares at lunch instead, and puts its frozen Portion at the snack.
	cooksEarlier := domain.Plan{PrepSessions: slices.Clone(cooks.PrepSessions), Kept: []domain.Kept{{Portion: domain.PlannedID(3), At: slot(4, domain.Snack)}}}
	cooksEarlier.PrepSessions[0].At = slot(4, domain.Lunch)
	tests := []struct {
		name  string
		plan  domain.Plan
		stock map[domain.StockKey]domain.Quantity
		want  domain.Forecast
	}{
		{"no Plan: what the kitchen holds now", domain.Plan{}, stock(500, 400, 0),
			domain.Forecast{Stock: stock(500, 400, 0), Lots: lasagne(7, 8, 9)}},
		{"a Prep Session to come uses up stock outside the freezer", sets(4, domain.Dinner, breast, 300), stock(500, 400, 0),
			domain.Forecast{Stock: stock(200, 400, 0), Lots: lasagne(7, 8, 9)}},
		{"then what the freezer holds", sets(4, domain.Dinner, breast, 600), stock(500, 400, 0),
			domain.Forecast{Stock: stock(0, 300, 0), Lots: lasagne(7, 8, 9)}},
		{"and never below nothing", sets(4, domain.Dinner, breast, 1000), stock(500, 400, 0),
			domain.Forecast{Stock: stock(0, 0, 0), Lots: lasagne(7, 8, 9)}},
		{"a Prep Session already past changes nothing", sets(3, domain.Breakfast, breast, 300), stock(500, 400, 0),
			domain.Forecast{Stock: stock(500, 400, 0), Lots: lasagne(7, 8, 9)}},
		{"nor one at the Slot itself", sets(5, domain.Breakfast, breast, 300), stock(500, 400, 0),
			domain.Forecast{Stock: stock(500, 400, 0), Lots: lasagne(7, 8, 9)}},
		{"its Portions not eaten before the Slot are left without one, by PlannedID", cooks, nil,
			domain.Forecast{Lots: append(lasagne(7, 8, 9), chiliIn(domain.Fridge, -2), chiliIn(domain.Freezer, -3))}},
		{"one it puts in a Slot before the Slot is gone", cooksEarlier, nil,
			domain.Forecast{Lots: append(lasagne(7, 8, 9), chiliIn(domain.Fridge, -2))}},
		{"a Portion without a Slot eaten before the Slot is gone", kept(8, slot(4, domain.Lunch)), nil,
			domain.Forecast{Lots: lasagne(7, 9)}},
		{"one eaten from the Slot on is left", kept(8, slot(5, domain.Lunch)), nil,
			domain.Forecast{Lots: lasagne(7, 8, 9)}},
		{"one eaten already is as the kitchen holds it", kept(8, slot(2, domain.Lunch)), nil,
			domain.Forecast{Lots: lasagne(7, 8, 9)}},
		{"a lot all eaten is none", domain.Plan{Kept: []domain.Kept{{Portion: 7, At: slot(3, domain.Lunch)}, {Portion: 8, At: slot(3, domain.Dinner)}, {Portion: 9, At: slot(4, domain.Lunch)}}}, nil,
			domain.Forecast{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.plan.Forecast(tt.stock, lasagne(7, 8, 9), domain.DefaultSlotTimes, now, at)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Forecast() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The kitchen forecast at an upcoming Cycle's first Slot adds its fixed Grocery List, put
// away, once its Delivery is ordered: 400 g of chicken breast, 300 g of it split to the
// freezer, and 200 g of frozen peas. While it is tentative it adds nothing: what's
// missing is what the list buys.
func TestTheKitchenForecastWithAnOrderedDelivery(t *testing.T) {
	lines := []domain.GroceryLine{{Ingredient: breast, Quantity: grams(400)}, {Ingredient: peas, Quantity: grams(200)}}
	splits := []domain.RecipeIngredient{{Ingredient: breast, Quantity: grams(300)}}
	f := domain.Forecast{Stock: stock(100, 0, 50)}
	tests := []struct {
		name  string
		state domain.DeliveryState
		want  domain.Forecast
	}{
		{"tentative: nothing arrives yet", domain.Tentative, domain.Forecast{Stock: stock(100, 0, 50)}},
		{"ordered: the fixed lines, put away", domain.Ordered, domain.Forecast{Stock: stock(200, 300, 250), Arriving: []domain.PutAwayLine{
			{GroceryLine: domain.GroceryLine{Ingredient: breast, Quantity: grams(100)}, Location: domain.Fridge},
			{GroceryLine: domain.GroceryLine{Ingredient: breast, Quantity: grams(300)}, Location: domain.Freezer},
			{GroceryLine: domain.GroceryLine{Ingredient: peas, Quantity: grams(200)}, Location: domain.Freezer},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.WithOrdered(domain.Delivery{State: tt.state}, lines, splits)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WithOrdered() = %+v, want %+v", got, tt.want)
			}
		})
	}
	if want := stock(100, 0, 50); !reflect.DeepEqual(f.Stock, want) {
		t.Errorf("WithOrdered() changed the forecast it was given: %+v, want %+v", f.Stock, want)
	}
}
