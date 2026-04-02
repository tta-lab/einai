package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"charm.land/fantasy"
	"github.com/tta-lab/einai/internal/config"
)

// taskIDPattern matches taskwarrior hex IDs (6-32 hex chars) and full UUIDs (36 chars with hyphens)
var taskIDPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{6,32}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// TaskID represents a validated taskwarrior task identifier.
type TaskID string

// IsValid checks if the task ID is a valid taskwarrior hex ID or UUID.
func (t TaskID) IsValid() bool {
	return taskIDPattern.MatchString(string(t))
}

// IsUUID checks if the task ID is a full UUID format.
func (t TaskID) IsUUID() bool {
	return strings.Contains(string(t), "-")
}

// String returns the string representation of the task ID.
func (t TaskID) String() string {
	return string(t)
}

// SessionFilePath returns the path to the session file for this agent/task combination.
func SessionFilePath(agentName string, taskID TaskID) string {
	safeTaskID := strings.ReplaceAll(string(taskID), "/", "_")
	return filepath.Join(config.DefaultDataDir(), "sessions", agentName+"-"+safeTaskID+".jsonl")
}

// SessionMessage represents one message in the session history for persistence.
type SessionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning,omitempty"`
}

// SessionHistory holds the persisted conversation history for an agent task session.
type SessionHistory struct {
	AgentName string           `json:"agent_name"`
	TaskID    string           `json:"task_id"`
	Messages  []SessionMessage `json:"messages"`
}

// LoadSession loads a persisted session from disk.
func LoadSession(agentName string, taskID TaskID) (*SessionHistory, error) {
	path := SessionFilePath(agentName, taskID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No session exists yet
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}

	var history SessionHistory
	if err := json.Unmarshal(data, &history); err != nil {
		// Try parsing as JSONL (one JSON object per line)
		return parseJSONLSession(data, agentName, string(taskID))
	}

	return &history, nil
}

// parseJSONLSession parses session history from JSONL format.
func parseJSONLSession(data []byte, agentName, taskID string) (*SessionHistory, error) {
	history := &SessionHistory{
		AgentName: agentName,
		TaskID:    taskID,
		Messages:  []SessionMessage{},
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg SessionMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines
		}
		history.Messages = append(history.Messages, msg)
	}

	return history, nil
}

// SaveSession saves the session history to disk.
func SaveSession(agentName string, taskID TaskID, messages []SessionMessage) error {
	dir := filepath.Join(config.DefaultDataDir(), "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}

	path := SessionFilePath(agentName, taskID)

	// Load existing messages to append
	existingMessages := []SessionMessage{}
	if existing, err := LoadSession(agentName, taskID); err == nil && existing != nil {
		existingMessages = existing.Messages
	}

	allMessages := append(existingMessages, messages...)

	// Write as JSON
	history := SessionHistory{
		AgentName: agentName,
		TaskID:    string(taskID),
		Messages:  allMessages,
	}

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}

	return nil
}

// ToFantasyMessages converts SessionMessages to []fantasy.Message for logos.Run.
func (h *SessionHistory) ToFantasyMessages() []fantasy.Message {
	messages := make([]fantasy.Message, 0, len(h.Messages))
	for _, msg := range h.Messages {
		role := fantasy.MessageRoleUser
		switch msg.Role {
		case "assistant":
			role = fantasy.MessageRoleAssistant
		case "system":
			role = fantasy.MessageRoleSystem
		case "tool":
			role = fantasy.MessageRoleTool
		}
		messages = append(messages, fantasy.Message{
			Role:    role,
			Content: []fantasy.MessagePart{fantasy.TextContent(msg.Content)},
		})
	}
	return messages
}
