package session

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskIDValidation(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		// Valid hex IDs (6-32 chars)
		{"abc123", true},                              // 6 char hex
		{"abcdef123456", true},                         // 12 char hex
		{"abcdefghijklmnopqrstuvwxyz123456", true},     // 32 char hex
		{"abc123def456789012345678901234", true},       // 30 char hex
		// Valid UUIDs
		{"12345678-1234-1234-1234-123456789abc", true}, // Full UUID
		// Invalid IDs
		{"abc", false},          // Too short (less than 6 chars)
		{"abcde", false},        // 5 chars - too short
		{"not-a-valid-id", false},
		{"", false},
		{"abc def", false},      // Contains space
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
		id    string
		isUUID bool
	}{
		{"abc123", false},
		{"abcdef123456", false},
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
	taskID := TaskID("abc123")
	
	// The path should be: ~/.einai/sessions/<agent-name>-<task-id>.jsonl
	// We can't test the full path without the config, but we can verify
	// the components work correctly
	path := SessionFilePath(agentName, taskID)
	assert.Contains(t, path, "sessions")
	assert.Contains(t, path, "test-agent-abc123.jsonl")
}
