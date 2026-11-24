package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// The harness that starts the app for tests lives in apptest, which the
// production binary must never link: it brings the test database with it.
func TestTheBinaryDoesNotLinkTheTestHarness(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for dep := range strings.FieldsSeq(string(out)) {
		if dep == "github.com/simopzz/dandori/internal/apptest" {
			t.Fatalf("cmd/dandori depends on %s, which starts the app for tests", dep)
		}
	}
}

// The dev routes are mounted only in dev mode: without it, a request for
// one reaches the app, which has no such route.
func TestTheDevRoutesAreMountedOnlyInDevMode(t *testing.T) {
	asked := ""
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked = r.URL.Path })
	handler(app, nil).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/_test/clock", nil))
	if asked != "/_test/clock" {
		t.Errorf("the request went to %q, want the app's own handler", asked)
	}
}
