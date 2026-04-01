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

// IsRetryable returns true if the error is a retryable 5xx error.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "500") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "529")
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

		// Calculate delay with exponential backoff and jitter
		jitter := time.Duration(rand.Int63n(500)) * time.Millisecond
		waitTime := delay + jitter

		msg := fmt.Sprintf("provider returned 5xx, retrying in %ds (attempt %d/%d)",
			int(waitTime.Seconds()), attempt+1, cfg.maxRetries)
		emitStatus(msg)

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
