package web

import (
	"strings"
	"testing"
)

// Every icon is a 10×10 bitmap of ink and paper, so it renders square and crisp.
func TestIconsAreTenByTenBitmaps(t *testing.T) {
	for name, rows := range icons {
		if len(rows) != 10 {
			t.Errorf("icon %q has %d rows, want 10", name, len(rows))
		}
		for y, row := range rows {
			if len(row) != 10 || strings.Trim(row, "#.") != "" {
				t.Errorf("icon %q row %d = %q, want 10 of '#' and '.'", name, y, row)
			}
		}
	}
}

// The pixel art's every colour is a class app.css fills from a token.
func TestPixelArtUsesOnlyItsColours(t *testing.T) {
	for name, rows := range map[string][]string{"sun": sun(), "fuji": fuji()} {
		for y, row := range rows {
			for _, r := range row {
				if _, ok := artColours[r]; !ok && r != '.' {
					t.Errorf("%s row %d has %q, which artColours lacks", name, y, r)
				}
			}
		}
	}
}
