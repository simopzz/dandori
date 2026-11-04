package clock_test

import (
	"context"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/clock"
)

// A Manual clock beats when it is set past a multiple of the interval, once for however
// many it passes, at the latest of them; set short of the next one, it doesn't.
func TestManualBeatsWhenSetPastAMultiple(t *testing.T) {
	start := time.Date(2026, time.November, 2, 14, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		setTo []time.Time
		want  []time.Time
	}{
		{"short of the next", []time.Time{start.Add(4 * time.Minute)}, nil},
		{"onto the next", []time.Time{start.Add(5 * time.Minute)}, []time.Time{start.Add(5 * time.Minute)}},
		{"past several", []time.Time{start.Add(17 * time.Minute)}, []time.Time{start.Add(15 * time.Minute)}},
		{"to the same moment again", []time.Time{start.Add(5 * time.Minute), start.Add(5 * time.Minute)}, []time.Time{start.Add(5 * time.Minute)}},
		{"twice within one interval", []time.Time{start.Add(6 * time.Minute), start.Add(9 * time.Minute)}, []time.Time{start.Add(5 * time.Minute)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := clock.NewManual(start)
			var beats []time.Time
			c.Every(t.Context(), 5*time.Minute, func(now time.Time) { beats = append(beats, now) })
			for _, at := range tc.setTo {
				c.Set(at)
			}
			if len(beats) != len(tc.want) {
				t.Fatalf("beats = %v, want %v", beats, tc.want)
			}
			for i := range beats {
				if !beats[i].Equal(tc.want[i]) {
					t.Fatalf("beats = %v, want %v", beats, tc.want)
				}
			}
		})
	}
}

// Once its context ends, a Manual clock's beat stops.
func TestManualStopsBeatingWhenItsContextEnds(t *testing.T) {
	start := time.Date(2026, time.November, 2, 14, 0, 0, 0, time.UTC)
	c := clock.NewManual(start)
	ctx, cancel := context.WithCancel(t.Context())
	beats := 0
	c.Every(ctx, 5*time.Minute, func(time.Time) { beats++ })
	cancel()
	c.Set(start.Add(time.Hour))
	if beats != 0 {
		t.Fatalf("beats = %d after the context ended, want 0", beats)
	}
}
