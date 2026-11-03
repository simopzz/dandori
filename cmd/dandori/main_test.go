package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The clock switch tests set the time through (POST /_test/clock) lives in
// apptest, which the production binary must never link: there, the time
// is the machine's.
func TestTheBinaryHasNoClockSwitch(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for dep := range strings.FieldsSeq(string(out)) {
		if dep == "github.com/simopzz/dandori/internal/apptest" {
			t.Fatalf("cmd/dandori depends on %s, which serves the test-only clock switch", dep)
		}
	}
}
