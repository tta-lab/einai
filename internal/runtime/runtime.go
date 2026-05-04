// Package runtime defines the agent execution runtime type.
package runtime

import "fmt"

// Runtime identifies the agent execution backend.
type Runtime string

const (
	// Lenos runs the agent by spawning `lenos run`.
	Lenos Runtime = "lenos"
	// ClaudeCode runs the agent by spawning `claude -p` (Claude Code CLI).
	ClaudeCode Runtime = "claude-code"

	// Default is the runtime used when no flag or config is set.
	Default = Lenos
)

// Parse parses a runtime string and returns the Runtime constant or an error.
func Parse(s string) (Runtime, error) {
	switch Runtime(s) {
	case Lenos:
		return Lenos, nil
	case ClaudeCode:
		return ClaudeCode, nil
	default:
		return "", fmt.Errorf("unknown runtime %q (want lenos or claude-code)", s)
	}
}

// String returns the string representation of the runtime.
func (r Runtime) String() string {
	return string(r)
}
