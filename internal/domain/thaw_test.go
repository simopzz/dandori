package domain_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// breast is chicken breast, fresh, usually kept in the fridge; peas are usually kept in
// the freezer.
var (
	breast = domain.Ingredient{ID: 1, Name: "chicken breast", Unit: domain.Grams, Usual: domain.Fridge}
	peas   = domain.Ingredient{ID: 2, Name: "frozen peas", Unit: domain.Grams, Usual: domain.Freezer, UseFromFrozen: true}
)

// setsOut is a Prep Session at dinner on `day` setting out `quantity` of `ing` for
// one Portion.
func setsOut(day int, ing domain.Ingredient, quantity float64) domain.PrepSession {
	return domain.PrepSession{At: slot(day, domain.Dinner), Ingredient: &ing, Quantity: grams(quantity), Portions: make([]domain.PlannedPortion, 1)}
}

// stock is what the kitchen holds of chicken breast and peas, by Storage Location.
func stock(breastFridge, breastFreezer, peasFreezer float64) map[domain.StockKey]domain.Quantity {
	return map[domain.StockKey]domain.Quantity{
		{Ingredient: breast.ID, Location: domain.Fridge}:  grams(breastFridge),
		{Ingredient: breast.ID, Location: domain.Freezer}: grams(breastFreezer),
		{Ingredient: peas.ID, Location: domain.Freezer}:   grams(peasFreezer),
	}
}

// A Prep Session uses up stock outside the freezer, and frozen stock of an Ingredient
// usually kept there; what it still needs is thawed from the freezer the evening before,
// as far as the freezer holds it. Prep Sessions use up stock in the order of their Slots.
func TestPrepSessionsUseUpStockAndThawTheRest(t *testing.T) {
	tests := []struct {
		name     string
		sessions []domain.PrepSession
		stock    map[domain.StockKey]domain.Quantity
		want     []domain.Thaw
	}{
		{"stock outside the freezer, nothing thawed", []domain.PrepSession{setsOut(6, breast, 300)}, stock(500, 400, 0), nil},
		{"an Ingredient used from frozen", []domain.PrepSession{setsOut(6, peas, 200)}, stock(0, 0, 500), nil},
		{
			"split stock: what the fridge no longer holds thawed the evening before",
			[]domain.PrepSession{setsOut(6, breast, 300), setsOut(9, breast, 300)}, stock(400, 400, 0),
			[]domain.Thaw{{At: slot(9, domain.Dinner), Ingredient: breast, Quantity: grams(200)}},
		},
		{
			"in the order of their Slots, whatever the order listed",
			[]domain.PrepSession{setsOut(9, breast, 300), setsOut(6, breast, 300)}, stock(400, 400, 0),
			[]domain.Thaw{{At: slot(9, domain.Dinner), Ingredient: breast, Quantity: grams(200)}},
		},
		{
			"no more thawed than the freezer holds",
			[]domain.PrepSession{setsOut(6, breast, 300)}, stock(0, 100, 0),
			[]domain.Thaw{{At: slot(6, domain.Dinner), Ingredient: breast, Quantity: grams(100)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.Plan{PrepSessions: tt.sessions}.Thaws(tt.stock)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Thaws() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A freezer Ingredient that needs thawing is distinct from one used straight from frozen.
func TestFreezerIngredientsCanNeedThawing(t *testing.T) {
	chicken := domain.Ingredient{ID: 3, Name: "whole chicken", Unit: domain.Grams, Usual: domain.Freezer}
	plan := domain.Plan{PrepSessions: []domain.PrepSession{setsOut(6, chicken, 300)}}
	stock := map[domain.StockKey]domain.Quantity{{Ingredient: chicken.ID, Location: domain.Freezer}: grams(500)}
	got := plan.Thaws(stock)
	want := []domain.Thaw{{At: slot(6, domain.Dinner), Ingredient: chicken, Quantity: grams(300)}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Thaws() = %+v, want freezer chicken thawed: %+v", got, want)
	}
}

// Frozen food needs an evening before its Slot still to come, at midnight of the Slot's
// day at the latest: a frozen Portion in the kitchen, freezer stock of an Ingredient not
// usually kept there, and a Portion the Plan freezes, eaten on a later day only.
func TestFrozenFoodNeedsAnEveningLeftToThaw(t *testing.T) {
	frozen := map[int64]bool{9: true}
	kept := func(portion int64, s domain.Slot) domain.Plan {
		return domain.Plan{Kept: []domain.Kept{{Portion: portion, At: s}}}
	}
	freezes := func(prepared, eaten domain.Slot) domain.Plan {
		return domain.Plan{PrepSessions: []domain.PrepSession{{At: prepared, Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{
			{Location: domain.Freezer, EatAt: &eaten},
		}}}}
	}
	tests := []struct {
		name  string
		plan  domain.Plan
		now   string
		stock map[domain.StockKey]domain.Quantity
		want  int
	}{
		{"a frozen Portion with no evening left", kept(9, slot(3, domain.Dinner)), "08:00", nil, 1},
		{"a frozen Portion the next day, tonight still to come", kept(9, slot(4, domain.Lunch)), "21:30", nil, 0},
		{"a Portion not frozen", kept(8, slot(3, domain.Dinner)), "08:00", nil, 0},
		{"freezer stock needed today", domain.Plan{PrepSessions: []domain.PrepSession{setsOut(3, breast, 300)}}, "08:00", stock(0, 400, 0), 1},
		{"freezer stock needed tomorrow", domain.Plan{PrepSessions: []domain.PrepSession{setsOut(4, breast, 300)}}, "08:00", stock(0, 400, 0), 0},
		{"an Ingredient used from frozen today", domain.Plan{PrepSessions: []domain.PrepSession{setsOut(3, peas, 300)}}, "08:00", stock(0, 0, 400), 0},
		{"a Portion frozen and eaten the day it is prepared", freezes(slot(4, domain.Breakfast), slot(4, domain.Dinner)), "08:00", nil, 1},
		{"a Portion frozen and eaten the next day", freezes(slot(4, domain.Dinner), slot(5, domain.Lunch)), "08:00", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := domain.NoEveningToThaw(tt.plan, frozen, tt.stock, at(3, tt.now), at(1, "07:00"))
			if len(vs) != tt.want || slices.ContainsFunc(vs, func(v domain.Violation) bool { return v.Rule != domain.RuleNoEveningToThaw }) {
				t.Errorf("NoEveningToThaw() = %v, want %d of %s", vs, tt.want, domain.RuleNoEveningToThaw)
			}
		})
	}
}

// Before its Delivery arrives, a Prep Session with no evening left to thaw freezer stock
// takes the Ingredient fresh from the Delivery; a frozen Portion still needs the evening.
func TestBeforeItArrivesTheDeliveryBringsFreshWhatThereIsNoEveningToThaw(t *testing.T) {
	now, arrives := at(3, "08:00"), at(3, "18:00")
	cooks := domain.Plan{PrepSessions: []domain.PrepSession{setsOut(3, breast, 300)}}
	if vs := domain.NoEveningToThaw(cooks, nil, stock(0, 400, 0), now, arrives); len(vs) != 0 {
		t.Errorf("freezer stock today, the Delivery to come: NoEveningToThaw() = %v, want none", vs)
	}
	thaws := domain.Plan{Kept: []domain.Kept{{Portion: 9, At: slot(3, domain.Dinner)}}}
	if vs := domain.NoEveningToThaw(thaws, map[int64]bool{9: true}, nil, now, arrives); len(vs) != 1 {
		t.Errorf("a frozen Portion today, the Delivery to come: NoEveningToThaw() = %v, want 1", vs)
	}
}

// An evening's thawed Movements are told as Thawings: a Stock Item's with its Ingredient
// and how much, for its Prep Session's Slot; a Portion's with the Portion, for the Slot
// it fills.
func TestThawedMovementsAreToldAsThawings(t *testing.T) {
	lunch, snack := slot(7, domain.Lunch), slot(7, domain.Snack)
	lasagne := domain.Portion{ID: 7, Name: "lasagne"}
	thawed := []domain.Movement{
		{Key: domain.StockKey{Ingredient: breast.ID, Location: domain.Freezer}, To: domain.Fridge, Change: grams(200), Cause: domain.Thawed, PrepSession: 3, Slot: lunch},
		{Key: domain.StockKey{Location: domain.Freezer}, To: domain.Fridge, Portions: []int64{7}, Change: grams(1), Cause: domain.Thawed, Slot: snack},
	}
	got := domain.ThawingsOf(thawed, []domain.Ingredient{peas, breast}, []domain.Portion{{ID: 6, Name: "chili"}, lasagne})
	want := []domain.Thawing{
		{For: lunch, Ingredient: &breast, Quantity: grams(200)},
		{For: snack, Portion: &lasagne},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ThawingsOf() = %+v, want %+v", got, want)
	}
}
