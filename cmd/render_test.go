package cmd

import (
	"strings"
	"testing"
)

func TestDiscardCmdBlockDeltas(t *testing.T) {
	// Since logos v1.2.0-pre.1 sends cmd blocks atomically,
	// we discard deltas that start with <cmd> - they're rendered via EventCommandResult.
	//
	// Note: When <think>/</think> tags are stripped by logos, surrounding newlines remain.
	// This can result in whitespace-only deltas (e.g., "\n\n\n\n") after discarding cmd blocks.
	// These are handled by flushBuffer skipping whitespace-only content to avoid extra blank lines.
	tests := []struct {
		name          string
		input         string
		shouldDiscard bool
	}{
		{
			name:          "empty string",
			input:         "",
			shouldDiscard: false,
		},
		{
			name:          "plain text",
			input:         "Hello, world!",
			shouldDiscard: false,
		},
		{
			name:          "cmd block start",
			input:         "<cmd>echo hello</cmd>",
			shouldDiscard: true,
		},
		{
			name:          "prose before cmd block",
			input:         "Let me run this command:<cmd>echo hello</cmd>",
			shouldDiscard: false, // doesn't start with <cmd>
		},
		{
			name:          "cmd in middle of text",
			input:         "Some text <cmd>cmd</cmd> more text",
			shouldDiscard: false, // doesn't start with <cmd>
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the discard logic: strings.HasPrefix(text, logos.CmdBlockOpen)
			isCmdBlock := strings.HasPrefix(tt.input, "<cmd>")
			if isCmdBlock != tt.shouldDiscard {
				t.Errorf("cmd block detection = %v, want %v", isCmdBlock, tt.shouldDiscard)
			}
		})
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "under limit",
			input:    "line1\nline2\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "exactly at limit (10 lines)",
			input:    "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10",
			expected: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10",
		},
		{
			name:     "over limit shows truncation message",
			input:    "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11",
			expected: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n... (1 more line)",
		},
		{
			name:     "well over limit shows plural message",
			input:    "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\nline13",
			expected: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n... (3 more lines)",
		},
		{
			name:     "trailing empty lines not counted",
			input:    "line1\nline2\n\n\n",
			expected: "line1\nline2\n\n\n",
		},
		{
			name:     "over limit with trailing empty lines",
			input:    "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\n\n\n",
			expected: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n... (1 more line)",
		},
		{
			name:     "single line",
			input:    "just one line",
			expected: "just one line",
		},
		{
			name:     "single line over limit",
			input:    "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11",
			expected: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n... (1 more line)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateOutput(tt.input)
			if result != tt.expected {
				t.Errorf("truncateOutput(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
