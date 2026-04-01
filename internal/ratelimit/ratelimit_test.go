package ratelimit

import (
	"testing"
	"time"
)

func TestAllow_UnderLimit(t *testing.T) {
	l := New(Config{RequestsPerMinute: 5})
	for i := 0; i < 5; i++ {
		allowed, retryAfter := l.Allow()
		if !allowed {
			t.Errorf("request %d: expected allowed=true, got false", i+1)
		}
		if retryAfter != 0 {
			t.Errorf("request %d: expected retryAfter=0, got %v", i+1, retryAfter)
		}
	}
}

func TestAllow_AtLimit(t *testing.T) {
	l := New(Config{RequestsPerMinute: 2})
	// Use up the limit
	l.Allow()
	l.Allow()
	// Next request should be denied
	allowed, retryAfter := l.Allow()
	if allowed {
		t.Error("expected allowed=false at limit, got true")
	}
	if retryAfter <= 0 {
		t.Errorf("expected retryAfter > 0, got %v", retryAfter)
	}
}

func TestAllow_Unlimited(t *testing.T) {
	l := New(Config{RequestsPerMinute: 0})
	// Should always be allowed
	for i := 0; i < 100; i++ {
		allowed, retryAfter := l.Allow()
		if !allowed {
			t.Errorf("request %d: expected allowed=true (unlimited), got false", i+1)
		}
		if retryAfter != 0 {
			t.Errorf("request %d: expected retryAfter=0, got %v", i+1, retryAfter)
		}
	}
}

func TestAcquire_UnderLimit(t *testing.T) {
	l := New(Config{ConcurrentSessions: 3})
	for i := 0; i < 3; i++ {
		if !l.Acquire() {
			t.Errorf("slot %d: expected acquire=true, got false", i+1)
		}
	}
}

func TestAcquire_AtLimit(t *testing.T) {
	l := New(Config{ConcurrentSessions: 2})
	l.Acquire()
	l.Acquire()
	// Should fail immediately
	if l.Acquire() {
		t.Error("expected acquire=false at limit, got true")
	}
}

func TestAcquire_ReleaseFreesSlot(t *testing.T) {
	l := New(Config{ConcurrentSessions: 1})
	if !l.Acquire() {
		t.Fatal("first acquire should succeed")
	}
	if l.Acquire() {
		t.Error("second acquire should fail before release")
	}
	l.Release()
	if !l.Acquire() {
		t.Error("acquire should succeed after release")
	}
}

func TestAcquire_Unlimited(t *testing.T) {
	l := New(Config{ConcurrentSessions: 0})
	// Should always succeed
	for i := 0; i < 100; i++ {
		if !l.Acquire() {
			t.Errorf("acquire %d: expected true (unlimited), got false", i+1)
		}
	}
	// Release does nothing when unlimited, but should not panic
	l.Release()
}
