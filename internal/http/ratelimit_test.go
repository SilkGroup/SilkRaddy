package http

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenBucketAllowsUpToCapacity(t *testing.T) {
	tb := newTokenBucket(3, 1)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !tb.take("k", now) {
			t.Fatalf("take %d rejected before capacity spent", i+1)
		}
	}
	if tb.take("k", now) {
		t.Errorf("take beyond capacity should have been rejected")
	}
}

func TestTokenBucketRefills(t *testing.T) {
	tb := newTokenBucket(2, 1) // 1 token / sec
	now := time.Now()
	tb.take("k", now)
	tb.take("k", now)
	if tb.take("k", now) {
		t.Fatalf("expected empty bucket to reject")
	}
	if !tb.take("k", now.Add(2*time.Second)) {
		t.Errorf("after 2s at 1/s refill, one token should be available")
	}
}

func TestTokenBucketIsPerKey(t *testing.T) {
	tb := newTokenBucket(1, 0.1)
	now := time.Now()
	if !tb.take("a", now) {
		t.Fatalf("first take for key a rejected")
	}
	if !tb.take("b", now) {
		t.Errorf("key b should have its own bucket")
	}
	if tb.take("a", now) {
		t.Errorf("key a should be empty")
	}
}

func TestClientIPHonoursXFF(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.5" {
		t.Errorf("clientIP = %q, want %q", got, "203.0.113.5")
	}
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "198.51.100.7:54321"
	if got := clientIP(r); got != "198.51.100.7" {
		t.Errorf("clientIP = %q, want %q", got, "198.51.100.7")
	}
}
