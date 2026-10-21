package app_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/mealops/internal/apptest"
)

func TestAppShellLinksAnInstallableManifest(t *testing.T) {
	h := apptest.Start(t)

	page := h.Get(t, "/")
	if page.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", page.StatusCode)
	}
	if !strings.Contains(page.Body, `<link rel="manifest" href="/manifest.webmanifest"`) {
		t.Errorf("app shell does not link the manifest:\n%s", page.Body)
	}
	if !strings.Contains(page.Body, "/static/htmx.min.js") {
		t.Errorf("app shell does not load HTMX:\n%s", page.Body)
	}

	res := h.Get(t, "/manifest.webmanifest")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET manifest status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/manifest+json" {
		t.Errorf("manifest Content-Type = %q", ct)
	}
	var manifest struct {
		Name     string `json:"name"`
		StartURL string `json:"start_url"`
		Display  string `json:"display"`
		Icons    []struct {
			Src   string `json:"src"`
			Sizes string `json:"sizes"`
		} `json:"icons"`
	}
	if err := json.Unmarshal([]byte(res.Body), &manifest); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if manifest.Name != "MealOps" || manifest.StartURL != "/" || manifest.Display != "standalone" {
		t.Errorf("manifest = %+v", manifest)
	}
	sizes := map[string]bool{}
	for _, icon := range manifest.Icons {
		sizes[icon.Sizes] = true
		if got := h.Get(t, icon.Src); got.StatusCode != http.StatusOK {
			t.Errorf("icon %s status = %d", icon.Src, got.StatusCode)
		}
	}
	if !sizes["192x192"] || !sizes["512x512"] {
		t.Errorf("manifest icons %v lack 192x192 and 512x512", sizes)
	}

	if sw := h.Get(t, "/sw.js"); sw.StatusCode != http.StatusOK {
		t.Errorf("service worker status = %d", sw.StatusCode)
	}
}
