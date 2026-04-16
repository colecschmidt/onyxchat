package http

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// InitTracer sets up the OpenTelemetry trace provider.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is set (e.g. "tempo:4317"), spans are
// exported via OTLP gRPC. Otherwise a no-op provider is used so the binary
// runs fine in prod without a collector configured.
//
// Returns a shutdown function that must be called on server exit to flush
// any buffered spans.
func InitTracer(log *zap.Logger) func(context.Context) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		log.Info("tracing disabled (OTEL_EXPORTER_OTLP_ENDPOINT not set)")
		return func(context.Context) {}
	}

	ctx := context.Background()

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		log.Error("failed to create OTLP exporter, tracing disabled", zap.Error(err))
		return func(context.Context) {}
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("onyxchat-server"),
		),
	)
	if err != nil {
		log.Warn("failed to create OTel resource", zap.Error(err))
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)

	log.Info("tracing enabled", zap.String("endpoint", endpoint))

	return func(ctx context.Context) {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Error("tracer shutdown error", zap.Error(err))
		}
	}
}
