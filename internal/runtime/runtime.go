// Package runtime defines the agent execution runtime type.
package runtime

import "fmt"

// Runtime identifies the agent execution backend.
type Runtime string

const (
	// EiNative runs the agent using the logos+temenos loop built into einai.
	EiNative Runtime = "ei-native"
	// ClaudeCode runs the agent by spawning `claude -p` (Claude Code CLI).
	ClaudeCode Runtime = "claude-code"

	// Default is the runtime used when no flag or config is set.
	Default = ClaudeCode
)

// Parse parses a runtime string and returns the Runtime constant or an error.
func Parse(s string) (Runtime, error) {
	switch Runtime(s) {
	case EiNative:
		return EiNative, nil
	case ClaudeCode:
		return ClaudeCode, nil
	default:
		return "", fmt.Errorf("unknown runtime %q (want ei-native or claude-code)", s)
	}
}

// String returns the string representation of the runtime.
func (r Runtime) String() string {
	return string(r)
}
