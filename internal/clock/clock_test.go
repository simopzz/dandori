package clock

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readsTheTime is a call that reads the machine's time instead of the
// app's Clock.
var readsTheTime = regexp.MustCompile(`\btime\.(Now|Since|Until)\(`)

// Time is an input (ADR-0014): nothing outside this package reads the
// machine's time, in Go code, tests or templates, so tests can set and
// step it. Take a Clock instead.
func TestOnlyTheClockReadsTheTime(t *testing.T) {
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == filepath.Join("..", "..", "internal", "clock") || d.Name() == ".git" {
				return filepath.SkipDir // this package reads the time
			}
			return nil
		}
		if ext := filepath.Ext(path); ext != ".go" && ext != ".templ" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if readsTheTime.MatchString(line) {
				t.Errorf("%s line %d reads the machine's time; take a clock.Clock: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
