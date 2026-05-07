package api

import (
	"testing"
	"time"
)

func TestRateLimiter_StaleCleanup_DoesNotBlockRequests(t *testing.T) {
	rl := newRateLimiter(6000, 100) // very high rate — should never reject

	// Simulate many distinct IPs.
	for i := 0; i < 1000; i++ {
		ip := "10.0.0." + time.Now().Format("150405") + ":" + string(rune(i))
		rl.allow(ip)
	}

	// The current request should still be fast and not blocked.
	start := time.Now()
	allowed := rl.allow("fresh-ip")
	elapsed := time.Since(start)

	if !allowed {
		t.Error("fresh IP was unexpectedly rate-limited")
	}
	if elapsed > 10*time.Millisecond {
		t.Errorf("allow() took %v — cleanup may be too expensive", elapsed)
	}
}

func TestRateLimiter_StaleBucketsAreRemoved(t *testing.T) {
	rl := newRateLimiter(60, 1)

	rl.allow("stale-ip")

	// Manually age the bucket.
	rl.mu.Lock()
	b := rl.buckets["stale-ip"]
	b.lastSeen = time.Now().Add(-15 * time.Minute)
	rl.buckets["stale-ip"] = b
	rl.mu.Unlock()

	// Trigger cleanup.
	rl.cleanup()

	rl.mu.Lock()
	_, exists := rl.buckets["stale-ip"]
	rl.mu.Unlock()

	if exists {
		t.Error("stale bucket was not cleaned up")
	}
}
