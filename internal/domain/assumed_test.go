package domain_test

import (
	"reflect"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// What Dandori assumed is grouped by day, oldest first, a row per Slot in the day's
// order, and the evening's thaws in a row of their own after its dinner, under the day
// before the Slots they are for. A Movement with no Slot, a Delivery's or one reported,
// isn't listed.
func TestAssumedMovementsAreGroupedByDayAndSlot(t *testing.T) {
	fridge := domain.StockKey{Location: domain.Fridge}
	usedUp := domain.Movement{At: at(5, "13:00"), Key: domain.StockKey{Ingredient: plainRice.ID, Location: domain.Pantry}, Change: -375, Cause: domain.UsedUp, PrepSession: 1, Slot: slot(5, domain.Lunch)}
	prepared := domain.Movement{At: at(5, "13:00"), Key: fridge, Portions: []int64{7, 8}, Cause: domain.Prepared, PrepSession: 1, Slot: slot(5, domain.Lunch)}
	eatenAtLunch := domain.Movement{At: at(5, "13:00"), Key: fridge, Portions: []int64{7}, Cause: domain.PortionEaten, Slot: slot(5, domain.Lunch)}
	breakfast := domain.Movement{At: at(5, "08:00"), Key: fridge, Portions: []int64{6}, Cause: domain.PortionEaten, Slot: slot(5, domain.Breakfast)}
	thawed := domain.Movement{At: at(5, "20:00"), Key: domain.StockKey{Location: domain.Freezer}, To: domain.Fridge, Portions: []int64{3}, Cause: domain.Thawed, Slot: slot(6, domain.Snack)}
	dinner := domain.Movement{At: at(5, "20:00"), Key: fridge, Portions: []int64{8}, Cause: domain.PortionEaten, Slot: slot(5, domain.Dinner)}
	nextLunch := domain.Movement{At: at(6, "13:00"), Key: fridge, Portions: []int64{9}, Cause: domain.PortionEaten, Slot: slot(6, domain.Lunch)}
	delivered := domain.Movement{At: at(5, "19:00"), Key: domain.StockKey{Ingredient: plainRice.ID, Location: domain.Pantry}, Change: 500, Cause: domain.Delivered}

	got := domain.AssumedByDay([]domain.Movement{breakfast, usedUp, prepared, eatenAtLunch, delivered, dinner, thawed, nextLunch})
	want := []domain.AssumedDay{
		{On: day(5), Rows: []domain.AssumedRow{
			{Slot: slot(5, domain.Breakfast), Movements: []domain.Movement{breakfast}},
			{Slot: slot(5, domain.Lunch), Movements: []domain.Movement{usedUp, prepared, eatenAtLunch}},
			{Slot: slot(5, domain.Dinner), Movements: []domain.Movement{dinner}},
			{Slot: slot(5, domain.Dinner), Thaws: true, Movements: []domain.Movement{thawed}},
		}},
		{On: day(6), Rows: []domain.AssumedRow{{Slot: slot(6, domain.Lunch), Movements: []domain.Movement{nextLunch}}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("grouped as %+v, want %+v", got, want)
	}
	if got := domain.AssumedByDay(nil); len(got) != 0 {
		t.Errorf("nothing assumed is grouped as %+v", got)
	}
}

// A Slot's row is summed up per Prep Session, how many Portions it prepared and how many
// of those the Slot ate, then each Portion it ate from earlier on its own.
func TestAnAssumedSlotIsSummedUpPerPrepSession(t *testing.T) {
	fridge := domain.StockKey{Location: domain.Fridge}
	row := domain.AssumedRow{Slot: slot(5, domain.Dinner), Movements: []domain.Movement{
		{Key: domain.StockKey{Ingredient: plainRice.ID, Location: domain.Pantry}, Change: -375, Cause: domain.UsedUp, PrepSession: 1},
		{Key: fridge, Portions: []int64{7, 8}, Cause: domain.Prepared, PrepSession: 1},
		{Key: domain.StockKey{Location: domain.Freezer}, Portions: []int64{9}, Cause: domain.Prepared, PrepSession: 1},
		{Key: fridge, Portions: []int64{10}, Cause: domain.Prepared, PrepSession: 2},
		{Key: fridge, Portions: []int64{7}, Cause: domain.PortionEaten},
		{Key: fridge, Portions: []int64{3}, Cause: domain.PortionEaten},
	}}
	want := []domain.AssumedPart{{Portion: 7, Prepared: 3, Eaten: 1}, {Portion: 10, Prepared: 1}, {Portion: 3, Eaten: 1}}
	if got := row.Summary(); !reflect.DeepEqual(got, want) {
		t.Errorf("summed up as %+v, want %+v", got, want)
	}
}
