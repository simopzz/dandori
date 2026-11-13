package app_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/telemetry"
)

// collector stands in for Grafana Cloud's Faro collector.
const collector = "https://faro.example/collect/key"

// With a Faro url, the pages send the collector what the browser saw: the User's id, the
// page's lang, whether it was translated, and each htmx request with its server span's
// trace id. A query string never leaves.
func TestBrowserSendsFaroWhatThePageSaw(t *testing.T) {
	withSpans, spans := withSpans(t)
	h := apptest.Start(t, withSpans, apptest.WithConfig(func(c *app.Config) { c.FaroURL = collector }))
	h.Invite("ada@example.com")
	b := h.Browser(t)
	var (
		mu   sync.Mutex
		sent []string
	)
	err := b.Route(collector, func(r playwright.Route) {
		body, _ := r.Request().PostData()
		mu.Lock()
		sent = append(sent, body)
		mu.Unlock()
		err := r.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(202), Headers: map[string]string{
			"Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*",
		}})
		if err != nil {
			t.Logf("answer the collector's request: %v", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/kitchen?secret=ada")
	spans.Reset()

	// Its response, so the route has recorded it.
	_, err = b.ExpectResponse(func(r playwright.Response) bool {
		body, _ := r.Request().PostData()
		return strings.Contains(body, `"htmx request"`)
	}, func() error {
		return b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Recipes"}).Click()
	})
	if err != nil {
		t.Fatalf("no htmx request reached the collector: %v", err)
	}

	recipes := onlySpan(t, spans.GetSpans(), "GET /recipes", func(s tracetest.SpanStub) bool {
		return s.SpanKind == trace.SpanKindServer && s.Name == "GET /recipes"
	})
	user, _ := attr(recipes, telemetry.UserID)
	mu.Lock()
	all := strings.Join(sent, "\n")
	mu.Unlock()
	for _, want := range []string{
		`"trace_id":"` + recipes.SpanContext.TraceID().String() + `"`,
		`"path":"/recipes"`,
		`"user":{"id":"` + user.AsString() + `"}`,
		`"lang":"en"`,
		`"translated":"false"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("the collector was not sent %s; it got:\n%s", want, all)
		}
	}
	if strings.Contains(all, "secret") {
		t.Errorf("the collector was sent a query string:\n%s", all)
	}
}

// Without a Faro url, as in `mise run dev` and every other test, the pages load no Faro.
func TestBrowserLoadsNoFaroWithoutItsURL(t *testing.T) {
	h := apptest.Start(t)
	b := h.Browser(t)

	b.Open(t, "/sign-in")

	if n, err := b.Locator(`script[src*="faro"]`).Count(); err != nil || n != 0 {
		t.Errorf("the page has %d Faro scripts (%v), want none", n, err)
	}
	if loaded, err := b.Evaluate(`typeof GrafanaFaroWebSdk !== "undefined"`); err != nil || loaded != false {
		t.Errorf("Faro loaded (%v, %v), want not", loaded, err)
	}
}
