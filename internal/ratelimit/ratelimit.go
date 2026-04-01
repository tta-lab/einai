package ratelimit

import (
	"sync"
	"time"
)

// Config configures the rate limiter.
type Config struct {
	RequestsPerMinute  int
	ConcurrentSessions int
}

// Limiter implements rate limiting with a sliding window for requests
// and a semaphore for concurrent sessions.
type Limiter struct {
	requestsPerMinute  int
	concurrentSessions int
	mu                 sync.Mutex
	recentRequests     []time.Time
	sessionSem         chan struct{}
}

// New creates a new Limiter with the given config.
func New(cfg Config) *Limiter {
	if cfg.RequestsPerMinute < 0 {
		panic("ratelimit: RequestsPerMinute must be non-negative")
	}
	if cfg.ConcurrentSessions < 0 {
		panic("ratelimit: ConcurrentSessions must be non-negative")
	}
	l := &Limiter{
		requestsPerMinute:  cfg.RequestsPerMinute,
		concurrentSessions: cfg.ConcurrentSessions,
	}
	if cfg.ConcurrentSessions > 0 {
		l.sessionSem = make(chan struct{}, cfg.ConcurrentSessions)
	}
	return l
}

// Allow checks if a new request is allowed.
// Returns (allowed bool, retryAfter time.Duration).
// retryAfter is 0 if allowed, or the time until the next slot opens.
func (l *Limiter) Allow() (bool, time.Duration) {
	if l.requestsPerMinute == 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	window := now.Add(-time.Minute)

	// Evict old entries
	for len(l.recentRequests) > 0 && l.recentRequests[0].Before(window) {
		l.recentRequests = l.recentRequests[1:]
	}

	if len(l.recentRequests) < l.requestsPerMinute {
		l.recentRequests = append(l.recentRequests, now)
		return true, 0
	}

	// Calculate retry after — time until oldest request falls out of window
	oldest := l.recentRequests[0]
	retryAfter := oldest.Add(time.Minute).Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return false, retryAfter
}

// Acquire acquires a concurrent session slot.
// Returns false immediately if ConcurrentSessions limit is reached.
func (l *Limiter) Acquire() bool {
	if l.concurrentSessions == 0 {
		return true
	}
	select {
	case l.sessionSem <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release releases a concurrent session slot.
func (l *Limiter) Release() {
	if l.concurrentSessions == 0 {
		return
	}
	select {
	case <-l.sessionSem:
	default:
	}
}
