package web

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// The pixel art of Pixel ledger, the README banner's look: the setting sun and Fuji over
// a tree line, drawn as bitmaps and rendered as inline SVG on the server. Each colour of
// a bitmap is a rune; its pixels become one path whose fill a token sets in app.css
// (px-sun, px-snow, px-slate, px-navy). docs/design.md says where the art may go.

// artColours are the classes the runes of the pixel art are filled with.
var artColours = map[rune]string{
	'v': "px-sun",
	's': "px-snow",
	'l': "px-slate",
	'n': "px-navy",
}

var (
	// sunArt is the banner's sun, striped as it sets: 16×16.
	sunArt = pixelArt(sun())
	// fujiArt is Fuji, snow-capped, over a tree line: 60×24.
	fujiArt = pixelArt(fuji())
)

// pixelArt is rows, a bitmap in artColours' runes, as an aria-hidden SVG one unit per
// pixel, sized by its class in app.css.
func pixelArt(rows []string) string {
	paths := pixelPaths(rows)
	runes := make([]rune, 0, len(paths))
	for r := range paths {
		runes = append(runes, r)
	}
	slices.Sort(runes)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="px" aria-hidden="true" viewBox="0 0 %d %d" preserveAspectRatio="xMaxYMax meet" shape-rendering="crispEdges">`, len(rows[0]), len(rows))
	for _, r := range runes {
		fmt.Fprintf(&b, `<path class="%s" d="%s"/>`, artColours[r], paths[r])
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// pixelPaths turns a bitmap into SVG path data, one path per rune but '.', with each
// horizontal run of a rune's pixels one rectangle.
func pixelPaths(rows []string) map[rune]string {
	paths := map[rune]*strings.Builder{}
	for y, row := range rows {
		cells := []rune(row)
		for x := 0; x < len(cells); {
			r, w := cells[x], 1
			for x+w < len(cells) && cells[x+w] == r {
				w++
			}
			if r != '.' {
				if paths[r] == nil {
					paths[r] = &strings.Builder{}
				}
				fmt.Fprintf(paths[r], "M%d %dh%dv1h-%dz", x, y, w, w)
			}
			x += w
		}
	}
	d := make(map[rune]string, len(paths))
	for r, b := range paths {
		d[r] = b.String()
	}
	return d
}

// ditherSteps are the classes of a ramp's steps down through colours, sky tokens named
// without their prefix: each colour solid, then the next over it at 25, 50 and 75%, and
// the last solid.
func ditherSteps(colours []string) [][]string {
	var steps [][]string
	for i, under := range colours[:len(colours)-1] {
		on := colours[i+1]
		for _, density := range []string{"0", "25", "50", "75"} {
			steps = append(steps, []string{"dither", "dither-" + density, "under-" + under, "on-" + on})
		}
	}
	return append(steps, []string{"dither", "dither-0", "under-" + colours[len(colours)-1]})
}

// canvas is a blank bitmap of w×h to draw pixel art on.
type canvas [][]rune

func newCanvas(w, h int) canvas {
	c := make(canvas, h)
	for y := range c {
		c[y] = []rune(strings.Repeat(".", w))
	}
	return c
}

// fill colours the run of w pixels from x on row y, clipped to the canvas.
func (c canvas) fill(x, y, w int, r rune) {
	for i := max(x, 0); i < min(x+w, len(c[y])); i++ {
		c[y][i] = r
	}
}

func (c canvas) rows() []string {
	rows := make([]string, len(c))
	for y, row := range c {
		rows[y] = string(row)
	}
	return rows
}

func sun() []string {
	const n = 16
	c := newCanvas(n, n)
	for y := range n {
		if y == 10 || y == 12 || y == 14 {
			continue // the stripes it sets behind
		}
		for x := range n {
			if dx, dy := float64(x)-7.5, float64(y)-7.5; dx*dx+dy*dy < 62 {
				c.fill(x, y, 1, 'v')
			}
		}
	}
	return c.rows()
}

func fuji() []string {
	const w, h, peak = 60, 24, 38
	c := newCanvas(w, h)
	for y := 2; y < h; y++ {
		half := int(math.Round(float64(y-2) * 1.35))
		if y%3 == 0 {
			half++
		}
		colour := 'l'
		if y < 6 {
			colour = 's'
		}
		c.fill(peak-half, y, 2*half+1, colour)
		if y >= 6 && y < 9 {
			c.fill(peak-(y-3), y, 2, 's') // snow running down its west face
		}
	}
	for y := 18; y < h; y++ {
		c.fill(0, y, 26, 'l') // the hills to its west
	}
	for t := 0; t < w; t += 5 {
		th := 3 + (t*7)%3
		for i := range th {
			c.fill(t+2-i/2, h-th+i, 1+2*(i/2), 'n')
		}
	}
	c.fill(0, h-1, w, 'n')
	return c.rows()
}
