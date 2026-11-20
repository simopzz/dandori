package domain_test

import (
	"reflect"
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
	// sets is a Prep Session at kind on day setting out quantity of ing, eaten there.
	sets := func(day int, kind domain.SlotKind, ing domain.Ingredient, quantity float64) domain.Plan {
		ps := setsOut(day, ing, quantity)
		ps.At, ps.Portions = slot(day, kind), []domain.PlannedPortion{eats(slot(day, kind))}
		return domain.Plan{PrepSessions: []domain.PrepSession{ps}}
	}
	cooks := domain.Plan{PrepSessions: []domain.PrepSession{{At: slot(4, domain.Dinner), Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{
		eats(slot(4, domain.Dinner)), eats(slot(5, domain.Dinner)), {Location: domain.Freezer},
	}}}}
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
		{"its Portions not eaten before the Slot are left without one", cooks, nil,
			domain.Forecast{Lots: lasagne(7, 8, 9), Made: []domain.MadeForAfter{
				{Name: "Chili", Location: domain.Fridge, Portions: 1}, {Name: "Chili", Location: domain.Freezer, Portions: 1},
			}}},
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
