package cmd

import (
	"strings"
	"testing"
)

func TestStripCmdMarkers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkFunc func(result string) bool
	}{
		{
			name:  "empty string",
			input: "",
			checkFunc: func(result string) bool {
				return result == ""
			},
		},
		{
			name:  "no markers",
			input: "Hello, world!",
			checkFunc: func(result string) bool {
				return result == "Hello, world!"
			},
		},
		{
			name:  "strip cmd tags",
			input: "<cmd>echo hello</cmd>",
			checkFunc: func(result string) bool {
				return !strings.Contains(result, "<cmd>") && !strings.Contains(result, "</cmd>")
			},
		},
		{
			name:  "strip multiple cmd tags",
			input: "<cmd>first</cmd> middle <cmd>second</cmd>",
			checkFunc: func(result string) bool {
				return !strings.Contains(result, "<cmd>") && !strings.Contains(result, "</cmd>")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripCmdMarkers(tt.input)
			if !tt.checkFunc(result) {
				t.Errorf("stripCmdMarkers(%q) = %q, check failed", tt.input, result)
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
