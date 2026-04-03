package cmd

import (
	"testing"

	"github.com/tta-lab/einai/internal/event"
)

func TestHandleEvent(t *testing.T) {
	tests := []struct {
		name           string
		e              event.Event
		expectResponse bool
		expectError    bool
		errorContains  string
	}{
		{
			name: "delta event",
			e: event.Event{
				Type: event.EventDelta,
				Text: "Hello, world!",
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "error event returns error",
			e: event.Event{
				Type:    event.EventError,
				Message: "something went wrong",
			},
			expectResponse: false,
			expectError:    true,
			errorContains:  "something went wrong",
		},
		{
			name: "done event with response",
			e: event.Event{
				Type:     event.EventDone,
				Response: "final response content",
			},
			expectResponse: true,
			expectError:    false,
		},
		{
			name: "done event empty response",
			e: event.Event{
				Type:     event.EventDone,
				Response: "",
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "command start event",
			e: event.Event{
				Type:    event.EventCommandStart,
				Command: "git status",
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "command result event with exit code 0",
			e: event.Event{
				Type:     event.EventCommandResult,
				Command:  "git status",
				Output:   "On branch main",
				ExitCode: 0,
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "command result event with exit code 1",
			e: event.Event{
				Type:     event.EventCommandResult,
				Command:  "git status",
				Output:   "fatal: not a git repository",
				ExitCode: 1,
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "retry event",
			e: event.Event{
				Type:   event.EventRetry,
				Reason: "rate limited",
				Step:   1,
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "status event",
			e: event.Event{
				Type:    event.EventStatus,
				Message: "processing request",
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "warning event",
			e: event.Event{
				Type:    event.EventWarning,
				Message: "deprecated feature",
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "rate limit event with retry after",
			e: event.Event{
				Type:                event.EventRateLimit,
				RateLimitRetryAfter: 30,
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "rate limit event without retry after",
			e: event.Event{
				Type:                event.EventRateLimit,
				RateLimitRetryAfter: 0,
			},
			expectResponse: false,
			expectError:    false,
		},
		{
			name: "unknown event type",
			e: event.Event{
				Type: "unknown_type",
			},
			expectResponse: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response string
			err := handleEvent(tt.e, &response)

			if tt.expectError {
				if err == nil {
					t.Errorf("handleEvent() expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !stringsContains(err.Error(), tt.errorContains) {
					t.Errorf("handleEvent() error = %v, want error containing %q", err, tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("handleEvent() unexpected error: %v", err)
				}
			}

			if tt.expectResponse {
				if response == "" {
					t.Errorf("handleEvent() response is empty, want non-empty")
				}
				if response != tt.e.Response {
					t.Errorf("handleEvent() response = %q, want %q", response, tt.e.Response)
				}
			}
		})
	}
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
