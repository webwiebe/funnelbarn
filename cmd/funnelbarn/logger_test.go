package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
)

// discardHandler stands in for the SpanBarn OTLP handler.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h discardHandler) WithGroup(string) slog.Handler           { return h }

// Every pod log line used to appear twice: with BugBarn configured, the plain
// JSON handler and the BugBarn handler wrapping it were both in the fan-out.
func TestBuildLoggerWritesEachRecordOnce(t *testing.T) {
	cases := map[string]config.Config{
		"plain":            {LogLevel: slog.LevelInfo},
		"bugbarn":          {LogLevel: slog.LevelInfo, SelfEndpoint: "https://bugbarn.invalid", SelfAPIKey: "k"},
		"bugbarn+spanbarn": {LogLevel: slog.LevelInfo, SelfEndpoint: "https://bugbarn.invalid", SelfAPIKey: "k", SpanBarnLogLevel: slog.LevelInfo},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			var spanbarn slog.Handler
			if strings.Contains(name, "spanbarn") {
				spanbarn = discardHandler{}
			}
			logger := buildLogger(&out, cfg, spanbarn)

			logger.Info("one line", "k", "v")
			logger.With("req", "r1").Warn("another line")

			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != 2 {
				t.Fatalf("2 log calls wrote %d lines:\n%s", len(lines), out.String())
			}
		})
	}
}
