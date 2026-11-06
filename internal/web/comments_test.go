package web

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A Go comment runs to commentWidth columns, counting a tab as commentTab.
const (
	commentWidth = 90
	commentTab   = 4
)

// Go comments wrap at commentWidth, and none leaves a lone word on its last line
// (docs/agents/coding-standards.md). Generated files and directives are not comments
// anyone wrapped.
func TestCommentsWrap(t *testing.T) {
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if (ext != ".go" && ext != ".templ") || strings.HasSuffix(path, "_templ.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, problem := range commentProblems(string(src)) {
			t.Errorf("%s:%s", path, problem)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The check itself: what it finds, and what it leaves alone.
func TestCommentProblems(t *testing.T) {
	long := "// " + strings.TrimSpace(strings.Repeat("word ", 18))
	cases := []struct {
		name, src string
		want      []string
	}{
		{"fits", "// one line\n// and another line\n", nil},
		{"a lone word", "// one line\n// word\n", []string{"2: leaves a lone word on its last line"}},
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
			if got := commentProblems(c.src); !slices.Equal(got, c.want) {
				t.Errorf("commentProblems = %q, want %q", got, c.want)
			}
		})
	}
}

// commentProblems says where src breaks the wrapping rule, as "line: why".
func commentProblems(src string) []string {
	var problems []string
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); {
		if !isComment(lines[i]) {
			i++
			continue
		}
		start := i
		for i < len(lines) && isComment(lines[i]) {
			if w := commentColumns(lines[i]); w > commentWidth && strings.Contains(commentText(lines[i]), " ") {
				problems = append(problems, fmt.Sprintf("%d: runs to column %d", i+1, w))
			}
			i++
		}
		if i-start > 1 && len(strings.Fields(commentText(lines[i-1]))) == 1 {
			problems = append(problems, fmt.Sprintf("%d: leaves a lone word on its last line", i))
		}
	}
	return problems
}

// isComment is whether line is a whole-line comment a person wrapped: not a directive
// such as //go:build or //nolint.
func isComment(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "// ")
}

// commentText is line's comment without its marker.
func commentText(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "//"))
}

// commentColumns is how many columns line takes.
func commentColumns(line string) int {
	return len([]rune(strings.ReplaceAll(line, "\t", strings.Repeat(" ", commentTab))))
}
