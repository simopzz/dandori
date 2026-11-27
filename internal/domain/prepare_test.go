package domain_test

import (
	"reflect"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// A Prep Session of a Recipe starts its Recipe's total minutes before its Slot, and is
// being prepared from then until its Slot's time; one without a Recipe has no start.
func TestAPrepSessionIsPreparedFromItsStartToItsSlot(t *testing.T) {
	bowl := riceBowl
	bowl.TotalMinutes = 25
	cooked := domain.PrepSession{At: slot(6, domain.Lunch), Recipe: &bowl, Size: 1}
	if start, ok := cooked.Start(domain.DefaultSlotTimes); !ok || !start.Equal(at(6, "12:35")) {
		t.Errorf("a 25-minute Recipe for a 1 pm lunch starts at %v (%v), want 12:35", start, ok)
	}
	for _, tt := range []struct {
		at   string
		want bool
	}{{"12:34", false}, {"12:35", true}, {"12:59", true}, {"13:00", false}} {
		if got := cooked.Preparing(at(6, tt.at), domain.DefaultSlotTimes); got != tt.want {
			t.Errorf("preparing at %s = %v, want %v", tt.at, got, tt.want)
		}
	}
	setOut := session(1, slot(6, domain.Lunch), plainYogurt, 150)
	if _, ok := setOut.Start(domain.DefaultSlotTimes); ok {
		t.Error("a Prep Session without a Recipe has a start")
	}
	if setOut.Preparing(at(6, "12:59"), domain.DefaultSlotTimes) {
		t.Error("a Prep Session without a Recipe is being prepared")
	}
}

// Where each of a Slot's Prep Sessions would take its Ingredients from: in use-up order,
// each after the one before took its share, none for one held nowhere, and no entry for
// a Staple.
func TestWhereAPrepSessionTakesItsIngredientsFrom(t *testing.T) {
	lunch := slot(6, domain.Lunch)
	salt := domain.Ingredient{ID: 9, Name: "salt", Unit: domain.Grams, Staple: true}
	l := domain.NewLedger([]domain.Movement{holds(plainRice, domain.Fridge, 100), holds(plainRice, domain.Pantry, 100)})
	got := l.TakesFrom(at(6, "12:00"), []domain.PrepSession{
		session(1, lunch, plainRice, 150, planned(11, domain.Fridge, nil)),
		session(2, lunch, plainRice, 50, planned(12, domain.Fridge, nil)),
		session(3, lunch, plainYogurt, 50, planned(13, domain.Fridge, nil)),
		session(4, lunch, salt, 5, planned(14, domain.Fridge, nil)),
	})
	want := []map[int64][]domain.StorageLocation{
		{plainRice.ID: {domain.Pantry, domain.Fridge}},
		{plainRice.ID: {domain.Fridge}},
		{plainYogurt.ID: {}},
		{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("takes from %v, want %v", got, want)
	}
}
