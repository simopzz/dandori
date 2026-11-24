package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// A panicking handler is answered as a server error, as the bar holds for any failure:
// the span an error, the page quoting the trace id, and one logged line with the stack.
// The std log package would leave Grafana holding nothing of it.
func TestAPanickingHandlerIsAServerError(t *testing.T) {
	spans := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	logged := &bytes.Buffer{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, _ *http.Request) {
		panic("unknown Target zucchini")
	})
	handler := otelhttp.NewHandler(named(recovered(slog.New(slog.NewTextHandler(logged, nil)), mux)),
		"request", otelhttp.WithTracerProvider(tp))

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if res.Code != http.StatusInternalServerError {
		t.Errorf("the panic answered %d, want 500", res.Code)
	}
	if len(spans.GetSpans()) != 1 {
		t.Fatalf("%d spans, want the request's; spans: %v", len(spans.GetSpans()), spans.GetSpans())
	}
	request := spans.GetSpans()[0]
	if request.Name != "GET /boom" {
		t.Errorf("the request span is named %q, want %q: recovered must sit inside named", request.Name, "GET /boom")
	}
	if request.Status.Code != codes.Error {
		t.Errorf("the request span's status %s, want error", request.Status.Code)
	}
	if id := request.SpanContext.TraceID().String(); !strings.Contains(res.Body.String(), id) {
		t.Errorf("the error page %q doesn't show the trace id %s", res.Body.String(), id)
	}
	for _, want := range []string{"a handler panicked", "unknown Target zucchini", "goroutine"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("the logged line %q doesn't hold %q", logged.String(), want)
		}
	}
}
