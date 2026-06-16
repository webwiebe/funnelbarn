package tracing

import (
	"context"
	"fmt"
	"time"

	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// InitMetrics pushes the process's Prometheus metrics to SpanBarn via OTLP.
// It bridges the default Prometheus registry (every funnelbarn_* metric) into an
// OTEL MeterProvider with a periodic OTLP exporter, so no metric needs to be
// re-instrumented. It is a no-op (nil-safe shutdown) when SpanBarn isn't
// configured.
func InitMetrics(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.Endpoint == "" || cfg.APIKey == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(signalURL(cfg.Endpoint, "metrics")),
		otlpmetrichttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.APIKey,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	reader := sdkmetric.NewPeriodicReader(exporter,
		sdkmetric.WithInterval(60*time.Second),
		// Bridge the existing Prometheus default registry so all funnelbarn_*
		// metrics are exported without re-instrumentation.
		sdkmetric.WithProducer(prombridge.NewMetricProducer()),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return mp.Shutdown(shutdownCtx)
	}, nil
}
