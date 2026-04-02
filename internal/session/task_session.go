package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"charm.land/fantasy"
	"github.com/tta-lab/einai/internal/config"
)

// taskIDPattern matches 8-char hex IDs and full UUIDs (36 chars with hyphens)
var taskIDPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

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

// ValidateWithTaskwarrior checks task existence and status via taskwarrior.
func (t TaskID) ValidateWithTaskwarrior() error {
	args := []string{"rc:" + config.TaskrcPath(), "rc.json.array:on", "export", string(t)}
	cmd := exec.Command("task", args...)
	output, err := cmd.Output()
	if err != nil {
		if execErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("task %s not found: %s", string(t), strings.TrimSpace(string(execErr.Stderr)))
		}
		return fmt.Errorf("task %s: %w", string(t), err)
	}

	// Parse task JSON output to check status
	output = bytes.TrimSpace(output)
	if len(output) == 0 || string(output) == "[]" {
		return fmt.Errorf("task %s not found", string(t))
	}

	// Simple JSON parsing to extract status field
	var tasks []map[string]interface{}
	if err := json.Unmarshal(output, &tasks); err != nil {
		return fmt.Errorf("parse task output: %w", err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("task %s not found", string(t))
	}

	status, ok := tasks[0]["status"].(string)
	if !ok {
		return fmt.Errorf("task %s: status field missing", string(t))
	}
	if status != "pending" {
		return fmt.Errorf("task %s is %s, must be pending", string(t), status)
	}

	return nil
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
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No session exists yet
		}
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer file.Close()

	history := &SessionHistory{
		AgentName: agentName,
		TaskID:    string(taskID),
		Messages:  []SessionMessage{},
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg SessionMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines
		}
		history.Messages = append(history.Messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}

	if len(history.Messages) == 0 {
		return nil, nil // Empty session
	}

	return history, nil
}

// SaveSession saves the session history to disk in JSONL format (one JSON object per line).
func SaveSession(agentName string, taskID TaskID, messages []SessionMessage) error {
	dir := filepath.Join(config.DefaultDataDir(), "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}

	path := SessionFilePath(agentName, taskID)

	// Open file in truncate mode to replace content with complete history
	file, err := os.OpenFile(path, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	defer file.Close()

	// Write each message as a JSON line
	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshal session message: %w", err)
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write session line: %w", err)
		}
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
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: msg.Content}},
		})
	}
	return messages
}
