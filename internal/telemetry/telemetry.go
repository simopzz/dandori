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

	"github.com/simopzz/dandori/internal/db"
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
	res, err := Resource(ctx)
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

// Version is the commit the binary was built from, which the image's build sets with
// -ldflags "-X github.com/simopzz/dandori/internal/telemetry.Version=<sha>".
var Version = "dev"

// Resource describes the server every span and log comes from: dandori, at Version. The
// environment's OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES, last, win.
func Resource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx, resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.name", "dandori"), attribute.String("service.version", Version)),
		resource.WithFromEnv())
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
// context carries the span's ids, trace_id and span_id, on w, so it links to the span;
// the OTLP record carries them as its own. A line logged in a signed-in User's request
// carries their id, dandori.user.id, on both.
func Logger(w io.Writer, logs log.LoggerProvider) *slog.Logger {
	var h slog.Handler = adding{slog.NewTextHandler(w, nil), spanIDs}
	if logs != nil {
		h = slog.NewMultiHandler(h, otelslog.NewHandler("github.com/simopzz/dandori", otelslog.WithLoggerProvider(logs)))
	}
	return slog.New(adding{h, user})
}

func spanIDs(ctx context.Context) []slog.Attr {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		return []slog.Attr{slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String())}
	}
	return nil
}

func user(ctx context.Context) []slog.Attr {
	if id, ok := db.UserID(ctx); ok {
		return []slog.Attr{slog.String("dandori.user.id", id)}
	}
	return nil
}

// adding adds the attributes attrs reads from its context to each record.
type adding struct {
	slog.Handler
	attrs func(context.Context) []slog.Attr
}

func (h adding) Handle(ctx context.Context, r slog.Record) error {
	if attrs := h.attrs(ctx); len(attrs) > 0 {
		r = r.Clone()
		r.AddAttrs(attrs...)
	}
	return h.Handler.Handle(ctx, r)
}

func (h adding) WithAttrs(attrs []slog.Attr) slog.Handler {
	return adding{h.Handler.WithAttrs(attrs), h.attrs}
}

func (h adding) WithGroup(name string) slog.Handler {
	return adding{h.Handler.WithGroup(name), h.attrs}
}
