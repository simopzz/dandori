package web

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"
)

// Icon is a pixel icon, inline, in the text colour. It is decorative: say what it means
// in text next to it or in an aria-label. A name missing from icons fails the render, so
// a typo shows up in tests, not as a blank.
func Icon(name string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		rows, ok := icons[name]
		if !ok {
			return fmt.Errorf("web: no icon %q in icons.go", name)
		}
		_, err := io.WriteString(w, `<svg class="icon" aria-hidden="true" width="20" height="20" viewBox="0 0 10 10" shape-rendering="crispEdges" fill="currentColor"><path d="`+pixelPaths(rows)['#']+`"/></svg>`)
		return err
	})
}

// icons are 10×10 bitmaps, one string per row, drawn on a 20px square so each pixel is
// two: '#' is ink, '.' is paper. Add one by drawing it here.
var icons = map[string][]string{
	// Runs: a Run's activity, as bars rising.
	"activity": {
		"..........",
		"........#.",
		"........#.",
		".....#..#.",
		".....#..#.",
		"..#..#..#.",
		"..#..#..#.",
		"..#..#..#.",
		"..#..#..#.",
		"##########",
	},
	// Recipes: an open book.
	"book-open": {
		".###..###.",
		"#...##...#",
		"#...##...#",
		"#...##...#",
		"#...##...#",
		"#...##...#",
		"#...##...#",
		"#...##...#",
		"####..####",
		"..........",
	},
	// A floating Notice: done as asked.
	"check": {
		"..........",
		"..........",
		".........#",
		"........##",
		"#......##.",
		"##....##..",
		".##..##...",
		"..####....",
		"...##.....",
		"..........",
	},
	// Plan: a calendar page.
	"calendar": {
		".#......#.",
		"##########",
		"#........#",
		"##########",
		"#........#",
		"#.##.##..#",
		"#........#",
		"#.##.##..#",
		"#........#",
		"##########",
	},
	"chevron-down": {
		"..........",
		"..........",
		"..........",
		"##......##",
		".##....##.",
		"..##..##..",
		"...####...",
		"....##....",
		"..........",
		"..........",
	},
	// Kitchen: a labelled crate.
	"package": {
		"..######..",
		"..#....#..",
		".########.",
		"#........#",
		"#.######.#",
		"#.#....#.#",
		"#.######.#",
		"#........#",
		"#........#",
		"##########",
	},
	// Catalog: a shopping basket.
	"shopping-basket": {
		"..........",
		"...####...",
		"..#....#..",
		".#......#.",
		"##########",
		".#.#..#.#.",
		".#.#..#.#.",
		".#.#..#.#.",
		"..######..",
		"..........",
	},
	// A Count: tally marks.
	"tally": {
		".........#",
		".#.#.#.##.",
		".#.#.#.#..",
		".#.#.###..",
		".#.###.#..",
		".#.#.#.#..",
		".###.#.#..",
		".#.#.#.#..",
		"##.#.#.#..",
		"..........",
	},
	// A Portion: a bowl, still warm.
	"bowl": {
		"..........",
		"...#..#...",
		"..#..#....",
		"...#..#...",
		"..........",
		"##########",
		"##########",
		".########.",
		"..######..",
		"...####...",
	},
	"plus": {
		"....##....",
		"....##....",
		"....##....",
		"....##....",
		"##########",
		"##########",
		"....##....",
		"....##....",
		"....##....",
		"....##....",
	},
	// A search: a magnifying glass.
	"search": {
		"..###.....",
		".#...#....",
		"#.....#...",
		"#.....#...",
		"#.....#...",
		".#...#....",
		"..###.#...",
		".......#..",
		"........#.",
		".........#",
	},
	// Profile: a bust.
	"user": {
		"...####...",
		"..#....#..",
		"..#....#..",
		"..#....#..",
		"...####...",
		"..........",
		".########.",
		"#........#",
		"#........#",
		"##########",
	},
}
