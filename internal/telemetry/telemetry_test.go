package telemetry_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/simopzz/dandori/internal/telemetry"
)

// A line logged inside a span names its trace and span, both on stderr and in the record
// sent over OTLP, so Grafana links it to the span.
func TestALogLineInASpanCarriesItsIDs(t *testing.T) {
	records := &recordExporter{}
	logs := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(records)))
	var stderr bytes.Buffer
	log := telemetry.Logger(&stderr, logs)
	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(t.Context(), "request")
	defer span.End()
	traceID, spanID := span.SpanContext().TraceID(), span.SpanContext().SpanID()

	log.ErrorContext(ctx, "render", "err", "boom")

	line := stderr.String()
	if !strings.Contains(line, "trace_id="+traceID.String()) || !strings.Contains(line, "span_id="+spanID.String()) {
		t.Errorf("stderr = %q, want trace_id=%s and span_id=%s", line, traceID, spanID)
	}
	got := records.all()
	if len(got) != 1 {
		t.Fatalf("exported %d records, want 1", len(got))
	}
	if got[0].TraceID() != traceID || got[0].SpanID() != spanID {
		t.Errorf("record carries trace %s span %s, want %s %s", got[0].TraceID(), got[0].SpanID(), traceID, spanID)
	}
}

// A line logged outside any span is logged as before, without ids.
func TestALogLineOutsideASpanHasNoIDs(t *testing.T) {
	var stderr bytes.Buffer
	telemetry.Logger(&stderr, nil).Info("serving", "addr", "127.0.0.1:8080")

	if line := stderr.String(); strings.Contains(line, "trace_id") || !strings.Contains(line, "msg=serving") {
		t.Errorf("stderr = %q, want the line without ids", line)
	}
}

// Locally and in tests no endpoint is set: nothing is exported, and nothing fails.
func TestWithoutAnEndpointNothingIsExported(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	tel, err := telemetry.Setup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tel.Logs != nil {
		t.Error("Setup made a log provider without an endpoint")
	}
	_, span := tel.Tracing.Tracer("test").Start(t.Context(), "request")
	if span.IsRecording() {
		t.Error("Setup records spans without an endpoint")
	}
	if err := tel.Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

// With an endpoint and its token set, as in production, spans and logs are sent there
// over OTLP with the token, by the time Shutdown returns.
func TestWithAnEndpointSpansAndLogsAreSentThere(t *testing.T) {
	collector := &fakeCollector{}
	srv := httptest.NewServer(collector)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL+"/otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Basic%20dG9rZW4=")

	tel, err := telemetry.Setup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := tel.Tracing.Tracer("test").Start(t.Context(), "request")
	telemetry.Logger(&bytes.Buffer{}, tel.Logs).InfoContext(ctx, "hello")
	span.End()
	if err := tel.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/otlp/v1/traces", "/otlp/v1/logs"} {
		if auth, ok := collector.received(path); !ok {
			t.Errorf("nothing was sent to %s; received %v", path, collector.paths())
		} else if auth != "Basic dG9rZW4=" {
			t.Errorf("%s was sent with Authorization %q, want the token", path, auth)
		}
	}
}

// recordExporter keeps the log records exported to it.
type recordExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *recordExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range records {
		e.records = append(e.records, r.Clone())
	}
	return nil
}

func (e *recordExporter) Shutdown(context.Context) error   { return nil }
func (e *recordExporter) ForceFlush(context.Context) error { return nil }

func (e *recordExporter) all() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.records
}

// fakeCollector stands in for Grafana Cloud's OTLP endpoint: it accepts every export and
// keeps the Authorization each path was sent with.
type fakeCollector struct {
	mu   sync.Mutex
	auth map[string]string
}

func (c *fakeCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.auth == nil {
		c.auth = map[string]string{}
	}
	c.auth[r.URL.Path] = r.Header.Get("Authorization")
	w.Header().Set("Content-Type", "application/x-protobuf")
}

func (c *fakeCollector) received(path string) (auth string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	auth, ok = c.auth[path]
	return auth, ok
}

func (c *fakeCollector) paths() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var p []string
	for path := range c.auth {
		p = append(p, path)
	}
	return p
}
