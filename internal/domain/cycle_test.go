package domain_test

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// day is a day of November 2026 at midnight in Rome.
func day(d int) time.Time { return time.Date(2026, time.November, d, 0, 0, 0, 0, rome) }

// cycle is a Cycle whose Delivery comes on the day of November given, `from` and `to` the
// times of day given, lasting to the day `ends`.
func cycle(delivery int, from, to string, ends int) domain.Cycle {
	f, _ := domain.ParseTimeOfDay(from)
	t, _ := domain.ParseTimeOfDay(to)
	return domain.Cycle{Delivery: domain.Delivery{On: day(delivery), From: f, To: t}, Ends: day(ends)}
}

// Opening a Cycle reads its Delivery's date and window and its end date as typed, ten
// days after the Delivery's date when left blank, refusing each field on its own.
func TestOpeningACycle(t *testing.T) {
	now := at(2, "11:00")
	typed := func(change func(map[string]string)) map[string]string {
		f := map[string]string{
			domain.CycleDelivery: "11/05/2026", domain.CycleFrom: "18:00", domain.CycleTo: "20:00",
			domain.CycleEnds: "11/12/2026",
		}
		change(f)
		return f
	}
	tests := []struct {
		name     string
		typed    map[string]string
		want     domain.Cycle
		problems map[string]domain.Problem
	}{
		{"as typed", typed(func(map[string]string) {}), cycle(5, "18:00", "20:00", 12), nil},
		{"the end date left blank", typed(func(f map[string]string) { f[domain.CycleEnds] = "" }), cycle(5, "18:00", "20:00", 15), nil},
		{"14 days after", typed(func(f map[string]string) { f[domain.CycleEnds] = "11/19/2026" }), cycle(5, "18:00", "20:00", 19), nil},
		{"the day after", typed(func(f map[string]string) { f[domain.CycleEnds] = "11/06/2026" }), cycle(5, "18:00", "20:00", 6), nil},
		{"a window going on today", typed(func(f map[string]string) {
			f[domain.CycleDelivery], f[domain.CycleFrom], f[domain.CycleTo] = "11/02/2026", "10:00", "11:30"
		}), cycle(2, "10:00", "11:30", 12), nil},
		{"nothing typed", map[string]string{}, domain.Cycle{}, map[string]domain.Problem{
			domain.CycleDelivery: domain.Required, domain.CycleFrom: domain.Required, domain.CycleTo: domain.Required,
		}},
		{"not a date", typed(func(f map[string]string) { f[domain.CycleDelivery] = "soon" }), domain.Cycle{}, map[string]domain.Problem{
			domain.CycleDelivery: domain.NotADate,
		}},
		{"a window ending before it starts", typed(func(f map[string]string) { f[domain.CycleTo] = "17:00" }), domain.Cycle{}, map[string]domain.Problem{
			domain.CycleTo: domain.NotAfterFrom,
		}},
		{"a window ending as it starts", typed(func(f map[string]string) { f[domain.CycleTo] = "18:00" }), domain.Cycle{}, map[string]domain.Problem{
			domain.CycleTo: domain.NotAfterFrom,
		}},
		{"a window ended earlier today", typed(func(f map[string]string) {
			f[domain.CycleDelivery], f[domain.CycleFrom], f[domain.CycleTo] = "11/02/2026", "08:00", "11:00"
		}), domain.Cycle{}, map[string]domain.Problem{domain.CycleTo: domain.AlreadyEnded}},
		{"a Delivery yesterday", typed(func(f map[string]string) { f[domain.CycleDelivery] = "11/01/2026" }), domain.Cycle{}, map[string]domain.Problem{
			domain.CycleDelivery: domain.AlreadyEnded,
		}},
		{"a Delivery yesterday, its window ending before it starts", typed(func(f map[string]string) {
			f[domain.CycleDelivery], f[domain.CycleTo] = "11/01/2026", "17:00"
		}), domain.Cycle{}, map[string]domain.Problem{domain.CycleDelivery: domain.AlreadyEnded, domain.CycleTo: domain.NotAfterFrom}},
		{"an end date on the Delivery's", typed(func(f map[string]string) { f[domain.CycleEnds] = "11/05/2026" }), domain.Cycle{}, map[string]domain.Problem{
			domain.CycleEnds: domain.NotAfterDelivery,
		}},
		{"an end date before the Delivery's", typed(func(f map[string]string) { f[domain.CycleEnds] = "11/04/2026" }), domain.Cycle{}, map[string]domain.Problem{
			domain.CycleEnds: domain.NotAfterDelivery,
		}},
		{"an end date 15 days after", typed(func(f map[string]string) { f[domain.CycleEnds] = "11/20/2026" }), domain.Cycle{}, map[string]domain.Problem{
			domain.CycleEnds: domain.TooLong,
		}},
		{"an end date that isn't a date", typed(func(f map[string]string) { f[domain.CycleEnds] = "later" }), domain.Cycle{}, map[string]domain.Problem{
			domain.CycleEnds: domain.NotADate,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, problems := domain.ParseCycle(domain.English, tt.typed, now)
			if !maps.Equal(problems, tt.problems) && (len(problems) > 0 || len(tt.problems) > 0) {
				t.Fatalf("problems %v, want %v", problems, tt.problems)
			}
			if len(tt.problems) == 0 && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A Cycle is upcoming until its Delivery's window ends, current until the next one's
// ends, then closed; the last one opened stays current past its end date.
func TestACyclesStatus(t *testing.T) {
	cycles := []domain.Cycle{cycle(2, "18:00", "20:00", 10), cycle(9, "10:00", "12:00", 19)}
	tests := []struct {
		name string
		now  time.Time
		want []domain.CycleStatus
	}{
		{"before the first arrives", at(2, "19:00"), []domain.CycleStatus{domain.Upcoming, domain.Upcoming}},
		{"as the first's window ends", at(2, "20:00"), []domain.CycleStatus{domain.Current, domain.Upcoming}},
		{"before the second's window ends", at(9, "11:59"), []domain.CycleStatus{domain.Current, domain.Upcoming}},
		{"as the second's window ends", at(9, "12:00"), []domain.CycleStatus{domain.Closed, domain.Current}},
		{"past the second's end date", at(25, "12:00"), []domain.CycleStatus{domain.Closed, domain.Current}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.CycleStatuses(cycles, tt.now); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// A Cycle's Slots run from the first Slot after its Delivery window's end to the last of
// its end date, every kind every day, or to the last before the next Cycle's Delivery
// arrives, when it comes first: the later Slots of that day are the next Cycle's.
func TestACyclesSlots(t *testing.T) {
	early := cycle(2, "18:00", "20:00", 4)
	early.Until = cycle(3, "14:00", "16:00", 9).Delivery.Arrives()
	late := cycle(2, "18:00", "20:00", 3)
	late.Until = cycle(5, "14:00", "16:00", 9).Delivery.Arrives()
	tests := []struct {
		name        string
		cycle       domain.Cycle
		first, last domain.Slot
		count       int
	}{
		{"a window ending in the afternoon", cycle(2, "14:00", "16:00", 4), slot(2, domain.Snack), slot(4, domain.Dinner), 2 + 4 + 4},
		{"a window ending at a Slot's time", cycle(2, "18:00", "20:00", 4), slot(3, domain.Breakfast), slot(4, domain.Dinner), 8},
		{"a window ending in the morning", cycle(2, "06:00", "07:00", 3), slot(2, domain.Breakfast), slot(3, domain.Dinner), 8},
		{"the next Delivery arriving before the end date", early, slot(3, domain.Breakfast), slot(3, domain.Lunch), 2},
		{"the next Delivery arriving after the end date", late, slot(3, domain.Breakfast), slot(3, domain.Dinner), 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cycle.Slots(domain.DefaultSlotTimes)
			if len(got) != tt.count || got[0] != tt.first || got[len(got)-1] != tt.last {
				t.Errorf("got %d Slots from %v to %v, want %d from %v to %v", len(got), got[0], got[len(got)-1], tt.count, tt.first, tt.last)
			}
		})
	}
}

// A Cycle across a change of the clocks, from summer time to standard time in October
// and back in March, keeps its days at midnight, and its window and Slots on the wall
// clock: an hour further from UTC, or nearer, after the change.
func TestACycleAcrossAChangeOfTheClocks(t *testing.T) {
	utc := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, time.UTC) }
	tests := []struct {
		name                  string
		now                   time.Time
		delivery              string
		arrives, ends, dinner time.Time
	}{
		{"to standard time", time.Date(2026, time.October, 20, 10, 0, 0, 0, rome), "10/24/2026",
			utc(2026, time.October, 24, 18), time.Date(2026, time.November, 3, 0, 0, 0, 0, rome), utc(2026, time.October, 25, 19)},
		{"to summer time", time.Date(2027, time.March, 25, 10, 0, 0, 0, rome), "03/27/2027",
			utc(2027, time.March, 27, 19), time.Date(2027, time.April, 6, 0, 0, 0, 0, rome), utc(2027, time.March, 28, 18)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typed := map[string]string{domain.CycleDelivery: tt.delivery, domain.CycleFrom: "18:00", domain.CycleTo: "20:00"}
			c, problems := domain.ParseCycle(domain.English, typed, tt.now)
			if len(problems) > 0 {
				t.Fatalf("refused: %v", problems)
			}
			if !c.Delivery.Arrives().Equal(tt.arrives) {
				t.Errorf("arrives at %v, want %v", c.Delivery.Arrives().UTC(), tt.arrives)
			}
			if !c.Ends.Equal(tt.ends) {
				t.Errorf("ends on %v, want %v", c.Ends, tt.ends)
			}
			slots := c.Slots(domain.DefaultSlotTimes)
			if len(slots) != 40 {
				t.Errorf("has %d Slots, want 40: four a day for ten days", len(slots))
			}
			if got := slots[3].At(domain.DefaultSlotTimes); slots[3].Kind != domain.Dinner || !got.Equal(tt.dinner) {
				t.Errorf("the first day's dinner is %v at %v, want at %v", slots[3].Kind, got.UTC(), tt.dinner)
			}
			cycles := []domain.Cycle{c}
			if got := domain.CycleStatuses(cycles, tt.arrives.Add(-time.Minute)); got[0] != domain.Upcoming {
				t.Errorf("a minute before arriving it is %v, want upcoming", got[0])
			}
			if got := domain.CycleStatuses(cycles, tt.arrives); got[0] != domain.Current {
				t.Errorf("as it arrives it is %v, want current", got[0])
			}
		})
	}
}
