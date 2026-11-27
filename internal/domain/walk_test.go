package domain_test

import (
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// key is the Stock Item of `ing` in `location`.
func key(ing domain.Ingredient, location domain.StorageLocation) domain.StockKey {
	return domain.StockKey{Ingredient: ing.ID, Location: location}
}

// holds is a Count on the 5th that `ing` in `location` holds `quantity`.
func holds(ing domain.Ingredient, location domain.StorageLocation, quantity float64) domain.Movement {
	return domain.Movement{At: at(5, "10:00"), Key: key(ing, location), Change: grams(quantity), Cause: domain.Counted, Stated: grams(quantity)}
}

// session is the Prep Session `id` at `s` setting out `quantity` of `ing` for each
// of `portions`.
func session(id int64, s domain.Slot, ing domain.Ingredient, quantity float64, portions ...domain.PlannedPortion) domain.PrepSession {
	return domain.PrepSession{ID: id, At: s, Ingredient: &ing, Quantity: grams(quantity), Portions: portions}
}

// planned is the Planned Portion `id` kept in `location`, eaten at `eat` or without a
// Slot when nil.
func planned(id int64, location domain.StorageLocation, eat *domain.Slot) domain.PlannedPortion {
	return domain.PlannedPortion{ID: id, Location: location, EatAt: eat}
}

// ptr is a pointer to `v`.
func ptr[T any](v T) *T { return &v }

// usedUp is what the Prep Session `ps` at `s` used up of `k`, `quantity`, at `when`.
func usedUp(when time.Time, k domain.StockKey, quantity float64, ps int64, s domain.Slot) domain.Movement {
	return domain.Movement{At: when, Key: k, Change: -grams(quantity), Cause: domain.UsedUp, PrepSession: ps, Slot: s}
}

// prepared is the Portions the Prep Session `ps` at `s` prepared in `location` at `when`
// from its Planned Portions `planned`, listed by their PlannedIDs.
func prepared(when time.Time, location domain.StorageLocation, ps int64, s domain.Slot, planned ...int64) domain.Movement {
	ids := make([]int64, len(planned))
	for i, id := range planned {
		ids[i] = domain.PlannedID(id)
	}
	return domain.Movement{At: when, Key: domain.StockKey{Location: location}, Portions: ids, Change: grams(float64(len(ids))), Cause: domain.Prepared, PrepSession: ps, Slot: s}
}

// ate is the Portion `portion` filling `s` eaten from `location` at `when`.
func ate(when time.Time, location domain.StorageLocation, portion int64, s domain.Slot) domain.Movement {
	return domain.Movement{At: when, Key: domain.StockKey{Location: location}, Portions: []int64{portion}, Change: -grams(1), Cause: domain.PortionEaten, Slot: s}
}

// A Prep Session uses up each Ingredient once: the Stock Items outside the freezer, the
// pantry before the fridge, and frozen ones only for an Ingredient usually kept in the
// freezer, as far as they go, from what the Prep Sessions before it left. A Staple
// is skipped.
func TestAPrepSessionUsesUpItsIngredients(t *testing.T) {
	lunch, dinner := slot(6, domain.Lunch), slot(6, domain.Dinner)
	scaled := domain.PrepSession{ID: 1, At: lunch, Recipe: &riceBowl, Size: 1.25,
		Portions: []domain.PlannedPortion{planned(11, domain.Fridge, nil), planned(12, domain.Fridge, nil), planned(13, domain.Freezer, nil)}}
	tests := []struct {
		name     string
		ledger   []domain.Movement
		sessions []domain.PrepSession
		want     []domain.Movement
	}{
		{
			"the pantry before the fridge",
			[]domain.Movement{holds(plainRice, domain.Fridge, 300), holds(plainRice, domain.Pantry, 100)},
			[]domain.PrepSession{session(1, lunch, plainRice, 300, planned(11, domain.Fridge, nil))},
			[]domain.Movement{usedUp(at(6, "13:00"), key(plainRice, domain.Pantry), 100, 1, lunch), usedUp(at(6, "13:00"), key(plainRice, domain.Fridge), 200, 1, lunch)},
		},
		{
			"a Recipe scaled to its Portions and size",
			[]domain.Movement{holds(plainRice, domain.Pantry, 800)},
			[]domain.PrepSession{scaled},
			[]domain.Movement{usedUp(at(6, "13:00"), key(plainRice, domain.Pantry), 375, 1, lunch)},
		},
		{
			"frozen stock of an Ingredient kept in the fridge is left, and short stock used up as far as it goes",
			[]domain.Movement{holds(breast, domain.Fridge, 200), holds(breast, domain.Freezer, 400)},
			[]domain.PrepSession{session(1, lunch, breast, 300, planned(11, domain.Fridge, nil))},
			[]domain.Movement{usedUp(at(6, "13:00"), key(breast, domain.Fridge), 200, 1, lunch)},
		},
		{
			"an Ingredient usually kept in the freezer from frozen",
			[]domain.Movement{holds(peas, domain.Freezer, 500)},
			[]domain.PrepSession{session(1, lunch, peas, 200, planned(11, domain.Fridge, nil))},
			[]domain.Movement{usedUp(at(6, "13:00"), key(peas, domain.Freezer), 200, 1, lunch)},
		},
		{
			"none held, nothing used up",
			nil,
			[]domain.PrepSession{session(1, lunch, breast, 300, planned(11, domain.Fridge, nil))},
			nil,
		},
		{
			"a Staple skipped",
			[]domain.Movement{holds(salt, domain.Pantry, 50)},
			[]domain.PrepSession{session(1, lunch, salt, 10, planned(11, domain.Fridge, nil))},
			nil,
		},
		{
			"several Prep Sessions, each from what those before it left",
			[]domain.Movement{holds(plainRice, domain.Pantry, 500)},
			[]domain.PrepSession{
				session(2, dinner, plainRice, 300, planned(21, domain.Fridge, nil)),
				session(1, lunch, plainRice, 300, planned(11, domain.Fridge, nil)),
			},
			[]domain.Movement{usedUp(at(6, "13:00"), key(plainRice, domain.Pantry), 300, 1, lunch), usedUp(at(6, "20:00"), key(plainRice, domain.Pantry), 200, 2, dinner)},
		},
		{
			"a Stock Item used up exactly holds nothing for the next",
			[]domain.Movement{holds(plainRice, domain.Pantry, 300)},
			[]domain.PrepSession{
				session(1, lunch, plainRice, 300, planned(11, domain.Fridge, nil)),
				session(2, dinner, plainRice, 100, planned(21, domain.Fridge, nil)),
			},
			[]domain.Movement{usedUp(at(6, "13:00"), key(plainRice, domain.Pantry), 300, 1, lunch)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := domain.Plan{PrepSessions: tt.sessions}
			walked, _ := domain.NewLedger(tt.ledger).UpToNow(plan, nil, nil, domain.DefaultSlotTimes, at(6, "12:00"), at(6, "21:00"))
			var got []domain.Movement
			for _, m := range walked {
				if m.Cause == domain.UsedUp {
					got = append(got, m)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("used up\n%+v, want\n%+v", got, tt.want)
			}
		})
	}
}

// Lunch moved from 13:00 to 13:30 on the 6th after it happened: a walk from then over the
// 7th prepares the 6th's lunch no second time, and the 7th's at 13:30.
func TestAWalkCrossesTheDayTimesOfDayChange(t *testing.T) {
	sixth, seventh := slot(6, domain.Lunch), slot(7, domain.Lunch)
	later := domain.SlotTimes{domain.Breakfast: 8 * 60, domain.Lunch: 13*60 + 30, domain.Snack: 17 * 60, domain.Dinner: 20 * 60}
	change := domain.TimesChange{Was: domain.DefaultSlotTimes, Now: later, From: seventh.On}
	plan := domain.Plan{PrepSessions: []domain.PrepSession{
		session(1, sixth, plainRice, 300, planned(11, domain.Fridge, nil)),
		session(2, seventh, plainRice, 100, planned(21, domain.Fridge, nil)),
	}}
	got, _ := domain.NewLedger([]domain.Movement{holds(plainRice, domain.Pantry, 800)}).
		UpToNow(plan, nil, nil, change, at(6, "13:05"), at(7, "14:00"))
	want := []domain.Movement{
		usedUp(at(7, "13:30"), key(plainRice, domain.Pantry), 100, 2, seventh),
		prepared(at(7, "13:30"), domain.Fridge, 2, seventh, 21),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("walked\n%+v, want\n%+v", got, want)
	}
}

// A Prep Session's Portions are prepared in their Storage Locations with its name and
// values per Portion, scaled to their size, and the one filling its own Slot is eaten.
func TestAPrepSessionPreparesItsPortions(t *testing.T) {
	lunch := slot(6, domain.Lunch)
	plan := domain.Plan{PrepSessions: []domain.PrepSession{{ID: 2, At: lunch, Recipe: &riceBowl, Size: 1.25, Portions: []domain.PlannedPortion{
		planned(21, domain.Fridge, &lunch), planned(22, domain.Fridge, ptr(slot(8, domain.Lunch))), planned(23, domain.Freezer, nil),
	}}}}
	got, made := domain.NewLedger([]domain.Movement{holds(plainRice, domain.Pantry, 800)}).
		UpToNow(plan, nil, nil, domain.DefaultSlotTimes, at(6, "12:00"), at(6, "13:00"))
	noon := at(6, "13:00")
	want := []domain.Movement{
		usedUp(noon, key(plainRice, domain.Pantry), 375, 2, lunch),
		prepared(noon, domain.Fridge, 2, lunch, 21, 22),
		prepared(noon, domain.Freezer, 2, lunch, 23),
		ate(noon, domain.Fridge, domain.PlannedID(21), lunch),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the walk\n%+v, want\n%+v", got, want)
	}
	// 125 g of rice a Portion: 437.5 kcal, 8.75 g protein, 97.5 g carbs, 1.25 g fat.
	bowl := nutrition(438, 9, 98, 1, domain.Unknown, domain.Unknown)
	wantMade := []domain.PreparedPortion{{Planned: 21, Name: "Rice bowl", Nutrition: bowl}, {Planned: 22, Name: "Rice bowl", Nutrition: bowl}, {Planned: 23, Name: "Rice bowl", Nutrition: bowl}}
	if !reflect.DeepEqual(made, wantMade) {
		t.Errorf("prepared %+v, want %+v", made, wantMade)
	}
}

// The walk over a window: Slots and Delivery arrivals in time order, at one moment
// arrival, used up, prepared, eaten, each dated at its time or the latest Movement's.
func TestTheWalkRecordsWhatThePlanAssumed(t *testing.T) {
	breakfast, lunch, snack := slot(6, domain.Breakfast), slot(6, domain.Lunch), slot(6, domain.Snack)
	lasagne := domain.Movement{At: at(5, "10:00"), Key: domain.StockKey{Location: domain.Freezer}, Portions: []int64{7}, Change: grams(1), Cause: domain.Added}
	yogurtAtBreakfast := domain.Plan{PrepSessions: []domain.PrepSession{session(4, breakfast, plainYogurt, 150, planned(41, domain.Fridge, &breakfast))}}
	delivery := func(when time.Time) []domain.Arrival {
		return []domain.Arrival{{At: when, Put: []domain.PutAwayLine{{GroceryLine: domain.GroceryLine{Ingredient: plainYogurt, Quantity: grams(150)}, Location: domain.Fridge}}}}
	}
	delivered := func(when time.Time) domain.Movement {
		return domain.Movement{At: when, Key: key(plainYogurt, domain.Fridge), Change: grams(150), Cause: domain.Delivered}
	}
	riceAt := func(id int64, s domain.Slot) domain.PrepSession {
		return session(id, s, plainRice, 100, planned(id*10, domain.Fridge, &s))
	}
	cooked := func(when time.Time, id int64, s domain.Slot) []domain.Movement {
		return []domain.Movement{
			usedUp(when, key(plainRice, domain.Pantry), 100, id, s),
			prepared(when, domain.Fridge, id, s, id*10),
			ate(when, domain.Fridge, domain.PlannedID(id*10), s),
		}
	}
	tests := []struct {
		name     string
		ledger   []domain.Movement
		plan     domain.Plan
		arrivals []domain.Arrival
		from, to time.Time
		want     []domain.Movement
	}{
		{
			name:   "a kept Portion eaten from where the kitchen holds it",
			ledger: []domain.Movement{lasagne, {At: at(5, "12:00"), Key: domain.StockKey{Location: domain.Freezer}, To: domain.Fridge, Portions: []int64{7}, Change: grams(1), Cause: domain.Moved}},
			plan:   domain.Plan{Kept: []domain.Kept{{Portion: 7, At: snack}}},
			from:   at(6, "16:00"), to: at(6, "17:00"),
			want: []domain.Movement{ate(at(6, "17:00"), domain.Fridge, 7, snack)},
		},
		{
			name:   "a Portion the kitchen doesn't hold is not eaten",
			ledger: []domain.Movement{lasagne, {At: at(5, "12:00"), Key: domain.StockKey{Location: domain.Freezer}, Portions: []int64{7}, Change: -grams(1), Cause: domain.Counted, Believed: grams(1)}},
			plan:   domain.Plan{Kept: []domain.Kept{{Portion: 7, At: snack}}},
			from:   at(6, "16:00"), to: at(6, "17:00"),
		},
		{
			name: "a Delivery arriving between two Slots, put away before the next",
			plan: yogurtAtBreakfast, arrivals: delivery(at(5, "20:00")),
			from: at(5, "19:00"), to: at(6, "09:00"),
			want: []domain.Movement{
				delivered(at(5, "20:00")),
				usedUp(at(6, "08:00"), key(plainYogurt, domain.Fridge), 150, 4, breakfast),
				prepared(at(6, "08:00"), domain.Fridge, 4, breakfast, 41),
				ate(at(6, "08:00"), domain.Fridge, domain.PlannedID(41), breakfast),
			},
		},
		{
			name: "at one moment: arrival, used up, prepared, eaten",
			plan: yogurtAtBreakfast, arrivals: delivery(at(6, "08:00")),
			from: at(6, "07:00"), to: at(6, "08:00"),
			want: []domain.Movement{
				delivered(at(6, "08:00")),
				usedUp(at(6, "08:00"), key(plainYogurt, domain.Fridge), 150, 4, breakfast),
				prepared(at(6, "08:00"), domain.Fridge, 4, breakfast, 41),
				ate(at(6, "08:00"), domain.Fridge, domain.PlannedID(41), breakfast),
			},
		},
		{
			name:   "dated at the latest Movement when that is later",
			ledger: []domain.Movement{{At: at(6, "13:30"), Key: key(plainRice, domain.Pantry), Change: grams(400), Cause: domain.Counted, Stated: grams(400)}},
			plan:   domain.Plan{PrepSessions: []domain.PrepSession{riceAt(1, lunch)}},
			from:   at(6, "12:00"), to: at(6, "13:05"),
			want: cooked(at(6, "13:30"), 1, lunch),
		},
		{
			name:   "an empty window",
			ledger: []domain.Movement{holds(plainRice, domain.Pantry, 800)},
			plan:   domain.Plan{PrepSessions: []domain.PrepSession{riceAt(1, lunch)}},
			from:   at(6, "13:00"), to: at(6, "13:00"),
		},
		{
			name:   "a window crossing days, with a day between Cycles",
			ledger: []domain.Movement{holds(plainRice, domain.Pantry, 800)},
			plan: domain.Plan{PrepSessions: []domain.PrepSession{
				riceAt(3, slot(8, domain.Dinner)), riceAt(1, lunch), riceAt(2, slot(6, domain.Dinner)),
			}},
			from: at(6, "14:00"), to: at(8, "21:00"),
			want: slices.Concat(cooked(at(6, "20:00"), 2, slot(6, domain.Dinner)), cooked(at(8, "20:00"), 3, slot(8, domain.Dinner))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := domain.NewLedger(tt.ledger).UpToNow(tt.plan, nil, tt.arrivals, domain.DefaultSlotTimes, tt.from, tt.to)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the walk\n%+v, want\n%+v", got, tt.want)
			}
		})
	}
}

// The Portion filling a Slot is the Planned Portion eaten there once prepared, or a
// Portion without a Slot put there: one the kitchen held, or a Planned Portion of an
// earlier Prep Session, of this Plan or another Cycle's, once prepared.
func TestThePortionFillingASlot(t *testing.T) {
	plan := domain.Plan{
		PrepSessions: []domain.PrepSession{{ID: 5, At: slot(6, domain.Dinner), Ingredient: &plainRice, Quantity: grams(100), Portions: []domain.PlannedPortion{
			planned(51, domain.Fridge, ptr(slot(6, domain.Dinner))), planned(52, domain.Fridge, ptr(slot(8, domain.Lunch))), planned(53, domain.Freezer, nil),
		}}},
		Kept: []domain.Kept{
			{Portion: 7, At: slot(7, domain.Lunch)},
			{Portion: domain.PlannedID(61), At: slot(7, domain.Dinner)}, // of another Cycle's Prep Session
		},
	}
	madeFrom := map[int64]int64{51: 101, 61: 102}
	tests := []struct {
		name string
		at   domain.Slot
		want int64
		ok   bool
	}{
		{"a Planned Portion of the Plan, prepared", slot(6, domain.Dinner), 101, true},
		{"a Planned Portion not prepared yet", slot(8, domain.Lunch), 0, false},
		{"a kept Portion", slot(7, domain.Lunch), 7, true},
		{"a kept Planned Portion of another Cycle's Prep Session", slot(7, domain.Dinner), 102, true},
		{"a Slot nothing fills", slot(7, domain.Breakfast), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := plan.Filling(tt.at, madeFrom)
			if got != tt.want || ok != tt.ok {
				t.Errorf("Filling(%s) = %d, %v, want %d, %v", tt.at, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// The Slot each Portion fills is the one Filling finds it in: a Planned Portion once
// prepared, a kept Portion, and the earliest for one two Plans put in two Slots. One not
// prepared yet fills none.
func TestTheSlotEachPortionFills(t *testing.T) {
	plan := domain.Plan{
		PrepSessions: []domain.PrepSession{{ID: 5, At: slot(6, domain.Dinner), Ingredient: &plainRice, Quantity: grams(100), Portions: []domain.PlannedPortion{
			planned(51, domain.Fridge, ptr(slot(6, domain.Dinner))), planned(52, domain.Fridge, ptr(slot(8, domain.Lunch))), planned(53, domain.Freezer, nil),
		}}},
		Kept: []domain.Kept{
			{Portion: 7, At: slot(8, domain.Dinner)},
			{Portion: 7, At: slot(7, domain.Lunch)}, // the upcoming Cycle's Plan, placing it again
		},
	}
	got := plan.Filled(map[int64]int64{51: 101})
	want := map[int64]domain.Slot{101: slot(6, domain.Dinner), 7: slot(7, domain.Lunch)}
	if !maps.EqualFunc(got, want, domain.Slot.Equal) {
		t.Errorf("Filled() = %v, want %v", got, want)
	}
}

// A Slot passes in a window from just after its start to its end, whatever days it spans.
func TestASlotPassesBetween(t *testing.T) {
	tests := []struct {
		name     string
		from, to time.Time
		want     bool
	}{
		{"none between two Slots", at(6, "08:00"), at(6, "12:59"), false},
		{"one at the window's end", at(6, "12:00"), at(6, "13:00"), true},
		{"none at the window's start", at(6, "13:00"), at(6, "14:00"), false},
		{"overnight, none", at(6, "20:30"), at(7, "07:59"), false},
		{"days later", at(6, "20:30"), at(9, "07:00"), true},
		{"from read in another zone, the Slots in the clock's", at(6, "12:00").UTC(), at(6, "13:00"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.PassBetween(domain.DefaultSlotTimes, tt.from, tt.to); got != tt.want {
				t.Errorf("PassBetween(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// thawed is what the evening at `when` thawed of `k` into the fridge, `quantity`, for the
// Prep Session `ps` at `s`.
func thawed(when time.Time, k domain.StockKey, quantity float64, ps int64, s domain.Slot) domain.Movement {
	return domain.Movement{At: when, Key: k, To: domain.Fridge, Change: grams(quantity), Cause: domain.Thawed, PrepSession: ps, Slot: s}
}

// The evening before a day of the Plan, after that dinner's own Movements, thaws into the
// fridge what the day's Prep Sessions need beyond what the kitchen holds outside the
// freezer, counting an ordered Delivery due before the day's first Slot as held, and
// every frozen Portion filling one of its Slots. An Ingredient used from frozen is
// not thawed.
func TestTheEveningBeforeThaws(t *testing.T) {
	tonight, dinner, lunch := at(6, "20:00"), slot(6, domain.Dinner), slot(7, domain.Lunch)
	lasagne := domain.Movement{At: at(5, "10:00"), Key: domain.StockKey{Location: domain.Freezer}, Portions: []int64{7}, Change: grams(1), Cause: domain.Added}
	chili := domain.Movement{At: at(5, "10:00"), Key: domain.StockKey{Location: domain.Fridge}, Portions: []int64{8}, Change: grams(1), Cause: domain.Added}
	cooks := func(ing domain.Ingredient) domain.Plan {
		return domain.Plan{PrepSessions: []domain.PrepSession{session(3, lunch, ing, 300, planned(31, domain.Fridge, &lunch))}}
	}
	delivery := func(ordered bool) []domain.Arrival {
		return []domain.Arrival{{At: at(7, "10:00"), Ordered: ordered, Put: []domain.PutAwayLine{
			{GroceryLine: domain.GroceryLine{Ingredient: breast, Quantity: grams(200)}, Location: domain.Fridge},
			{GroceryLine: domain.GroceryLine{Ingredient: breast, Quantity: grams(100)}, Location: domain.Freezer},
		}}}
	}
	tests := []struct {
		name     string
		ledger   []domain.Movement
		plan     domain.Plan
		arrivals []domain.Arrival
		want     []domain.Movement
	}{
		{
			name:   "freezer stock the next day's Prep Session needs",
			ledger: []domain.Movement{holds(breast, domain.Fridge, 100), holds(breast, domain.Freezer, 400)},
			plan:   cooks(breast),
			want:   []domain.Movement{thawed(tonight, key(breast, domain.Freezer), 200, 3, lunch)},
		},
		{
			name:   "no more than the freezer holds",
			ledger: []domain.Movement{holds(breast, domain.Freezer, 100)},
			plan:   cooks(breast),
			want:   []domain.Movement{thawed(tonight, key(breast, domain.Freezer), 100, 3, lunch)},
		},
		{
			name:   "stock in the fridge not thawed again",
			ledger: []domain.Movement{holds(breast, domain.Fridge, 300), holds(breast, domain.Freezer, 400)},
			plan:   cooks(breast),
		},
		{
			name:   "an Ingredient used from frozen not thawed",
			ledger: []domain.Movement{holds(peas, domain.Freezer, 400)},
			plan:   cooks(peas),
		},
		{
			name:     "an ordered Delivery due before the day's first Slot held, but not its freezer split",
			ledger:   []domain.Movement{holds(breast, domain.Freezer, 400)},
			plan:     cooks(breast),
			arrivals: delivery(true),
			want:     []domain.Movement{thawed(tonight, key(breast, domain.Freezer), 100, 3, lunch)},
		},
		{
			name:     "a tentative Delivery not held",
			ledger:   []domain.Movement{holds(breast, domain.Freezer, 400)},
			plan:     cooks(breast),
			arrivals: delivery(false),
			want:     []domain.Movement{thawed(tonight, key(breast, domain.Freezer), 300, 3, lunch)},
		},
		{
			name:   "a frozen Portion filling the next day's Slot, after that dinner's own Movements",
			ledger: []domain.Movement{lasagne, chili},
			plan:   domain.Plan{Kept: []domain.Kept{{Portion: 8, At: dinner}, {Portion: 7, At: lunch}}},
			want: []domain.Movement{
				ate(tonight, domain.Fridge, 8, dinner),
				{At: tonight, Key: domain.StockKey{Location: domain.Freezer}, To: domain.Fridge, Portions: []int64{7}, Change: grams(1), Cause: domain.Thawed, Slot: lunch},
			},
		},
		{
			name:   "a Portion in the fridge not thawed",
			ledger: []domain.Movement{chili},
			plan:   domain.Plan{Kept: []domain.Kept{{Portion: 8, At: lunch}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := domain.NewLedger(tt.ledger).UpToNow(tt.plan, nil, tt.arrivals, domain.DefaultSlotTimes, at(6, "19:00"), at(6, "21:00"))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the walk\n%+v, want\n%+v", got, tt.want)
			}
			after := domain.NewLedger(append(slices.Clone(tt.ledger), got...))
			if again, _ := after.UpToNow(tt.plan, nil, tt.arrivals, domain.DefaultSlotTimes, at(6, "21:00"), at(6, "22:00")); len(again) != 0 {
				t.Errorf("an hour later the walk wrote %+v, want nothing", again)
			}
		})
	}
}
