package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	buckets sync.Map
	rate    int
	window  time.Duration
}

type rlBucket struct {
	mu    sync.Mutex
	count int
	reset time.Time
}

func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{rate: rate, window: window}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	v, _ := rl.buckets.LoadOrStore(ip, &rlBucket{reset: time.Now().Add(rl.window)})
	b := v.(*rlBucket)
	b.mu.Lock()
	defer b.mu.Unlock()
	if now := time.Now(); now.After(b.reset) {
		b.count = 0
		b.reset = now.Add(rl.window)
	}
	if b.count >= rl.rate {
		return false
	}
	b.count++
	return true
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.buckets.Range(func(k, v any) bool {
			b := v.(*rlBucket)
			b.mu.Lock()
			expired := time.Now().After(b.reset)
			b.mu.Unlock()
			if expired {
				rl.buckets.Delete(k)
			}
			return true
		})
	}
}

func (rl *rateLimiter) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}
