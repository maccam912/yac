package yac

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// tracer is the package-level tracer used by yac internals.
var tracer = otel.Tracer("github.com/maccam912/yac")

// Tracer returns the OpenTelemetry tracer used by yac. This is useful
// for creating custom spans in user code that integrate with yac's
// trace hierarchy.
func Tracer() trace.Tracer {
	return tracer
}

// InitTracing sets up OpenTelemetry tracing with an OTLP exporter.
// It configures the global TracerProvider so all yac operations are
// automatically traced.
//
// The exporter reads standard OTel environment variables:
//   - OTEL_EXPORTER_OTLP_ENDPOINT (e.g. "http://localhost:4318")
//   - OTEL_EXPORTER_OTLP_HEADERS
//   - OTEL_EXPORTER_OTLP_PROTOCOL ("grpc" or "http/protobuf")
//
// By default, the HTTP exporter is used. Set protocol to "grpc" to
// use the gRPC exporter instead.
//
// Returns a shutdown function that must be called on application exit
// to flush pending spans.
//
// Example:
//
//	shutdown, err := yac.InitTracing(ctx, "my-agent-app")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer shutdown(ctx)
func InitTracing(ctx context.Context, serviceName string, opts ...TracingOption) (func(context.Context) error, error) {
	cfg := &tracingConfig{protocol: "http"}
	for _, opt := range opts {
		opt(cfg)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	var exporter sdktrace.SpanExporter
	switch cfg.protocol {
	case "grpc":
		exporter, err = otlptracegrpc.New(ctx)
	default:
		exporter, err = otlptracehttp.New(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("create exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// Update the package-level tracer to use the new provider.
	tracer = tp.Tracer("github.com/maccam912/yac")

	return tp.Shutdown, nil
}

// tracingConfig holds configuration for InitTracing.
type tracingConfig struct {
	protocol string
}

// TracingOption configures InitTracing.
type TracingOption func(*tracingConfig)

// WithGRPC configures the OTLP exporter to use gRPC instead of HTTP.
func WithGRPC() TracingOption {
	return func(c *tracingConfig) {
		c.protocol = "grpc"
	}
}
