package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/mealops/internal/apptest"
)

func TestUnreachableDatabaseShowsRecoverableErrorAndRecovers(t *testing.T) {
	h := apptest.Start(t)
	if res := h.Get(t, "/"); res.StatusCode != http.StatusOK {
		t.Fatalf("GET / before cut status = %d, want 200", res.StatusCode)
	}

	h.CutDatabase()

	res := h.Get(t, "/")
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET / while unreachable status = %d, want 503", res.StatusCode)
	}
	if !strings.Contains(res.Body, "Database unavailable") || !strings.Contains(res.Body, "Nothing has been lost") {
		t.Errorf("error page does not explain the database is unavailable:\n%s", res.Body)
	}
	if !strings.Contains(res.Body, `hx-get="/healthz"`) {
		t.Errorf("error page does not poll for recovery:\n%s", res.Body)
	}
	if health := h.Get(t, "/healthz"); health.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("GET /healthz while unreachable status = %d, want 503", health.StatusCode)
	}

	h.RestoreDatabase()

	if res := h.Get(t, "/"); res.StatusCode != http.StatusOK {
		t.Fatalf("GET / after restore status = %d, want 200:\n%s", res.StatusCode, res.Body)
	}
}

func TestErrorPageReloadsOnceDatabaseIsBack(t *testing.T) {
	h := apptest.Start(t)

	req, err := http.NewRequest(http.MethodGet, h.URL+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("HX-Request", "true")
	res, err := h.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("HX-Refresh") != "true" {
		t.Errorf("HTMX health poll = %d, HX-Refresh %q; want 200 and a refresh", res.StatusCode, res.Header.Get("HX-Refresh"))
	}
}

func TestAppStartsWhileDatabaseIsUnreachable(t *testing.T) {
	h := apptest.Start(t, apptest.WithDatabaseCut())

	if res := h.Get(t, "/"); res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET / status = %d, want 503", res.StatusCode)
	}

	h.RestoreDatabase()

	if res := h.Get(t, "/"); res.StatusCode != http.StatusOK {
		t.Fatalf("GET / after restore status = %d, want 200:\n%s", res.StatusCode, res.Body)
	}
}
