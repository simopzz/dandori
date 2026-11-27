package domain_test

import (
	"reflect"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// A Plan Proposal from a Slot to the Cycle's last replaces the Plan between them: the
// Prep Sessions before the first Slot stay, their Portions eaten before it stay in their
// Slots, and those eaten from it on or without a Slot are the Plan Proposal's to put in
// Slots again, as are the Portions without a Slot the kitchen holds that the Plan didn't
// put in a Slot before the first. Here the first Slot is Thursday's dinner, once
// Thursday's lunch has passed, and the last Sunday's dinner.
func TestWhatAPlanProposalReplacesAndKeeps(t *testing.T) {
	first, last := slot(5, domain.Dinner), slot(8, domain.Dinner)
	// cooks is a Prep Session of chili at s, its Portions eaten at each of at, nil kept
	// for later, with the ids 1, 2 and on.
	cooks := func(s domain.Slot, at ...*domain.Slot) domain.PrepSession {
		ps := domain.PrepSession{At: s, Recipe: &chili, Size: 1}
		for i, e := range at {
			ps.Portions = append(ps.Portions, domain.PlannedPortion{ID: int64(i + 1), Location: domain.Fridge, EatAt: e})
		}
		return ps
	}
	plan := func(kept []domain.Kept, sessions ...domain.PrepSession) domain.Plan {
		return domain.Plan{PrepSessions: sessions, Kept: kept}
	}
	lasagne := func(ids ...int64) domain.PortionLot {
		return domain.PortionLot{Name: "lasagne", Location: domain.Freezer, IDs: ids}
	}
	// planned is the Portions of chili prepared and not yet in the kitchen, ids as the
	// Portions without a Slot name them.
	planned := func(ids ...int64) domain.PortionLot {
		return domain.PortionLot{Name: "Chili", Location: domain.Fridge, Nutrition: chili.NutritionPerPortion().Nutrition, IDs: ids}
	}
	tests := []struct {
		name string
		plan domain.Plan
		lots []domain.PortionLot
		kept domain.Plan
		free []domain.PortionLot
	}{
		{"today's Slot passed: its Prep Session stays, and its Portion eaten from the first Slot on is free",
			plan(nil, cooks(slot(5, domain.Lunch), in(slot(5, domain.Lunch)), in(slot(5, domain.Dinner)))), nil,
			plan(nil, cooks(slot(5, domain.Lunch), in(slot(5, domain.Lunch)), nil)), []domain.PortionLot{planned(-2)}},
		{"a Prep Session at the first Slot is replaced, with its Portions",
			plan(nil, cooks(first, in(first), in(slot(6, domain.Lunch)))), nil,
			domain.Plan{}, nil},
		{"a Portion eaten at a Slot already past stays in it",
			plan(nil, cooks(slot(4, domain.Lunch), in(slot(4, domain.Lunch)), in(slot(4, domain.Dinner)))), nil,
			plan(nil, cooks(slot(4, domain.Lunch), in(slot(4, domain.Lunch)), in(slot(4, domain.Dinner)))), nil},
		{"a Portion carried over without a Slot is free, as is one the Plan made for after the Cycle",
			plan(nil, cooks(slot(4, domain.Dinner), in(slot(4, domain.Dinner)), nil)), []domain.PortionLot{lasagne(7, 8)},
			plan(nil, cooks(slot(4, domain.Dinner), in(slot(4, domain.Dinner)), nil)), []domain.PortionLot{lasagne(7, 8), planned(-2)}},
		{"a Portion the Plan put in a Slot before the first stays there; one put from the first on is free again",
			plan([]domain.Kept{{Portion: 7, At: slot(4, domain.Dinner)}, {Portion: 8, At: slot(7, domain.Lunch)}}), []domain.PortionLot{lasagne(7, 8, 9)},
			plan([]domain.Kept{{Portion: 7, At: slot(4, domain.Dinner)}}), []domain.PortionLot{lasagne(8, 9)}},
		{"a Portion the kitchen forecast holds already by PlannedID is free once",
			plan(nil, cooks(slot(4, domain.Dinner), in(slot(4, domain.Dinner)), nil)), []domain.PortionLot{planned(-2)},
			plan(nil, cooks(slot(4, domain.Dinner), in(slot(4, domain.Dinner)), nil)), []domain.PortionLot{planned(-2)}},
		{"after the last Slot nothing is replaced, nor free",
			plan([]domain.Kept{{Portion: 9, At: slot(9, domain.Dinner)}}, cooks(slot(9, domain.Lunch), in(slot(9, domain.Lunch)), nil)), []domain.PortionLot{lasagne(9)},
			plan([]domain.Kept{{Portion: 9, At: slot(9, domain.Dinner)}}, cooks(slot(9, domain.Lunch), in(slot(9, domain.Lunch)), nil)), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, free := tt.plan.Replace(first, last, tt.lots)
			if !reflect.DeepEqual(kept, tt.kept) {
				t.Errorf("Replace() kept %+v,\nwant %+v", kept, tt.kept)
			}
			if !reflect.DeepEqual(free, tt.free) {
				t.Errorf("Replace() freed %+v,\nwant %+v", free, tt.free)
			}
		})
	}
}

// Approving a Plan Proposal makes the Plan what Replace keeps of it with the Plan
// Proposal's Plan: a kept Prep Session's Portion it puts in a Slot again is eaten there.
func TestApprovingAPlanProposalReplacesThePlanFromItsFirstSlot(t *testing.T) {
	bowl := domain.PrepSession{At: slot(5, domain.Lunch), Recipe: &chili, Size: 1, Portions: []domain.PlannedPortion{
		{ID: 1, Location: domain.Fridge, EatAt: in(slot(5, domain.Lunch))},
		{ID: 2, Location: domain.Freezer, EatAt: in(slot(6, domain.Lunch))},
	}}
	soup := domain.PrepSession{At: slot(5, domain.Dinner), Ingredient: &yogurt, Quantity: grams(150), Portions: []domain.PlannedPortion{
		{ID: 3, Location: domain.Fridge, EatAt: in(slot(5, domain.Dinner))},
	}}
	plan := domain.Plan{PrepSessions: []domain.PrepSession{bowl, soup}, Kept: []domain.Kept{{Portion: 7, At: slot(6, domain.Dinner)}}}
	proposal := domain.Plan{
		PrepSessions: []domain.PrepSession{{At: slot(5, domain.Dinner), Ingredient: &yogurt, Quantity: grams(200), Portions: []domain.PlannedPortion{
			{Location: domain.Fridge, EatAt: in(slot(5, domain.Dinner))},
		}}},
		Kept: []domain.Kept{{Portion: -2, At: slot(6, domain.Dinner)}, {Portion: 7, At: slot(6, domain.Lunch)}},
	}

	got := plan.With(slot(5, domain.Dinner), proposal)
	moved := bowl
	moved.Portions = []domain.PlannedPortion{bowl.Portions[0], {ID: 2, Location: domain.Freezer, EatAt: in(slot(6, domain.Dinner))}}
	want := domain.Plan{
		PrepSessions: []domain.PrepSession{moved, proposal.PrepSessions[0]},
		Kept:         []domain.Kept{{Portion: 7, At: slot(6, domain.Lunch)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("With() = %+v,\nwant %+v", got, want)
	}
	if plan.PrepSessions[0].Portions[1].EatAt.Kind != domain.Lunch {
		t.Error("approving a Plan Proposal changed the Plan it replaced")
	}
}
