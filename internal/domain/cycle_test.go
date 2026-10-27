package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// day is the date s, as YYYY-MM-DD.
func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func at(date string, kind domain.SlotKind) *domain.Slot {
	return &domain.Slot{On: day(date), Kind: kind}
}

func TestCycleEndsAfterItsDelivery(t *testing.T) {
	tests := []struct {
		name, ends, want string
	}{
		{"ten days later", "2026-11-12", ""},
		{"the next day", "2026-11-03", ""},
		{"the same day", "2026-11-02", "Must be after the delivery date"},
		{"before", "2026-11-01", "Must be after the delivery date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := delivered("2026-11-02", "07:00", "07:30", tt.ends)
			if got := c.Validate()["EndsOn"]; got != tt.want {
				t.Errorf("Validate()[EndsOn] = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeliveryWindowEndsAfterItStarts(t *testing.T) {
	for _, tt := range []struct{ from, to, want string }{
		{"18:00", "20:00", ""},
		{"18:00", "18:00", "Must be after the start of the window"},
		{"20:00", "18:00", "Must be after the start of the window"},
	} {
		if got := delivered("2026-11-02", tt.from, tt.to, "2026-11-12").Validate()["Delivery.To"]; got != tt.want {
			t.Errorf("window %s–%s: Validate()[Delivery.To] = %q, want %q", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestCycleEndsAboutTenDaysAfterDeliveryByDefault(t *testing.T) {
	if got := domain.DefaultCycleEnd(day("2026-11-02")); !got.Equal(day("2026-11-12")) {
		t.Errorf("DefaultCycleEnd(2026-11-02) = %s, want 2026-11-12", got.Format(time.DateOnly))
	}
}

func TestCycleIsUpcomingUntilTheEndOfItsDeliveryWindow(t *testing.T) {
	c := delivered("2026-11-02", "18:00", "20:00", "2026-11-12")
	rome := time.FixedZone("Rome", 3600)
	for _, tt := range []struct {
		now  time.Time
		want bool
	}{
		{time.Date(2026, 11, 1, 21, 0, 0, 0, rome), false},
		{time.Date(2026, 11, 2, 19, 59, 0, 0, rome), false},
		// The window is on the user's wall clock, whatever the zone.
		{time.Date(2026, 11, 2, 20, 0, 0, 0, rome), true},
		{time.Date(2026, 11, 3, 8, 0, 0, 0, rome), true},
	} {
		if got := c.Arrived(tt.now); got != tt.want {
			t.Errorf("Arrived(%s) = %v, want %v", tt.now, got, tt.want)
		}
	}
}

// currentPlan is a Plan of the Cycle 2026-11-02 to 2026-11-12: a dinner
// Prep Session on the 5th eaten on the 5th, 7th and 10th; another on the
// 9th eaten that day and on the 11th; and a carried-over Portion eaten on
// the 10th.
func currentPlan() domain.Plan {
	return domain.Plan{
		PrepSessions: []domain.PrepSession{
			{ID: 1, At: *at("2026-11-05", domain.Dinner), Portions: []domain.PlannedPortion{
				{ID: 11, EatAt: at("2026-11-05", domain.Dinner)},
				{ID: 12, EatAt: at("2026-11-07", domain.Lunch)},
				{ID: 13, EatAt: at("2026-11-10", domain.Dinner)},
			}},
			{ID: 2, At: *at("2026-11-09", domain.Dinner), Portions: []domain.PlannedPortion{
				{ID: 21, EatAt: at("2026-11-09", domain.Dinner)},
				{ID: 22, EatAt: at("2026-11-11", domain.Lunch)},
			}},
		},
		Kept: []domain.KeptAssignment{{PortionID: 31, Slot: *at("2026-11-10", domain.Lunch)}},
	}
}

func TestArrivalOfTheNextDelivery(t *testing.T) {
	current := delivered("2026-11-02", "07:00", "07:30", "2026-11-12")
	tests := []struct {
		name          string
		delivery, to  string
		wantClosedAt  *domain.Slot
		wantDropped   []int64
		wantCarryOver []int64
	}{
		{name: "after the current Cycle ends, leaving a gap", delivery: "2026-11-15", to: "10:00"},
		{name: "the day after the current Cycle ends", delivery: "2026-11-13", to: "10:00"},
		{
			name: "on the current Cycle's last day closes it early", delivery: "2026-11-12", to: "10:00",
			wantClosedAt: at("2026-11-12", domain.Lunch),
		},
		{
			name: "after dinner on the last day leaves it whole", delivery: "2026-11-12", to: "21:00",
		},
		{
			// The Prep Session of the 9th is never prepared; the Portions of
			// the 5th still to eat are what is left.
			name: "mid-Cycle carries over what is left", delivery: "2026-11-08", to: "10:00",
			wantClosedAt: at("2026-11-08", domain.Lunch), wantDropped: []int64{2}, wantCarryOver: []int64{13, 31},
		},
		{
			name: "before a Prep Session's Slot drops it", delivery: "2026-11-09", to: "19:00",
			wantClosedAt: at("2026-11-09", domain.Dinner), wantDropped: []int64{2}, wantCarryOver: []int64{13, 31},
		},
		{
			// Dinner on the 9th is still the current Cycle's: it is prepared,
			// and its Portion for the 11th carries over.
			name: "after a Prep Session's Slot keeps it", delivery: "2026-11-09", to: "21:00",
			wantClosedAt: at("2026-11-10", domain.Breakfast), wantCarryOver: []int64{13, 22, 31},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := delivered(tt.delivery, "08:00", tt.to, domain.DefaultCycleEnd(day(tt.delivery)).Format(time.DateOnly))
			got := domain.Arrive(&current, currentPlan(), next)
			if got.ClosesEarly != (tt.wantClosedAt != nil) {
				t.Errorf("ClosesEarly = %v, want %v", got.ClosesEarly, tt.wantClosedAt != nil)
			}
			if (got.Current.ClosedAt == nil) != (tt.wantClosedAt == nil) || got.Current.ClosedAt != nil && *got.Current.ClosedAt != *tt.wantClosedAt {
				t.Errorf("Current.ClosedAt = %v, want %v", got.Current.ClosedAt, tt.wantClosedAt)
			}
			if !slices.Equal(got.DroppedPrepSessions, tt.wantDropped) {
				t.Errorf("DroppedPrepSessions = %v, want %v", got.DroppedPrepSessions, tt.wantDropped)
			}
			if !slices.Equal(got.CarriedOver, tt.wantCarryOver) {
				t.Errorf("CarriedOver = %v, want %v", got.CarriedOver, tt.wantCarryOver)
			}
		})
	}
}

func TestArrivalOfTheFirstDelivery(t *testing.T) {
	next := delivered("2026-11-02", "07:00", "07:30", "2026-11-12")
	got := domain.Arrive(nil, domain.Plan{}, next)
	if got.ClosesEarly || len(got.DroppedPrepSessions) > 0 || len(got.CarriedOver) > 0 {
		t.Errorf("Arrive(nil, …) = %+v; want nothing to close or carry over", got)
	}
}

func TestOpeningACycleLeavesTheCurrentOneUntouched(t *testing.T) {
	current := delivered("2026-11-02", "07:00", "07:30", "2026-11-12")
	next := delivered("2026-11-08", "18:00", "20:00", "2026-11-18")
	if err := domain.OpenCycle(&current, noon("2026-11-05"), next); err != nil {
		t.Errorf("OpenCycle() = %v, want the Cycle opened", err)
	}
	if current.ClosedAt != nil {
		t.Errorf("opening a Cycle closed the current one at %s", *current.ClosedAt)
	}
}

func TestOpeningACycleIsRefused(t *testing.T) {
	current := delivered("2026-11-02", "07:00", "07:30", "2026-11-12")
	tests := []struct {
		name, delivery, now string
		want                error
	}{
		{"on the current Cycle's delivery date", "2026-11-02", "2026-11-05", domain.ErrCycleOverlaps},
		{"before the current Cycle's delivery", "2026-10-30", "2026-11-05", domain.ErrCycleOverlaps},
		{"while a Cycle is upcoming", "2026-11-08", "2026-11-01", domain.ErrCycleUpcoming},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := delivered(tt.delivery, "18:00", "20:00", domain.DefaultCycleEnd(day(tt.delivery)).Format(time.DateOnly))
			if err := domain.OpenCycle(&current, noon(tt.now), next); !errors.Is(err, tt.want) {
				t.Errorf("OpenCycle() = %v, want %v", err, tt.want)
			}
		})
	}
	if err := domain.OpenCycle(nil, noon("2026-11-01"), current); err != nil {
		t.Errorf("OpenCycle() of the first Cycle = %v, want it opened", err)
	}
}

func noon(date string) time.Time { return day(date).Add(12 * time.Hour) }

func TestMovingADelivery(t *testing.T) {
	current := delivered("2026-11-02", "07:00", "07:30", "2026-11-12")
	upcoming := delivered("2026-11-12", "18:00", "20:00", "2026-11-22")
	tests := []struct {
		name, delivery, to string
		wantFlag           bool
		wantErr            error
	}{
		{"earlier", "2026-11-10", "20:00", false, nil},
		{"to the day after the current Plan ends", "2026-11-13", "07:30", false, nil},
		{"past the current Plan's end", "2026-11-13", "10:00", true, nil},
		{"days past the current Plan's end", "2026-11-15", "20:00", true, nil},
		{"onto the current Cycle's delivery date", "2026-11-02", "20:00", false, domain.ErrCycleOverlaps},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moved := delivered(tt.delivery, "06:00", tt.to, "2026-11-22")
			flag, err := domain.MoveDelivery(&current, upcoming, moved)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("MoveDelivery() error = %v, want %v", err, tt.wantErr)
			}
			if flag != tt.wantFlag {
				t.Errorf("current Plan needs a new Plan Proposal = %v, want %v", flag, tt.wantFlag)
			}
		})
	}
	// A current Plan that already fell short of the Delivery isn't flagged
	// again.
	gap := delivered("2026-11-15", "18:00", "20:00", "2026-11-25")
	if flag, _ := domain.MoveDelivery(&current, gap, delivered("2026-11-16", "18:00", "20:00", "2026-11-25")); flag {
		t.Errorf("a move within a gap flags the current Plan")
	}
}

func TestOnlyATentativeDeliverysCycleCanBeDropped(t *testing.T) {
	c := delivered("2026-11-12", "18:00", "20:00", "2026-11-22")
	if err := c.Drop(); err != nil {
		t.Errorf("Drop() of a tentative Delivery's Cycle = %v, want nil", err)
	}
	c.Delivery.Ordered = true
	if err := c.Drop(); !errors.Is(err, domain.ErrDeliveryOrdered) {
		t.Errorf("Drop() of an ordered Delivery's Cycle = %v, want ErrDeliveryOrdered", err)
	}
}

func TestPlanningTheNextCycleIsDueThreeDaysBeforeTheEndDate(t *testing.T) {
	c := delivered("2026-11-02", "07:00", "07:30", "2026-11-12")
	for date, want := range map[string]bool{"2026-11-08": false, "2026-11-09": true, "2026-11-12": true, "2026-11-14": true} {
		if got := c.PlanNextDue(day(date)); got != want {
			t.Errorf("PlanNextDue(%s) = %v, want %v", date, got, want)
		}
	}
}

// delivered is a Cycle delivered on date in the window from–to, ending on
// ends, with the default Slot times.
func delivered(date, from, to, ends string) domain.Cycle {
	d := domain.Delivery{On: day(date), From: clock(from), To: clock(to)}
	return domain.NewCycle(d, day(ends), domain.DefaultSlotTimes)
}

func clock(s string) domain.TimeOfDay {
	t, err := domain.ParseTimeOfDay(s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestCycleStartsAtTheFirstSlotAfterItsDeliveryWindow(t *testing.T) {
	tests := []struct {
		name, from, to string
		want           *domain.Slot
	}{
		{"before breakfast", "06:00", "07:30", at("2026-11-02", domain.Breakfast)},
		{"ending at lunch time", "11:00", "13:00", at("2026-11-02", domain.Lunch)},
		{"in the afternoon", "14:00", "16:00", at("2026-11-02", domain.Snack)},
		{"ending just after snack time", "15:00", "17:01", at("2026-11-02", domain.Dinner)},
		{"ending after dinner time", "19:00", "21:00", at("2026-11-03", domain.Breakfast)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := delivered("2026-11-02", tt.from, tt.to, "2026-11-12")
			if c.Start != *tt.want {
				t.Errorf("Start = %s, want %s", c.Start, *tt.want)
			}
			if !c.Contains(*tt.want) {
				t.Errorf("Contains(%s) = false, want its first Slot in it", *tt.want)
			}
		})
	}
}

func TestSlotsEarlierOnTheDeliveryDayStayWithTheCurrentCycle(t *testing.T) {
	current := delivered("2026-11-02", "07:00", "07:30", "2026-11-12")
	next := delivered("2026-11-08", "14:00", "16:00", "2026-11-18")
	current = domain.Arrive(&current, domain.Plan{}, next).Current
	for _, tt := range []struct {
		slot          *domain.Slot
		current, next bool
	}{
		{at("2026-11-07", domain.Dinner), true, false},
		{at("2026-11-08", domain.Lunch), true, false},
		{at("2026-11-08", domain.Snack), false, true},
		{at("2026-11-12", domain.Dinner), false, true},
	} {
		if got := current.Contains(*tt.slot); got != tt.current {
			t.Errorf("current Cycle Contains(%s) = %v, want %v", *tt.slot, got, tt.current)
		}
		if got := next.Contains(*tt.slot); got != tt.next {
			t.Errorf("next Cycle Contains(%s) = %v, want %v", *tt.slot, got, tt.next)
		}
	}
	if !current.LastDay().Equal(day("2026-11-08")) {
		t.Errorf("LastDay() = %s, want the delivery day 2026-11-08, its lunch still in it", current.LastDay().Format(time.DateOnly))
	}
}

func TestMovingADeliveryLaterMovesTheEndDateWithIt(t *testing.T) {
	c := delivered("2026-11-12", "18:00", "20:00", "2026-11-22")
	tests := []struct {
		name, delivery, wantEnds string
		wantStart                *domain.Slot
	}{
		// The Delivery is to last as long, so the Cycle keeps its length.
		{"two days later", "2026-11-14", "2026-11-24", at("2026-11-14", domain.Dinner)},
		// What is left may last to the same end date: the next Plan
		// Proposal judges.
		{"two days earlier", "2026-11-10", "2026-11-22", at("2026-11-10", domain.Dinner)},
		{"the same day, another window", "2026-11-12", "2026-11-22", at("2026-11-12", domain.Dinner)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := c.Delivery
			d.On = day(tt.delivery)
			got := c.MovedTo(d, domain.DefaultSlotTimes)
			if !got.EndsOn.Equal(day(tt.wantEnds)) {
				t.Errorf("EndsOn = %s, want %s", got.EndsOn.Format(time.DateOnly), tt.wantEnds)
			}
			if got.Start != *tt.wantStart {
				t.Errorf("Start = %s, want %s", got.Start, *tt.wantStart)
			}
		})
	}
}
