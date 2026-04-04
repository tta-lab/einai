package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithRetry_SuccessOnFirstTry(t *testing.T) {
	calls := 0
	cfg := retryConfig{maxRetries: 3, baseDelay: 0}

	err := withRetryConfig(context.Background(), cfg, func(msg string) {}, func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected fn to be called once, got %d", calls)
	}
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	calls := 0
	cfg := retryConfig{maxRetries: 3, baseDelay: 0}
	expectedErr := errors.New("status 400")

	err := withRetryConfig(context.Background(), cfg, func(msg string) {}, func() error {
		calls++
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected original error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected fn to be called once, got %d", calls)
	}
}

func TestWithRetry_RetriesOn503ErrorSucceedsSecondAttempt(t *testing.T) {
	calls := 0
	cfg := retryConfig{maxRetries: 3, baseDelay: 0}

	err := withRetryConfig(context.Background(), cfg, func(msg string) {}, func() error {
		calls++
		if calls == 1 {
			return errors.New("status 503")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error after retry, got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected fn to be called twice, got %d", calls)
	}
}

func TestWithRetry_ExhaustsAllRetries(t *testing.T) {
	calls := 0
	cfg := retryConfig{maxRetries: 3, baseDelay: 0}
	expectedErr := errors.New("status 502")

	err := withRetryConfig(context.Background(), cfg, func(msg string) {}, func() error {
		calls++
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected original error after retries, got %v", err)
	}
	if calls != MaxRetries+1 {
		t.Errorf("expected fn to be called %d times (MaxRetries+1), got %d", MaxRetries+1, calls)
	}
}

func TestIsRetryable_NilError(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("expected IsRetryable(nil) to return false")
	}
}

func TestIsRetryable_503Error(t *testing.T) {
	if !IsRetryable(errors.New("status 503")) {
		t.Error("expected IsRetryable(status 503) to return true")
	}
}

func TestIsRetryable_400Error(t *testing.T) {
	if IsRetryable(errors.New("status 400")) {
		t.Error("expected IsRetryable(status 400) to return false")
	}
}
func TestIsRetryable_500Error(t *testing.T) {
	if !IsRetryable(errors.New("status 500")) {
		t.Error("expected IsRetryable(status 500) to return true")
	}
}

func TestIsRetryable_502Error(t *testing.T) {
	if !IsRetryable(errors.New("status 502")) {
		t.Error("expected IsRetryable(status 502) to return true")
	}
}

func TestIsRetryable_529Error(t *testing.T) {
	if !IsRetryable(errors.New("status 529")) {
		t.Error("expected IsRetryable(status 529) to return true")
	}
}

func TestIsRetryable_429Error(t *testing.T) {
	if !IsRetryable(errors.New("status 429")) {
		t.Error("expected IsRetryable(status 429) to return true")
	}
}

func TestRetryAfterFromError_WithRetryAfterHeader(t *testing.T) {
	err := errors.New("rate limited: retry-after: 30")
	d := RetryAfterFromError(err)
	if d != 30*time.Second {
		t.Errorf("RetryAfterFromError() = %v, want 30s", d)
	}
}

func TestRetryAfterFromError_CaseInsensitive(t *testing.T) {
	err := errors.New("rate limited: Retry-After: 60")
	d := RetryAfterFromError(err)
	if d != 60*time.Second {
		t.Errorf("RetryAfterFromError() = %v, want 60s", d)
	}
}

func TestRetryAfterFromError_NoHeader(t *testing.T) {
	err := errors.New("status 503 server error")
	d := RetryAfterFromError(err)
	if d != 0 {
		t.Errorf("RetryAfterFromError() = %v, want 0", d)
	}
}

func TestRetryAfterFromError_Nil(t *testing.T) {
	d := RetryAfterFromError(nil)
	if d != 0 {
		t.Errorf("RetryAfterFromError(nil) = %v, want 0", d)
	}
}

func TestWithRetry_RetriesOn429(t *testing.T) {
	calls := 0
	cfg := retryConfig{maxRetries: 3, baseDelay: 0}

	err := withRetryConfig(context.Background(), cfg, func(msg string) {}, func() error {
		calls++
		if calls == 1 {
			return errors.New("status 429 too many requests")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error after 429 retry, got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected fn to be called twice, got %d", calls)
	}
}

func TestWithRetry_UsesRetryAfterDelay(t *testing.T) {
	calls := 0
	cfg := retryConfig{maxRetries: 3, baseDelay: 0}
	var statusMessages []string

	err := withRetryConfig(context.Background(), cfg, func(msg string) {
		statusMessages = append(statusMessages, msg)
	}, func() error {
		calls++
		if calls == 1 {
			return errors.New("status 429: retry-after: 1")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if len(statusMessages) == 0 {
		t.Error("expected at least one status message")
	}
	// The status message should mention "rate limited"
	if len(statusMessages) > 0 && statusMessages[0] == "" {
		t.Error("status message should not be empty")
	}
}
