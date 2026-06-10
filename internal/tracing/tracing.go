package tracing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func Tracer() trace.Tracer {
	return tracer
}

type Config struct {
	Endpoint    string
	APIKey      string
	ServiceName string
	Version     string
	Environment string
}

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.Endpoint == "" || cfg.APIKey == "" {
		tracer = otel.Tracer(cfg.ServiceName)
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.Endpoint),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.APIKey,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Version),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	tracer = tp.Tracer(cfg.ServiceName)

	return func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(shutdownCtx)
	}, nil
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tracer == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Provisional name until the mux resolves the route pattern.
		ctx, span := tracer.Start(r.Context(), r.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethod(r.Method),
				semconv.HTTPTarget(r.URL.Path),
				semconv.HTTPScheme("https"),
				attribute.String("http.query", r.URL.RawQuery),
			),
		)
		defer span.End()

		rw := &statusWriter{ResponseWriter: w}
		req := r.WithContext(ctx)
		next.ServeHTTP(rw, req)

		// After routing, req.Pattern is the matched route template
		// (e.g. "GET /api/v1/projects/{id}/events") — low cardinality and
		// much better for grouping in trace search.
		if req.Pattern != "" {
			span.SetName(req.Pattern)
			span.SetAttributes(semconv.HTTPRoute(req.Pattern))
		} else {
			span.SetName(r.Method + " " + r.URL.Path)
		}
		span.SetAttributes(semconv.HTTPStatusCode(rw.status))
		if rw.status >= 500 {
			// Record an error event in addition to the status so SpanBarn
			// surfaces it as an exception (handled=false 5xx) rather than
			// a silently-error-statused span. Per-handler code is expected
			// to call span.RecordError on the underlying err; this is the
			// envelope-level fallback for handlers that don't.
			err := fmt.Errorf("HTTP %d", rw.status)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func RecordError(span trace.Span, err error) {
	if span != nil && err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
