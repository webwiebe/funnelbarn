package tracing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// InitLogs ships structured logs to SpanBarn via OTLP and returns an slog.Handler
// that emits to it (trace-correlated via each record's context) together with a
// shutdown func. It returns (nil, nil-safe shutdown, nil) when SpanBarn isn't
// configured, so the caller can simply skip adding the handler.
func InitLogs(ctx context.Context, cfg Config) (slog.Handler, func(context.Context) error, error) {
	if cfg.Endpoint == "" || cfg.APIKey == "" {
		return nil, func(context.Context) error { return nil }, nil
	}

	exporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(signalURL(cfg.Endpoint, "logs")),
		otlploghttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.APIKey,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create log exporter: %w", err)
	}

	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create resource: %w", err)
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	handler := otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lp))

	return handler, func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return lp.Shutdown(shutdownCtx)
	}, nil
}
