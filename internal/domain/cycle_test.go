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
			c := domain.Cycle{DeliveryOn: day("2026-11-02"), EndsOn: day(tt.ends)}
			if got := c.Validate()["EndsOn"]; got != tt.want {
				t.Errorf("Validate()[EndsOn] = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCycleEndsAboutTenDaysAfterDeliveryByDefault(t *testing.T) {
	if got := domain.DefaultCycleEnd(day("2026-11-02")); !got.Equal(day("2026-11-12")) {
		t.Errorf("DefaultCycleEnd(2026-11-02) = %s, want 2026-11-12", got.Format(time.DateOnly))
	}
}

func TestCycleClosedEarlyLastsUntilTheDayBeforeTheNextDelivery(t *testing.T) {
	closed := day("2026-11-08")
	c := domain.Cycle{DeliveryOn: day("2026-11-02"), EndsOn: day("2026-11-12"), ClosedOn: &closed}
	if got := c.LastDay(); !got.Equal(day("2026-11-07")) {
		t.Errorf("LastDay() = %s, want 2026-11-07", got.Format(time.DateOnly))
	}
	open := domain.Cycle{DeliveryOn: day("2026-11-02"), EndsOn: day("2026-11-12")}
	if got := open.LastDay(); !got.Equal(day("2026-11-12")) {
		t.Errorf("LastDay() of an open Cycle = %s, want its end date 2026-11-12", got.Format(time.DateOnly))
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

func TestOpeningACycle(t *testing.T) {
	current := domain.Cycle{DeliveryOn: day("2026-11-02"), EndsOn: day("2026-11-12")}
	tests := []struct {
		name          string
		delivery      string
		wantErr       error
		wantClose     bool
		wantDropped   []int64
		wantCarryOver []int64
	}{
		{name: "after the current Cycle ends, leaving a gap", delivery: "2026-11-15"},
		{name: "the day after the current Cycle ends", delivery: "2026-11-13"},
		{
			name: "on the current Cycle's last day closes it early", delivery: "2026-11-12",
			wantClose: true,
		},
		{
			// The Prep Session of the 9th is never prepared; the Portions of
			// the 5th still to eat are leftovers.
			name: "mid-Cycle carries over the leftovers", delivery: "2026-11-08",
			wantClose: true, wantDropped: []int64{2}, wantCarryOver: []int64{13, 31},
		},
		{
			name: "on a Prep Session's day drops it", delivery: "2026-11-09",
			wantClose: true, wantDropped: []int64{2}, wantCarryOver: []int64{13, 31},
		},
		{name: "on the current Cycle's delivery date overlaps it", delivery: "2026-11-02", wantErr: domain.ErrCycleOverlaps},
		{name: "before the current Cycle's delivery overlaps it", delivery: "2026-10-30", wantErr: domain.ErrCycleOverlaps},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := domain.Cycle{DeliveryOn: day(tt.delivery), EndsOn: domain.DefaultCycleEnd(day(tt.delivery))}
			got, err := domain.OpenCycle(&current, currentPlan(), next)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("OpenCycle() error = %v, want %v", err, tt.wantErr)
			}
			if got.ClosesCurrent != tt.wantClose {
				t.Errorf("ClosesCurrent = %v, want %v", got.ClosesCurrent, tt.wantClose)
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

func TestOpeningTheFirstCycle(t *testing.T) {
	next := domain.Cycle{DeliveryOn: day("2026-11-02"), EndsOn: day("2026-11-12")}
	got, err := domain.OpenCycle(nil, domain.Plan{}, next)
	if err != nil || got.ClosesCurrent || len(got.DroppedPrepSessions) > 0 || len(got.CarriedOver) > 0 {
		t.Errorf("OpenCycle(nil, …) = %+v, %v; want nothing to close or carry over", got, err)
	}
}
