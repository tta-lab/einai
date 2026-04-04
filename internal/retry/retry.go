// Package retry provides exponential backoff retry logic for LLM provider errors.
package retry

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// MaxRetries is the maximum number of retry attempts.
const MaxRetries = 3

// retryConfig holds retry configuration.
type retryConfig struct {
	maxRetries int
	baseDelay  time.Duration
}

// defaultConfig is the default retry configuration.
var defaultConfig = retryConfig{
	maxRetries: MaxRetries,
	baseDelay:  time.Second,
}

// IsRetryable returns true if the error is a retryable 5xx or 429 error.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "500") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "529")
}

// RetryAfterFromError extracts the Retry-After duration from an error message
// if the error contains "retry-after: <seconds>" (case-insensitive).
// Returns 0 if not present.
func RetryAfterFromError(err error) time.Duration {
	if err == nil {
		return 0
	}
	msg := strings.ToLower(err.Error())
	// Look for "retry-after: <n>" in the error message
	const needle = "retry-after: "
	idx := strings.Index(msg, needle)
	if idx < 0 {
		return 0
	}
	rest := msg[idx+len(needle):]
	// Read digits
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	var secs int
	for _, c := range rest[:end] {
		secs = secs*10 + int(c-'0')
	}
	return time.Duration(secs) * time.Second
}

// WithRetry executes fn with exponential backoff retry for retryable errors.
func WithRetry(ctx context.Context, emitStatus func(string), fn func() error) error {
	return withRetryConfig(ctx, defaultConfig, emitStatus, fn)
}

// withRetryConfig executes fn with custom retry configuration (used by tests).
func withRetryConfig(ctx context.Context, cfg retryConfig, emitStatus func(string), fn func() error) error {
	var lastErr error
	delay := cfg.baseDelay

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !IsRetryable(lastErr) {
			return lastErr
		}

		if attempt == cfg.maxRetries {
			break
		}

		// Use Retry-After header value if present (e.g. from 429 responses),
		// otherwise fall back to exponential backoff with jitter.
		var waitTime time.Duration
		if ra := RetryAfterFromError(lastErr); ra > 0 {
			waitTime = ra
			msg := fmt.Sprintf("rate limited, retrying in %ds (attempt %d/%d)",
				int(waitTime.Seconds()), attempt+1, cfg.maxRetries)
			emitStatus(msg)
		} else {
			jitter := time.Duration(rand.Int63n(500)) * time.Millisecond
			waitTime = delay + jitter
			msg := fmt.Sprintf("provider returned 5xx, retrying in %ds (attempt %d/%d)",
				int(waitTime.Seconds()), attempt+1, cfg.maxRetries)
			emitStatus(msg)
		}

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}

		// Double delay for next attempt
		delay *= 2
	}

	return lastErr
}
