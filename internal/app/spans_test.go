package app_test

import (
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
)

// startWithSpans starts the app with its spans going to an in-memory exporter, in place
// of Grafana Cloud.
func startWithSpans(t *testing.T) (*apptest.Harness, *tracetest.InMemoryExporter) {
	t.Helper()
	spans := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	h := apptest.Start(t, apptest.WithConfig(func(c *app.Config) { c.Spans = tp }))
	return h, spans
}

// A request is a span named for its route, with a span for each database query it runs
// under it.
func TestARequestIsASpanWithItsQueriesUnderIt(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	spans.Reset()

	h.Get(t, "/profile")

	request := onlySpan(t, spans.GetSpans(), "server", func(s tracetest.SpanStub) bool {
		return s.SpanKind == trace.SpanKindServer
	})
	if request.Name != "GET /profile" {
		t.Errorf("request span is named %q, want %q", request.Name, "GET /profile")
	}
	if !hasQueryUnder(spans.GetSpans(), request) {
		t.Errorf("no query span under the request's span; spans: %s", names(spans.GetSpans()))
	}
}

// The container's health check asks every ten seconds: its requests are not spans.
func TestTheHealthCheckIsNotASpan(t *testing.T) {
	h, spans := startWithSpans(t)
	spans.Reset()

	h.Get(t, "/healthz")

	if got := spans.GetSpans(); len(got) > 0 {
		t.Errorf("GET /healthz made spans: %s", names(got))
	}
}

// A scheduler beat is a span, with the queries that bring each User's kitchen up to now
// under it.
func TestABeatIsASpanWithItsQueriesUnderIt(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	spans.Reset()

	h.Clock.Set(h.Clock.Now().Add(5 * time.Minute))

	beat := onlySpan(t, spans.GetSpans(), "kitchen beat", func(s tracetest.SpanStub) bool {
		return s.Name == "kitchen beat"
	})
	if !hasQueryUnder(spans.GetSpans(), beat) {
		t.Errorf("no query span under the beat's span; spans: %s", names(spans.GetSpans()))
	}
}

// onlySpan is the one span that matches, described as what; finding none or several fails
// the test.
func onlySpan(t *testing.T, spans tracetest.SpanStubs, what string, match func(tracetest.SpanStub) bool) tracetest.SpanStub {
	t.Helper()
	var found []tracetest.SpanStub
	for _, s := range spans {
		if match(s) {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("found %d %s spans, want 1; spans: %s", len(found), what, names(spans))
	}
	return found[0]
}

// hasQueryUnder reports whether a child of parent is a query's span: a client's.
func hasQueryUnder(spans tracetest.SpanStubs, parent tracetest.SpanStub) bool {
	for _, s := range spans {
		if s.Parent.SpanID() == parent.SpanContext.SpanID() && s.SpanKind == trace.SpanKindClient {
			return true
		}
	}
	return false
}

// names lists the spans' names, for a failure to show.
func names(spans tracetest.SpanStubs) string {
	var n []string
	for _, s := range spans {
		n = append(n, s.Name)
	}
	return strings.Join(n, ", ")
}
