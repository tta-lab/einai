package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/logos"
)

// getTextFromMessagePart extracts the text from a fantasy.MessagePart, returning
// empty string for non-text parts (e.g., ReasoningPart).
func getTextFromMessagePart(part fantasy.MessagePart) string {
	if tp, ok := part.(fantasy.TextPart); ok {
		return tp.Text
	}
	return ""
}

func TestTaskIDValidation(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		// Valid 8-char hex IDs
		{"abc12345", true}, // 8 char hex (lowercase)
		{"ABC12345", true}, // 8 char hex (uppercase)
		{"AbCdEf12", true}, // 8 char hex (mixed case)
		{"12345678", true}, // 8 char hex (all digits)
		// Valid UUIDs
		{"12345678-1234-1234-1234-123456789abc", true}, // Full UUID
		// Invalid IDs
		{"abc123", false},       // Too short (6 chars)
		{"abc", false},          // Too short (3 chars)
		{"abcde", false},        // 5 chars - too short
		{"abcdef123456", false}, // 12 chars - not valid (must be exactly 8)
		{"not-a-valid-id", false},
		{"", false},
		{"abc def", false}, // Contains space
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			tid := TaskID(tt.id)
			assert.Equal(t, tt.valid, tid.IsValid(), "TaskID(%q).IsValid()", tt.id)
		})
	}
}

func TestSessionFilePath(t *testing.T) {
	tests := []struct {
		agent    string
		taskID   TaskID
		expected string
	}{
		{
			agent:    "test-agent",
			taskID:   TaskID("abc12345"),
			expected: "test-agent-abc12345.jsonl",
		},
		{
			agent:    "coder",
			taskID:   TaskID("12345678-1234-1234-1234-123456789abc"),
			expected: "coder-12345678-1234-1234-1234-123456789abc.jsonl",
		},
		{
			agent:    "my-agent",
			taskID:   TaskID("xyz99999"),
			expected: "my-agent-xyz99999.jsonl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.agent+"-"+string(tt.taskID), func(t *testing.T) {
			path := SessionFilePath(tt.agent, tt.taskID)
			assert.True(t, strings.HasSuffix(path, tt.expected),
				"path %q should end with %q", path, tt.expected)
			assert.Contains(t, path, "sessions")
		})
	}
}

func TestSessionFilePath_SpecialChars(t *testing.T) {
	// Test that slashes in task ID are replaced with underscores
	path := SessionFilePath("agent", TaskID("abc/123/456"))

	assert.Contains(t, path, "sessions")
	assert.Contains(t, path, "agent-abc_123_456.jsonl")

	// The filename part should not contain path separators
	filename := filepath.Base(path)
	assert.NotContains(t, filename, "/", "filename should not contain path separators")
}

// ============================================================================
// ToFantasyMessages Tests (Critical)
// ============================================================================

func TestToFantasyMessages_AllRoles(t *testing.T) {
	history := &SessionHistory{
		AgentName: "test-agent",
		TaskID:    "abc12345",
		Messages: []SessionMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "tool", Content: "tool result"},
			{Role: "system", Content: "system prompt"},
		},
	}

	messages := history.ToFantasyMessages()

	require.Len(t, messages, 4)

	// Check roles using direct string comparison (MessageRole is a string type)
	assert.Equal(t, "user", string(messages[0].Role))
	assert.Equal(t, "hello", getTextFromMessagePart(messages[0].Content[0]))

	assert.Equal(t, "assistant", string(messages[1].Role))
	assert.Equal(t, "hi", getTextFromMessagePart(messages[1].Content[0]))

	assert.Equal(t, "tool", string(messages[2].Role))
	assert.Equal(t, "tool result", getTextFromMessagePart(messages[2].Content[0]))

	assert.Equal(t, "system", string(messages[3].Role))
	assert.Equal(t, "system prompt", getTextFromMessagePart(messages[3].Content[0]))
}

func TestToFantasyMessages_UnknownRole(t *testing.T) {
	history := &SessionHistory{
		Messages: []SessionMessage{
			{Role: "unknown_role", Content: "what?"},
		},
	}

	messages := history.ToFantasyMessages()

	require.Len(t, messages, 1)
	// Unknown roles should be treated as user (default case)
	assert.Equal(t, "user", string(messages[0].Role))
	assert.Equal(t, "what?", getTextFromMessagePart(messages[0].Content[0]))
}

func TestToFantasyMessages_EmptyHistory(t *testing.T) {
	history := &SessionHistory{
		Messages: []SessionMessage{},
	}

	messages := history.ToFantasyMessages()

	assert.Empty(t, messages)
}

func TestToFantasyMessages_NilContent(t *testing.T) {
	history := &SessionHistory{
		Messages: []SessionMessage{
			{Role: "user", Content: ""},
			{Role: "assistant", Content: ""},
		},
	}

	messages := history.ToFantasyMessages()

	require.Len(t, messages, 2)
	assert.Equal(t, "", getTextFromMessagePart(messages[0].Content[0]))
	assert.Equal(t, "", getTextFromMessagePart(messages[1].Content[0]))
}

// ============================================================================
// SaveSession/LoadSession Tests (Critical - Session Persistence)
// ============================================================================

func TestSaveAndLoadSession_RoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestDataDir(tempDir)
	defer config.ClearTestDataDir()

	agentName := "test-agent"
	taskID := TaskID("abc12345")

	messages := []SessionMessage{
		{Role: "user", Content: "Hello, how are you?", Reasoning: ""},
		{Role: "assistant", Content: "I'm doing well!", Reasoning: "The user asked how I am"},
		{Role: "tool", Content: "Tool executed successfully", Reasoning: "Called the tool"},
	}

	// Save the session
	err := SaveSession(agentName, taskID, messages)
	require.NoError(t, err, "SaveSession should not return an error")

	// Load the session back
	history, err := LoadSession(agentName, taskID)
	require.NoError(t, err, "LoadSession should not return an error")
	require.NotNil(t, history, "Loaded history should not be nil")

	// Verify the round-trip
	assert.Equal(t, agentName, history.AgentName)
	assert.Equal(t, string(taskID), history.TaskID)
	require.Len(t, history.Messages, 3)

	assert.Equal(t, "user", history.Messages[0].Role)
	assert.Equal(t, "Hello, how are you?", history.Messages[0].Content)
	assert.Equal(t, "", history.Messages[0].Reasoning)

	assert.Equal(t, "assistant", history.Messages[1].Role)
	assert.Equal(t, "I'm doing well!", history.Messages[1].Content)
	assert.Equal(t, "The user asked how I am", history.Messages[1].Reasoning)

	assert.Equal(t, "tool", history.Messages[2].Role)
	assert.Equal(t, "Tool executed successfully", history.Messages[2].Content)
	assert.Equal(t, "Called the tool", history.Messages[2].Reasoning)
}

func TestLoadSession_MalformedLine_SkipsGracefully(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestDataDir(tempDir)
	defer config.ClearTestDataDir()

	agentName := "test-agent"
	taskID := TaskID("abc12345")

	// First, save a valid message so we have a file to work with
	validMsg := SessionMessage{Role: "user", Content: "valid message"}
	err := SaveSession(agentName, taskID, []SessionMessage{validMsg})
	require.NoError(t, err)

	// Now read the file and prepend a malformed line
	sessionPath := SessionFilePath(agentName, taskID)
	content, err := os.ReadFile(sessionPath)
	require.NoError(t, err)

	// Create malformed line: take the valid JSON and corrupt it
	malformed := make([]byte, len(content))
	copy(malformed, content)
	malformed[0] = '{'                // Change first char to break JSON
	malformed[len(malformed)-1] = ',' // Change last char

	// Write combined: malformed line first, then newline, then valid line
	newContent := make([]byte, 0, len(malformed)+1+len(content))
	newContent = append(newContent, malformed...)
	newContent = append(newContent, '\n')
	newContent = append(newContent, content...)
	err = os.WriteFile(sessionPath, newContent, 0644)
	require.NoError(t, err)

	// Load should succeed and skip the malformed line
	history, err := LoadSession(agentName, taskID)
	require.NoError(t, err, "LoadSession should not return error even with malformed lines")
	require.NotNil(t, history)

	// Should only have the one valid message (from the valid part)
	require.Len(t, history.Messages, 1)
	assert.Equal(t, "valid message", history.Messages[0].Content)
}

func TestLoadSession_FileNotFound_ReturnsNil(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestDataDir(tempDir)
	defer config.ClearTestDataDir()

	history, err := LoadSession("nonexistent-agent", TaskID("abc12345"))
	assert.NoError(t, err, "LoadSession should not error on missing file")
	assert.Nil(t, history, "Should return nil for non-existent session")
}

func TestLoadSession_EmptyFile_ReturnsNil(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestDataDir(tempDir)
	defer config.ClearTestDataDir()

	agentName := "test-agent"
	taskID := TaskID("abc12345")

	// Save a session first to create the file
	err := SaveSession(agentName, taskID, []SessionMessage{{Role: "user", Content: "test"}})
	require.NoError(t, err)

	// Now overwrite with empty content
	sessionPath := SessionFilePath(agentName, taskID)
	err = os.WriteFile(sessionPath, []byte{}, 0644)
	require.NoError(t, err)

	history, err := LoadSession(agentName, taskID)
	assert.NoError(t, err)
	assert.Nil(t, history, "Empty session file should return nil")
}

func TestLoadSession_WhitespaceOnlyLines_Skipped(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestDataDir(tempDir)
	defer config.ClearTestDataDir()

	agentName := "test-agent"
	taskID := TaskID("abc12345")

	// Save a session first to create the file
	validMsg := SessionMessage{Role: "user", Content: "valid"}
	err := SaveSession(agentName, taskID, []SessionMessage{validMsg})
	require.NoError(t, err)

	// Read the file
	sessionPath := SessionFilePath(agentName, taskID)
	content, err := os.ReadFile(sessionPath)
	require.NoError(t, err)

	// Write with whitespace lines around the valid content
	newContent := "\n   \n\t\n" + string(content) + "\n   \n"
	err = os.WriteFile(sessionPath, []byte(newContent), 0644)
	require.NoError(t, err)

	history, err := LoadSession(agentName, taskID)
	require.NoError(t, err)
	require.NotNil(t, history)
	require.Len(t, history.Messages, 1)
	assert.Equal(t, "valid", history.Messages[0].Content)
}

func TestSaveSession_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestDataDir(tempDir)
	defer config.ClearTestDataDir()

	agentName := "new-agent"
	taskID := TaskID("abc12345")

	// Ensure sessions directory doesn't exist
	sessionsDir := filepath.Join(tempDir, "sessions")
	_, err := os.Stat(sessionsDir)
	assert.True(t, os.IsNotExist(err), "Sessions directory should not exist initially")

	// Save should create the directory
	err = SaveSession(agentName, taskID, []SessionMessage{{Role: "user", Content: "test"}})
	require.NoError(t, err, "SaveSession should create the sessions directory")

	// Verify directory was created
	_, err = os.Stat(sessionsDir)
	assert.NoError(t, err, "Sessions directory should exist after SaveSession")

	// Verify file was created
	_, err = os.Stat(SessionFilePath(agentName, taskID))
	assert.NoError(t, err, "Session file should exist after SaveSession")
}

func TestSaveSession_TruncatesExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestDataDir(tempDir)
	defer config.ClearTestDataDir()

	agentName := "test-agent"
	taskID := TaskID("abc12345")

	// Save initial session
	err := SaveSession(agentName, taskID, []SessionMessage{
		{Role: "user", Content: "first"},
	})
	require.NoError(t, err)

	// Save new session with different content
	err = SaveSession(agentName, taskID, []SessionMessage{
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "response"},
	})
	require.NoError(t, err)

	// Load and verify only the new content exists
	history, err := LoadSession(agentName, taskID)
	require.NoError(t, err)
	require.Len(t, history.Messages, 2)
	assert.Equal(t, "second", history.Messages[0].Content)
	assert.Equal(t, "response", history.Messages[1].Content)
}

// ============================================================================
// buildInitialPrompt Tests (Important)
// ============================================================================

func TestBuildInitialPrompt(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		task     string
		expected string
	}{
		{
			name:     "both_non_empty",
			user:     "user prompt",
			task:     "task context",
			expected: "task context\n\n---\n\nUser request: user prompt",
		},
		{
			name:     "only_user",
			user:     "user prompt",
			task:     "",
			expected: "user prompt",
		},
		{
			name:     "only_task",
			user:     "",
			task:     "task context",
			expected: "task context",
		},
		{
			name:     "both_empty",
			user:     "",
			task:     "",
			expected: "",
		},
		{
			name:     "multiline_task",
			user:     "user input",
			task:     "line1\nline2\nline3",
			expected: "line1\nline2\nline3\n\n---\n\nUser request: user input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildInitialPrompt(tt.user, tt.task)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================================================
// mergeSessionMessages Tests (Important)
// ============================================================================

func TestMergeSessionMessages_NilHistory(t *testing.T) {
	steps := []logos.StepMessage{
		{Role: logos.StepRoleUser, Content: "hello"},
		{Role: logos.StepRoleAssistant, Content: "hi"},
	}

	messages := mergeSessionMessages(nil, steps)

	require.Len(t, messages, 2)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "hello", messages[0].Content)
	assert.Equal(t, "assistant", messages[1].Role)
}

func TestMergeSessionMessages_EmptyHistory(t *testing.T) {
	history := &SessionHistory{Messages: []SessionMessage{}}
	steps := []logos.StepMessage{
		{Role: logos.StepRoleUser, Content: "hello"},
	}

	messages := mergeSessionMessages(history, steps)

	require.Len(t, messages, 1)
	assert.Equal(t, "hello", messages[0].Content)
}

func TestMergeSessionMessages_ExistingHistory(t *testing.T) {
	history := &SessionHistory{
		Messages: []SessionMessage{
			{Role: "user", Content: "original message"},
		},
	}
	steps := []logos.StepMessage{
		{Role: logos.StepRoleAssistant, Content: "response 1"},
		{Role: logos.StepRoleUser, Content: "follow up"},
	}

	messages := mergeSessionMessages(history, steps)

	require.Len(t, messages, 3)
	assert.Equal(t, "original message", messages[0].Content)
	assert.Equal(t, "response 1", messages[1].Content)
	assert.Equal(t, "follow up", messages[2].Content)
}

func TestMergeSessionMessages_EmptySteps(t *testing.T) {
	history := &SessionHistory{
		Messages: []SessionMessage{
			{Role: "user", Content: "existing"},
		},
	}

	messages := mergeSessionMessages(history, []logos.StepMessage{})

	require.Len(t, messages, 1)
	assert.Equal(t, "existing", messages[0].Content)
}

// ============================================================================
// ConvertFromStepMessages Tests (Nice-to-have)
// ============================================================================

func TestConvertFromStepMessages(t *testing.T) {
	steps := []logos.StepMessage{
		{Role: logos.StepRoleAssistant, Content: "hello", Reasoning: "thinking..."},
		{Role: logos.StepRoleUser, Content: "question", Reasoning: ""},
		{Role: logos.StepRoleResult, Content: "tool result", Reasoning: "execution"},
	}

	messages := ConvertFromStepMessages(steps)

	require.Len(t, messages, 3)

	assert.Equal(t, "assistant", messages[0].Role)
	assert.Equal(t, "hello", messages[0].Content)
	assert.Equal(t, "thinking...", messages[0].Reasoning)

	assert.Equal(t, "user", messages[1].Role)
	assert.Equal(t, "question", messages[1].Content)
	assert.Equal(t, "", messages[1].Reasoning)

	// logos.StepRoleResult becomes "result" when converted to string
	assert.Equal(t, "result", messages[2].Role)
	assert.Equal(t, "tool result", messages[2].Content)
	assert.Equal(t, "execution", messages[2].Reasoning)
}

func TestConvertFromStepMessages_EmptySlice(t *testing.T) {
	messages := ConvertFromStepMessages([]logos.StepMessage{})
	assert.Empty(t, messages)
}

func TestConvertFromStepMessages_PreservesContent(t *testing.T) {
	steps := []logos.StepMessage{
		{
			Role:      logos.StepRoleAssistant,
			Content:   "Detailed response with special chars: <>&\"'",
			Reasoning: "Simple reasoning",
		},
	}

	messages := ConvertFromStepMessages(steps)

	require.Len(t, messages, 1)
	assert.Equal(t, steps[0].Content, messages[0].Content)
	assert.Equal(t, steps[0].Reasoning, messages[0].Reasoning)
}

// ============================================================================
// Integration: Full Session Lifecycle
// ============================================================================

func TestFullSessionLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestDataDir(tempDir)
	defer config.ClearTestDataDir()

	agentName := "coder"
	taskID := TaskID("def67890")

	// Start with an empty session
	history, err := LoadSession(agentName, taskID)
	assert.NoError(t, err)
	assert.Nil(t, history)

	// Simulate first interaction
	initialMessages := []SessionMessage{
		{Role: "user", Content: "Write a hello world program"},
	}
	err = SaveSession(agentName, taskID, initialMessages)
	require.NoError(t, err)

	// Load and verify
	history, err = LoadSession(agentName, taskID)
	require.NoError(t, err)
	require.Len(t, history.Messages, 1)

	// Simulate continuation (merge new messages)
	newSteps := []logos.StepMessage{
		{Role: logos.StepRoleAssistant, Content: "// hello.go\npackage main\n\nfunc main() {}\n", Reasoning: "Creating file"},
		{Role: logos.StepRoleResult, Content: "Created hello.go", Reasoning: ""},
	}
	merged := mergeSessionMessages(history, newSteps)
	err = SaveSession(agentName, taskID, merged)
	require.NoError(t, err)

	// Verify full history
	history, err = LoadSession(agentName, taskID)
	require.NoError(t, err)
	require.Len(t, history.Messages, 3)

	// Convert to fantasy messages for agent replay
	fantasyMsgs := history.ToFantasyMessages()
	require.Len(t, fantasyMsgs, 3)
	assert.Equal(t, "user", string(fantasyMsgs[0].Role))
	assert.Equal(t, "assistant", string(fantasyMsgs[1].Role))
	// logos.StepRoleResult becomes "result" in SessionMessage, which is unknown to ToFantasyMessages,
	// so it falls through to user role (with a warning)
	assert.Equal(t, "user", string(fantasyMsgs[2].Role))
}
