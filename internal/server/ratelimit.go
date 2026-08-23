package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/fluxa/fluxa/internal/tenant"
	"golang.org/x/time/rate"
)

type rateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	rl := &rateLimiter{
		visitors: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rps),
		burst:    burst,
	}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.visitors[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[key] = limiter
	}
	return limiter
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for key, limiter := range rl.visitors {
			if limiter.TokensAt(time.Now()) == 0 {
				delete(rl.visitors, key)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit returns a middleware that limits requests per-tenant (or per-IP for unauthenticated).
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	globalLimiter := newRateLimiter(rps, burst)
	tenantLimiters := newRateLimiter(rps*10, burst*10) // 10x for authenticated tenants

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check global rate first
			if !globalLimiter.getLimiter("global").Allow() {
				http.Error(w, `{"error":{"code":"RATE_LIMITED","message":"global rate limit exceeded"}}`, http.StatusTooManyRequests)
				return
			}

			// Per-tenant or per-IP limiting
			tid := tenant.IDFromContext(r.Context())
			key := tid
			if key == "" {
				key = r.RemoteAddr
			}

			if !tenantLimiters.getLimiter(key).Allow() {
				http.Error(w, `{"error":{"code":"RATE_LIMITED","message":"rate limit exceeded"}}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
