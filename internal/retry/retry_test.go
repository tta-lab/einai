package retry

import (
	"context"
	"errors"
	"testing"
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
