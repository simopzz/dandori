package web

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// formTag is a form's opening tag, however its attributes are laid out.
var formTag = regexp.MustCompile(`(?s)<form\b[^>]*>`)

// Every form leaves validation to the server, so a User sees only its refusal, in their
// Language, never the browser's bubble. See docs/design.md.
func TestFormsShowOnlyTheAppsErrors(t *testing.T) {
	for path, src := range ownFrontendSources(t) {
		if filepath.Ext(path) != ".templ" {
			continue
		}
		for _, form := range formTag.FindAllString(src, -1) {
			if !strings.Contains(form, "novalidate") {
				t.Errorf("%s has a form without novalidate: %s", path, form)
			}
		}
	}
}
