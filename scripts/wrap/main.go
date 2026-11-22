// Command wrap rewraps the Go comments that break the wrapping rule TestCommentsWrap
// enforces, in every .go and .templ file under the paths it is given, or under the
// directory it runs in without any: mise run wrap. A comment that isn't plain prose,
// such as a list, it names for rewrapping by hand.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// A Go comment runs to width columns, counting a tab as tab.
const (
	width = 90
	tab   = 4
)

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	for _, root := range roots {
		err := walk(root, func(path, src string) error {
			out, left := wrap(src)
			for _, line := range left {
				fmt.Printf("%s:%d: not plain prose: rewrap by hand\n", path, line)
			}
			if out == src {
				return nil
			}
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			fmt.Println("rewrapped", path)
			return os.WriteFile(path, []byte(out), info.Mode().Perm())
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

// walk calls fn with each .go and .templ file under root a person wrote, and its source.
func walk(root string, fn func(path, src string) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		return fn(path, string(src))
	})
}

// problems says where src breaks the wrapping rule, as "line: why".
func problems(src string) []string {
	var found []string
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); {
		if !isComment(lines[i]) {
			i++
			continue
		}
		start := i
		for i < len(lines) && isComment(lines[i]) {
			if w := columns(lines[i]); w > width && strings.Contains(text(lines[i]), " ") {
				found = append(found, fmt.Sprintf("%d: runs to column %d", i+1, w))
			}
			i++
		}
		if i-start > 1 && len(strings.Fields(text(lines[i-1]))) == 1 {
			found = append(found, fmt.Sprintf("%d: leaves a lone word on its last line", i))
		}
	}
	return found
}

// wrap is src with each comment that breaks the wrapping rule filled again, and the
// lines where those that aren't plain prose start, left as they are.
func wrap(src string) (string, []int) {
	var out []string
	var left []int
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); {
		if !isComment(lines[i]) {
			out = append(out, lines[i])
			i++
			continue
		}
		start := i
		for i < len(lines) && isComment(lines[i]) {
			i++
		}
		block := lines[start:i]
		switch {
		case len(problems(strings.Join(block, "\n"))) == 0:
			out = append(out, block...)
		case !slices.ContainsFunc(block, notProse):
			out = append(out, fill(block)...)
		default:
			left = append(left, start+1)
			out = append(out, block...)
		}
	}
	return strings.Join(out, "\n"), left
}

// listItem starts a line of a list: "- ", "* ", "+ " or "1. ".
var listItem = regexp.MustCompile(`^([-*+]|\d+\.) `)

// notProse is whether line is part of a list, or indented further, as code is.
func notProse(line string) bool {
	after := strings.TrimPrefix(strings.TrimSpace(line), "// ")
	return strings.HasPrefix(after, " ") || listItem.MatchString(after)
}

// fill is block's words filled to width at its first line's indent, the last line taking
// a word from the one before rather than hold one alone.
func fill(block []string) []string {
	prefix := block[0][:strings.Index(block[0], "//")] + "// "
	var rows [][]string
	for _, line := range block {
		for w := range strings.FieldsSeq(text(line)) {
			if n := len(rows); n > 0 && columns(prefix+strings.Join(rows[n-1], " ")+" "+w) <= width {
				rows[n-1] = append(rows[n-1], w)
				continue
			}
			rows = append(rows, []string{w})
		}
	}
	if n := len(rows); n > 1 && len(rows[n-1]) == 1 && len(rows[n-2]) > 1 {
		k := len(rows[n-2]) - 1
		rows[n-1] = append([]string{rows[n-2][k]}, rows[n-1]...)
		rows[n-2] = rows[n-2][:k]
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = prefix + strings.Join(r, " ")
	}
	return out
}

// isComment is whether line is a whole-line comment a person wrapped: not a directive
// such as //go:build or //nolint.
func isComment(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "// ")
}

// text is line's comment without its marker.
func text(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "//"))
}

// columns is how many columns line takes.
func columns(line string) int {
	return len([]rune(strings.ReplaceAll(line, "\t", strings.Repeat(" ", tab))))
}
