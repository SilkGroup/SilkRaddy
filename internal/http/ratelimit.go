package http

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// tokenBucket implements a simple in-memory per-key rate limiter used to slow
// brute-force attempts against endpoints like /api/v1/login.
//
// Key is typically the client IP. The bucket refills at `refillPerSecond`
// up to `capacity` and each request costs one token. If the token cost
// cannot be paid, the handler returns 429 with a Retry-After header.
//
// This is a good-enough baseline. Production should replace with a
// distributed limiter (Redis / Memorystore) once multi-instance deploys
// land in Phase H.
type tokenBucket struct {
	mu              sync.Mutex
	buckets         map[string]*bucket
	capacity        float64
	refillPerSecond float64
	lastGC          time.Time
}

type bucket struct {
	tokens   float64
	lastFill time.Time
}

func newTokenBucket(capacity, refillPerSecond float64) *tokenBucket {
	return &tokenBucket{
		buckets:         make(map[string]*bucket),
		capacity:        capacity,
		refillPerSecond: refillPerSecond,
		lastGC:          time.Now(),
	}
}

// take attempts to spend one token for the key. Returns true when the
// request is allowed; false when it should be rejected. now is injectable
// for tests.
func (tb *tokenBucket) take(key string, now time.Time) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Periodic GC — every 5 minutes drop empty buckets so long-running
	// processes don't leak memory when many one-shot IPs hit us.
	if now.Sub(tb.lastGC) > 5*time.Minute {
		for k, b := range tb.buckets {
			if b.tokens >= tb.capacity-0.001 {
				delete(tb.buckets, k)
			}
		}
		tb.lastGC = now
	}

	b, ok := tb.buckets[key]
	if !ok {
		b = &bucket{tokens: tb.capacity, lastFill: now}
		tb.buckets[key] = b
	}
	elapsed := now.Sub(b.lastFill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * tb.refillPerSecond
		if b.tokens > tb.capacity {
			b.tokens = tb.capacity
		}
		b.lastFill = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// clientIP returns the best-effort client IP for r. If the server is behind
// a trusted proxy (Cloud Run, a CDN), it honours the leftmost X-Forwarded-For
// entry; otherwise falls back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := indexOf(xff, ','); comma > 0 {
			return trimSpace(xff[:comma])
		}
		return trimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func indexOf(s string, ch byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ch {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// rateLimit wraps h and rejects requests that exceed the bucket. keyFunc
// picks the per-request bucket key (e.g. clientIP). Rejections carry a
// Retry-After header so browsers back off.
func (s *Server) rateLimit(next http.Handler, tb *tokenBucket, keyFunc func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !tb.take(keyFunc(r), time.Now()) {
			w.Header().Set("Retry-After", "60")
			jsonMessage(w, http.StatusTooManyRequests, "Too many requests. Slow down.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
