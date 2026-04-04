package cmd

import (
	"os"
	"testing"
)

// TestAsyncFlagRegistered verifies the --async flag is registered on agentRunCmd.
func TestAsyncFlagRegistered(t *testing.T) {
	f := agentRunCmd.Flags().Lookup("async")
	if f == nil {
		t.Fatal("--async flag not registered on agentRunCmd")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("--async flag type = %q, want bool", f.Value.Type())
	}
}

// TestCaptureTmuxTarget_NoTmux verifies captureTmuxTarget returns empty string
// when not in a tmux session (tmux command unavailable or returns error).
func TestCaptureTmuxTarget_NoTmux(t *testing.T) {
	// Override PATH to make tmux unavailable.
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", "")                            //nolint:errcheck

	target := captureTmuxTarget()
	if target != "" {
		t.Errorf("captureTmuxTarget() = %q, want empty string when tmux unavailable", target)
	}
}

func TestBuildPrompt(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		stdinContent        string
		expectedPrompt      string
		stdinModeCharDevice bool
	}{
		{
			name:           "empty args and no stdin",
			args:           []string{"agent"},
			stdinContent:   "",
			expectedPrompt: "",
		},
		{
			name:           "only positional prompt",
			args:           []string{"agent", "implement auth"},
			stdinContent:   "",
			expectedPrompt: "implement auth",
		},
		{
			name:           "only stdin content",
			args:           []string{"agent"},
			stdinContent:   "stdin content here",
			expectedPrompt: "stdin content here",
		},
		{
			name:           "stdin and positional combined",
			args:           []string{"agent", "implement this"},
			stdinContent:   "stdin content here",
			expectedPrompt: "stdin content here\n\nimplement this",
		},
		{
			name:           "positional with empty stdin",
			args:           []string{"agent", "hello"},
			stdinContent:   "",
			expectedPrompt: "hello",
		},
		{
			name:           "multiline stdin",
			args:           []string{"agent", "review"},
			stdinContent:   "line1\nline2\nline3",
			expectedPrompt: "line1\nline2\nline3\n\nreview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stdinContent != "" {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatalf("failed to create pipe: %v", err)
				}
				defer r.Close()
				defer w.Close()

				_, err = w.WriteString(tt.stdinContent)
				if err != nil {
					t.Fatalf("failed to write to pipe: %v", err)
				}
				w.Close()

				oldStdin := os.Stdin
				os.Stdin = r
				defer func() { os.Stdin = oldStdin }()
			}

			result := buildPrompt(tt.args)
			if result != tt.expectedPrompt {
				t.Errorf("buildPrompt(%v) = %q, want %q", tt.args, result, tt.expectedPrompt)
			}
		})
	}
}
