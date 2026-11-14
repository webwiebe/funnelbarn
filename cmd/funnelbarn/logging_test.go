package main

import (
	"log/slog"
	"testing"
	"time"
)

func TestIsHealthProbeLog(t *testing.T) {
	record := func(msg, userAgent string) slog.Record {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
		r.AddAttrs(slog.String("path", "/api/v1/health"), slog.String("user_agent", userAgent))
		return r
	}
	cases := []struct {
		name string
		rec  slog.Record
		want bool
	}{
		{"kube probe access log", record("request", "kube-probe/1.31"), true},
		{"browser access log", record("request", "Mozilla/5.0"), false},
		{"other message from a probe", record("startup", "kube-probe/1.31"), false},
		{"access log without user agent", slog.NewRecord(time.Now(), slog.LevelInfo, "request", 0), false},
	}
	for _, tc := range cases {
		if got := isHealthProbeLog(tc.rec); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
