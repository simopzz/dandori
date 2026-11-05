// Package telemetry sends the server's spans and logs over OTLP to wherever
// OTEL_EXPORTER_OTLP_ENDPOINT points, with OTEL_EXPORTER_OTLP_HEADERS' token: Grafana
// Cloud in production. It is ops-level: what the server did, beside the Kitchen Ledger's
// record of what happened in the kitchen. Without an endpoint, as locally and in tests,
// nothing is exported.
package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Telemetry is where the app's spans and logs go.
type Telemetry struct {
	// Spans makes the spans; without an endpoint, it makes none.
	Spans trace.TracerProvider
	// Logs takes the log records to send; nil without an endpoint.
	Logs     log.LoggerProvider
	shutdown []func(context.Context) error
}

// Setup exports to the endpoint the environment sets, if any. The OTLP exporters read it
// and its headers themselves.
func Setup(ctx context.Context) (*Telemetry, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return &Telemetry{Spans: noop.NewTracerProvider()}, nil
	}
	// The environment's OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES, last, win.
	res, err := resource.New(ctx, resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.name", "dandori")), resource.WithFromEnv())
	if err != nil {
		return nil, err
	}
	spans, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	records, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(spans), sdktrace.WithResource(res))
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(records)), sdklog.WithResource(res))
	return &Telemetry{Spans: tp, Logs: lp, shutdown: []func(context.Context) error{tp.Shutdown, lp.Shutdown}}, nil
}

// Shutdown sends what is still buffered and stops exporting.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for _, shutdown := range t.shutdown {
		errs = append(errs, shutdown(ctx))
	}
	return errors.Join(errs...)
}

// Logger logs as text to w and, with logs, over OTLP too. A line logged with a span's
// context carries the span's ids, trace_id and span_id, so it links to the span.
func Logger(w io.Writer, logs log.LoggerProvider) *slog.Logger {
	var h slog.Handler = withIDs{slog.NewTextHandler(w, nil)}
	if logs != nil {
		h = slog.NewMultiHandler(h, otelslog.NewHandler("github.com/simopzz/dandori", otelslog.WithLoggerProvider(logs)))
	}
	return slog.New(h)
}

// withIDs adds the ids of the context's span to each record.
type withIDs struct{ slog.Handler }

func (h withIDs) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r = r.Clone()
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h withIDs) WithAttrs(attrs []slog.Attr) slog.Handler {
	return withIDs{h.Handler.WithAttrs(attrs)}
}

func (h withIDs) WithGroup(name string) slog.Handler { return withIDs{h.Handler.WithGroup(name)} }
