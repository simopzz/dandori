package app_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
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

// A signed-in request whose session has expired renews it with Supabase Auth: a
// client span under the request's.
func TestRenewingASessionIsASpanUnderTheRequest(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	h.Clock.Set(h.Clock.Now().Add(2 * time.Hour))
	spans.Reset()

	h.Get(t, "/profile")

	request := onlySpan(t, spans.GetSpans(), "server", func(s tracetest.SpanStub) bool {
		return s.SpanKind == trace.SpanKindServer
	})
	renew := onlySpan(t, spans.GetSpans(), "supabase renew session", func(s tracetest.SpanStub) bool {
		return s.Name == "supabase renew session"
	})
	if renew.SpanKind != trace.SpanKindClient || renew.Parent.SpanID() != request.SpanContext.SpanID() {
		t.Errorf("supabase renew session is a %s span under %s, want a client span under the request's", renew.SpanKind, renew.Parent.SpanID())
	}
}

// A fresh server fetches the JWKS from Supabase Auth to check the first access token it
// sees: a client span under that request's.
func TestFetchingTheJWKSIsASpanUnderTheFirstCheckedRequest(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")

	h.SignUp(t, "ada@example.com")

	fetch := onlySpan(t, spans.GetSpans(), "supabase fetch JWKS", func(s tracetest.SpanStub) bool {
		return s.Name == "supabase fetch JWKS"
	})
	parent := onlySpan(t, spans.GetSpans(), "parent", func(s tracetest.SpanStub) bool {
		return s.SpanContext.SpanID() == fetch.Parent.SpanID()
	})
	if fetch.SpanKind != trace.SpanKindClient || parent.SpanKind != trace.SpanKindServer {
		t.Errorf("supabase fetch JWKS is a %s span under %q, want a client span under a request's", fetch.SpanKind, parent.Name)
	}
}

// No span of a call to Supabase Auth holds a secret: the access or refresh
// tokens, the API key, or the User's email.
func TestSupabaseSpansHoldNoSecret(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	secrets := []string{"ada@example.com", "sb_publishable_test"}
	keepTokens := func() {
		for _, c := range h.Client.Jar.Cookies(mustParse(t, h.URL)) {
			if c.Name == "dandori_access" || c.Name == "dandori_refresh" {
				secrets = append(secrets, c.Value)
			}
		}
	}
	keepTokens()
	h.Clock.Set(h.Clock.Now().Add(2 * time.Hour))
	profile := h.Get(t, "/profile") // renews the session
	keepTokens()
	form := apptest.Form(t, profile.Body, "sign-out")
	h.PostForm(t, form.Action, form.Values)

	var called []string
	for _, s := range spans.GetSpans() {
		if !strings.HasPrefix(s.Name, "supabase ") {
			continue
		}
		called = append(called, s.Name)
		recorded := s.Name + " " + s.Status.Description
		for _, a := range s.Attributes {
			recorded += " " + a.Value.String()
		}
		for _, e := range s.Events {
			for _, a := range e.Attributes {
				recorded += " " + a.Value.String()
			}
		}
		for _, secret := range secrets {
			if secret != "" && strings.Contains(recorded, secret) {
				t.Errorf("%s span holds the secret %q: %s", s.Name, secret, recorded)
			}
		}
	}
	want := []string{"supabase send magic link", "supabase verify link", "supabase fetch JWKS", "supabase renew session", "supabase sign out"}
	for _, name := range want {
		if !slices.Contains(called, name) {
			t.Errorf("no %s span to check; supabase spans: %v", name, called)
		}
	}
}

// Supabase Auth is not told the spans' ids: no call to it sends a traceparent
// header, even with a propagator set for the whole process.
func TestSupabaseIsSentNoTraceparent(t *testing.T) {
	was := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(was) })
	h, _ := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	h.Clock.Set(h.Clock.Now().Add(2 * time.Hour))
	h.Get(t, "/profile")

	if got := h.Auth.Traceparents(); len(got) > 0 {
		t.Errorf("the fake Supabase Auth was sent traceparent headers: %v", got)
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

// hasQueryUnder reports whether a child of parent is a query's span: a client's,
// but not a call to Supabase Auth.
func hasQueryUnder(spans tracetest.SpanStubs, parent tracetest.SpanStub) bool {
	for _, s := range spans {
		if s.Parent.SpanID() == parent.SpanContext.SpanID() && s.SpanKind == trace.SpanKindClient && !strings.HasPrefix(s.Name, "supabase ") {
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
