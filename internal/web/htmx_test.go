package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// htmx2Only are attributes and events of htmx 2 that htmx 4 renamed or
// dropped; under htmx 4 they silently do nothing. The htmx-guidance skill
// lists what replaces each.
var htmx2Only = map[string]*regexp.Regexp{
	"hx-disabled-elt (htmx 4: hx-disable)":            regexp.MustCompile(`\bhx-disabled-elt\b`),
	"hx-ext (htmx 4: include the extension's script)": regexp.MustCompile(`\bhx-ext\b`),
	"hx-vars (htmx 4: hx-vals with js:)":              regexp.MustCompile(`\bhx-vars\b`),
	"hx-request (htmx 4: hx-config)":                  regexp.MustCompile(`\bhx-request\b`),
	"sse-connect/sse-swap (htmx 4: hx-sse:connect)":   regexp.MustCompile(`\bsse-(connect|swap)\b`),
	"camelCase event (htmx 4: htmx:after:swap style)": regexp.MustCompile(`\bhtmx:[a-z]+[A-Z]`),
}

// htmxForm is a form that htmx submits.
var htmxForm = regexp.MustCompile(`(?s)<form\b[^>]*\bhx-(get|post|put|patch|delete)\b[^>]*>`)

// The vendored htmx is version 4: markup and scripts use its names, not
// htmx 2's. See docs/agents/coding-standards.md.
func TestHTMXUsesVersion4Names(t *testing.T) {
	for path, src := range ownFrontendSources(t) {
		for i, line := range strings.Split(src, "\n") {
			for name, re := range htmx2Only {
				if re.MatchString(line) {
					t.Errorf("%s line %d uses htmx 2's %s: %s", path, i+1, name, strings.TrimSpace(line))
				}
			}
		}
	}
}

// A form htmx submits still works without it: it keeps the method and
// action a plain browser posts with. See docs/agents/coding-standards.md.
func TestHTMXFormsWorkWithoutHTMX(t *testing.T) {
	for path, src := range ownFrontendSources(t) {
		for _, form := range htmxForm.FindAllString(src, -1) {
			if !strings.Contains(form, "method=") || !strings.Contains(form, "action=") {
				t.Errorf("%s has an htmx form without method and action: %s", path, form)
			}
		}
	}
}

// ownFrontendSources reads every template and every script of ours,
// leaving out the vendored, minified ones.
func ownFrontendSources(t *testing.T) map[string]string {
	sources := map[string]string{}
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if ext != ".templ" && (ext != ".js" || strings.HasSuffix(path, ".min.js")) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources[path] = string(src)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sources
}
