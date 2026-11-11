package web_test

import (
	"context"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// Dates and times are written as the page's Language writes them.
func TestDatesAndTimesFollowTheLanguage(t *testing.T) {
	// Tuesday 3 November 2026, 13:00:05.
	at := time.Date(2026, time.November, 3, 13, 0, 5, 0, time.UTC)
	for _, tt := range []struct {
		l                                            domain.Language
		date, short, shortYear, clock, clockSec, tod string
	}{
		{domain.English, "Tuesday, November 3", "Tue, Nov 3", "Tue, Nov 3, 2026", "1:00 pm", "1:00:05 pm", "12:00 am"},
		{domain.Italian, "martedì 3 novembre", "Mar 3 Nov", "Mar 3 Nov 2026", "13:00", "13:00:05", "00:00"},
	} {
		ctx := web.WithLanguage(context.Background(), tt.l)
		for name, c := range map[string][2]string{
			"Date":            {web.Date(ctx, at), tt.date},
			"ShortDate":       {web.ShortDate(ctx, at), tt.short},
			"ShortDateInYear": {web.ShortDateInYear(ctx, at), tt.shortYear},
			"Time":            {web.Time(ctx, at), tt.clock},
			"TimeWithSeconds": {web.TimeWithSeconds(ctx, at), tt.clockSec},
			"TimeOfDay(0)":    {web.TimeOfDay(ctx, 0), tt.tod},
		} {
			if c[0] != c[1] {
				t.Errorf("%s in %s = %q, want %q", name, tt.l, c[0], c[1])
			}
		}
	}
}
