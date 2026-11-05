package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

var rome = mustZone("Europe/Rome")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// at is a moment of Monday 2 November 2026, or the day given, in Rome.
func at(day int, hm string) time.Time {
	t, err := time.Parse("15:04", hm)
	if err != nil {
		panic(err)
	}
	return time.Date(2026, time.November, day, t.Hour(), t.Minute(), t.Second(), 0, rome)
}

func slot(day int, kind domain.SlotKind) domain.Slot {
	return domain.Slot{On: time.Date(2026, time.November, day, 0, 0, 0, 0, rome), Kind: kind}
}

// A Slot kind is now from its time of day up to and including an hour after; otherwise
// the next one is next, past dinner tomorrow's breakfast. The day's later Slots follow.
func TestTodayIsTheSlotNowOrNext(t *testing.T) {
	late := domain.SlotTimes{domain.Breakfast: 9 * 60, domain.Lunch: 13 * 60, domain.Snack: 17 * 60, domain.Dinner: 23*60 + 30}
	tests := []struct {
		name      string
		at        time.Time
		times     domain.SlotTimes
		wantFirst domain.Slot
		wantNow   bool
		wantLater []domain.SlotKind
	}{
		{"before breakfast", at(2, "06:30"), domain.DefaultSlotTimes, slot(2, domain.Breakfast), false, []domain.SlotKind{domain.Lunch, domain.Snack, domain.Dinner}},
		{"just after midnight", at(2, "00:00"), domain.DefaultSlotTimes, slot(2, domain.Breakfast), false, []domain.SlotKind{domain.Lunch, domain.Snack, domain.Dinner}},
		{"at breakfast's time", at(2, "08:00"), domain.DefaultSlotTimes, slot(2, domain.Breakfast), true, []domain.SlotKind{domain.Lunch, domain.Snack, domain.Dinner}},
		{"an hour after breakfast's time", at(2, "09:00"), domain.DefaultSlotTimes, slot(2, domain.Breakfast), true, []domain.SlotKind{domain.Lunch, domain.Snack, domain.Dinner}},
		{"seconds into the hour's last minute", at(2, "09:00").Add(59 * time.Second), domain.DefaultSlotTimes, slot(2, domain.Breakfast), true, []domain.SlotKind{domain.Lunch, domain.Snack, domain.Dinner}},
		{"past breakfast's hour", at(2, "09:01"), domain.DefaultSlotTimes, slot(2, domain.Lunch), false, []domain.SlotKind{domain.Snack, domain.Dinner}},
		{"during lunch", at(2, "13:30"), domain.DefaultSlotTimes, slot(2, domain.Lunch), true, []domain.SlotKind{domain.Snack, domain.Dinner}},
		{"during dinner", at(2, "21:00"), domain.DefaultSlotTimes, slot(2, domain.Dinner), true, nil},
		{"past dinner's hour", at(2, "21:01"), domain.DefaultSlotTimes, slot(3, domain.Breakfast), false, nil},
		{"before midnight", at(2, "23:59"), domain.DefaultSlotTimes, slot(3, domain.Breakfast), false, nil},
		{"by the Profile's times", at(2, "08:30"), late, slot(2, domain.Breakfast), false, []domain.SlotKind{domain.Lunch, domain.Snack, domain.Dinner}},
		{"a dinner running past midnight", at(3, "00:15"), late, slot(2, domain.Dinner), true, []domain.SlotKind{domain.Breakfast, domain.Lunch, domain.Snack, domain.Dinner}},
		{"once it has run past midnight", at(3, "00:31"), late, slot(3, domain.Breakfast), false, []domain.SlotKind{domain.Lunch, domain.Snack, domain.Dinner}},
		{"a time of day closer than an hour to the next", at(2, "13:40"), domain.SlotTimes{domain.Breakfast: 8 * 60, domain.Lunch: 13 * 60, domain.Snack: 13*60 + 30, domain.Dinner: 20 * 60}, slot(2, domain.Snack), true, []domain.SlotKind{domain.Dinner}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.TodayAt(tt.at, tt.times)
			if !got.First.On.Equal(tt.wantFirst.On) || got.First.Kind != tt.wantFirst.Kind || got.Now != tt.wantNow {
				t.Errorf("TodayAt(%s) first = %s on %s, now %v; want %s on %s, now %v", tt.at.Format("Jan 2 15:04:05"),
					got.First.Kind, got.First.On.Format(time.DateOnly), got.Now, tt.wantFirst.Kind, tt.wantFirst.On.Format(time.DateOnly), tt.wantNow)
			}
			var later []domain.SlotKind
			for _, s := range got.Later {
				if !s.On.Equal(at(tt.at.Day(), "00:00")) {
					t.Errorf("TodayAt(%s) has a later Slot on %s, not today", tt.at.Format("Jan 2 15:04"), s.On.Format(time.DateOnly))
				}
				later = append(later, s.Kind)
			}
			if !slices.Equal(later, tt.wantLater) {
				t.Errorf("TodayAt(%s) later = %v, want %v", tt.at.Format("Jan 2 15:04"), later, tt.wantLater)
			}
		})
	}
}

// A Slot's time of day is on the wall clock, on a day the clocks go back as on any other.
func TestASlotIsAtItsTimeOfDayOnTheWallClock(t *testing.T) {
	// Sunday 25 October 2026: Rome's clocks go back at 03:00.
	got := domain.TodayAt(time.Date(2026, time.October, 25, 8, 30, 0, 0, rome), domain.DefaultSlotTimes)
	if got.First.Kind != domain.Breakfast || !got.Now {
		t.Errorf("at 08:30 on the day the clocks go back, first = %s, now %v; want breakfast, now", got.First.Kind, got.Now)
	}
}
