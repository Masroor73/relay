// Rate limiting is applied per-instance (in-memory, keyed by client IP),
// not globally across instances — see ARCHITECTURE.md Section 10 and 8.
// This is a deliberate, documented limitation appropriate at portfolio
// scale, not an oversight.
package server

import (
	"log/slog"
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

const (
	rateLimitPerSecond = 5
	rateLimitBurst     = 10
)

type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func newIPRateLimiter() *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*rate.Limiter),
	}
}

func (rl *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rate.Limit(rateLimitPerSecond), rateLimitBurst)
		rl.limiters[ip] = limiter
	}
	return limiter
}

func rateLimitMiddleware(rl *ipRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			slog.Warn("rate limit exceeded", "event_id", "unknown", "ip", ip)
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}
