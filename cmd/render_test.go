package cmd

import "testing"

func TestCleanModelMarkers(t *testing.T) {
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
			name:     "no markers",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "remove cmd open tag",
			input:    "<cmd>echo hello</cmd>",
			expected: "echo hello",
		},
		{
			name:     "remove cmd close tag only",
			input:    "echo hello</cmd>",
			expected: "echo hello",
		},
		{
			name:     "remove cmd open tag only",
			input:    "<cmd>echo hello",
			expected: "echo hello",
		},
		{
			name:     "remove multiple cmd tags",
			input:    "<cmd>first</cmd> middle <cmd>second</cmd>",
			expected: "first middle second",
		},
		{
			name:     "trim whitespace",
			input:    "  <cmd>hello</cmd>  ",
			expected: "hello",
		},
		{
			name:     "cmd with command prefix preserved",
			input:    "<cmd>§ echo hello</cmd>",
			expected: "§ echo hello",
		},
		{
			name:     "complex example with code",
			input:    "<cmd>§ git commit -m \"fix: resolve issue\"</cmd>",
			expected: "§ git commit -m \"fix: resolve issue\"",
		},
		{
			name:     "multiline content with command prefix",
			input:    "<cmd>line1\n§ line2</cmd>",
			expected: "line1\n§ line2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanModelMarkers(tt.input)
			if result != tt.expected {
				t.Errorf("cleanModelMarkers(%q) = %q, want %q", tt.input, result, tt.expected)
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
