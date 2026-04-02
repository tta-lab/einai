package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskIDValidation(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		// Valid 8-char hex IDs
		{"abc12345", true},    // 8 char hex (lowercase)
		{"ABC12345", true},    // 8 char hex (uppercase)
		{"AbCdEf12", true},    // 8 char hex (mixed case)
		{"12345678", true},    // 8 char hex (all digits)
		// Valid UUIDs
		{"12345678-1234-1234-1234-123456789abc", true}, // Full UUID
		// Invalid IDs
		{"abc123", false},     // Too short (6 chars)
		{"abc", false},        // Too short (3 chars)
		{"abcde", false},      // 5 chars - too short
		{"abcdef123456", false}, // 12 chars - not valid (must be exactly 8)
		{"not-a-valid-id", false},
		{"", false},
		{"abc def", false},    // Contains space
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			tid := TaskID(tt.id)
			assert.Equal(t, tt.valid, tid.IsValid(), "TaskID(%q).IsValid()", tt.id)
		})
	}
}

func TestTaskIDIsUUID(t *testing.T) {
	tests := []struct {
		id     string
		isUUID bool
	}{
		{"abc12345", false},
		{"12345678", false},
		{"12345678-1234-1234-1234-123456789abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			tid := TaskID(tt.id)
			assert.Equal(t, tt.isUUID, tid.IsUUID(), "TaskID(%q).IsUUID()", tt.id)
		})
	}
}

func TestSessionFilePathFormat(t *testing.T) {
	// Verify the expected path format for session files
	agentName := "test-agent"
	taskID := TaskID("abc12345")

	// The path should be: ~/.einai/sessions/<agent-name>-<task-id>.jsonl
	// We can't test the full path without the config, but we can verify
	// the components work correctly
	path := SessionFilePath(agentName, taskID)
	assert.Contains(t, path, "sessions")
	assert.Contains(t, path, "test-agent-abc12345.jsonl")
}
