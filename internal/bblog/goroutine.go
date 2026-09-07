package bblog

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Go runs fn in a new goroutine with panic recovery. A panic is logged at
// error level with the panic value and a stack trace instead of taking down
// the process — background workers and fire-and-forget goroutines (health
// pings, periodic cleanup) have no caller to propagate a panic to, so an
// unrecovered one either kills the pod (a long-lived worker loop) or just
// silently ends the goroutine (a one-shot side effect), and either way the
// failure was never reported. Logging with an "err" attribute at error level
// routes it through Handler into BugBarn the same way any other error-level
// log record does, so it shows up in the dashboard instead of only stderr.
//
// name identifies the goroutine in the log line and in BugBarn attributes; it
// should be a short, stable label such as "background-worker" or
// "recordings-health", not something that varies per call.
func Go(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in background goroutine",
					"goroutine", name,
					"err", fmt.Errorf("panic: %v", r),
					"stack", string(debug.Stack()),
					"handled", true,
				)
			}
		}()
		fn()
	}()
}
