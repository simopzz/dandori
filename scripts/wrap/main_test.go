package main

import (
	"slices"
	"strings"
	"testing"
)

// Go comments fill each line to width, and none leaves a lone word on its last line
// (docs/agents/coding-standards.md); mise run wrap rewraps them. Generated files and
// directives are not comments anyone wrapped.
func TestCommentsWrap(t *testing.T) {
	err := walk("../..", func(path, src string) error {
		for _, problem := range problems(src) {
			t.Errorf("%s:%s (mise run wrap rewraps it)", path, problem)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// long is a comment of 18 words, two columns too long, and full one of 17, with no room
// for another.
var (
	long = "// " + strings.TrimSpace(strings.Repeat("word ", 18))
	full = long[:87]
)

// The check itself: what it finds, and what it leaves alone.
func TestCommentProblems(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{"fits", full + "\n// and another line\n", nil},
		{"stops early", "// one line\n// and another line\n", []string{`1: stops before the next line's "and", which fits`}},
		{"stops early for the last two words", "// one line\n// two words\n", nil},
		{"a list stops where it likes", "// - one\n// - two\n", nil},
		{"a paragraph ends where it likes", "// one line\n//\n// and another line\n", nil},
		{"a lone word", "// one line\n// word\n", []string{"2: leaves a lone word on its last line", `1: stops before the next line's "word", which fits`}},
		{"one word alone is no wrap", "// word\n", nil},
		{"too long", long + "\n", []string{"1: runs to column 92"}},
		{"a tab counts as four", "\t" + long + "\n", []string{"1: runs to column 96"}},
		{"just fits", long[:90] + "\n", nil},
		{"a long URL can't wrap", "// https://example.com/" + strings.Repeat("a", 90) + "\n", nil},
		{"a directive isn't wrapped", "//go:build " + strings.Repeat("a ", 60) + "\n", nil},
		{"code separates two comments", "// one line\ncode\n// word\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := problems(c.src); !slices.Equal(got, c.want) {
				t.Errorf("problems = %q, want %q", got, c.want)
			}
		})
	}
}

// wrap fills again only the prose comments that break the rule, at their indent, and
// names the line of any other.
func TestWrap(t *testing.T) {
	sixteen := strings.TrimSpace(strings.Repeat("word ", 16))
	list := "// - " + strings.Repeat("item ", 18) + "\n"
	cases := []struct {
		name, src, want string
		left            []int
	}{
		{"too long, at its indent", "\t" + long + "\ncode\n", "\t// " + sixteen + "\n\t// word word\ncode\n", nil},
		{"the last line takes a word", long + "\n", "// " + sixteen + "\n// word word\n", nil},
		{"a lone word joins the line before", "// one line\n// word\n", "// one line word\n", nil},
		{"one that stops early is filled", "// one line\n// and one more\n", "// one line and one more\n", nil},
		{"one that fits is left alone", full + "\n// and another\n", full + "\n// and another\n", nil},
		{"a list is named, not filled", list, list, []int{1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, left := wrap(c.src)
			if got != c.want || !slices.Equal(left, c.left) {
				t.Errorf("wrap = %q, %v; want %q, %v", got, left, c.want, c.left)
			}
			if p := problems(got); len(p) > 0 && c.left == nil {
				t.Errorf("wrapped, it still breaks the rule: %q", p)
			}
		})
	}
}
