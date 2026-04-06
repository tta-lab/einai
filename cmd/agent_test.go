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

// TestCaptureSendTarget_NoAgentName verifies captureSendTarget returns empty string
// when TTAL_AGENT_NAME is not set.
func TestCaptureSendTarget_NoAgentName(t *testing.T) {
	// Ensure TTAL_AGENT_NAME is not set.
	oldAgentName := os.Getenv("TTAL_AGENT_NAME")
	t.Cleanup(func() {
		if oldAgentName == "" {
			os.Unsetenv("TTAL_AGENT_NAME") //nolint:errcheck
		} else {
			os.Setenv("TTAL_AGENT_NAME", oldAgentName) //nolint:errcheck
		}
	})
	os.Unsetenv("TTAL_AGENT_NAME") //nolint:errcheck

	target := captureSendTarget()
	if target != "" {
		t.Errorf("captureSendTarget() = %q, want empty string when TTAL_AGENT_NAME not set", target)
	}
}

// TestCaptureSendTarget_ManagerAgent verifies captureSendTarget returns just the
// agent name when TTAL_JOB_ID is not set (manager plane).
func TestCaptureSendTarget_ManagerAgent(t *testing.T) {
	oldAgentName := os.Getenv("TTAL_AGENT_NAME")
	oldJobID := os.Getenv("TTAL_JOB_ID")
	t.Cleanup(func() {
		if oldAgentName == "" {
			os.Unsetenv("TTAL_AGENT_NAME") //nolint:errcheck
		} else {
			os.Setenv("TTAL_AGENT_NAME", oldAgentName) //nolint:errcheck
		}
		if oldJobID == "" {
			os.Unsetenv("TTAL_JOB_ID") //nolint:errcheck
		} else {
			os.Setenv("TTAL_JOB_ID", oldJobID) //nolint:errcheck
		}
	})
	os.Setenv("TTAL_AGENT_NAME", "astra") //nolint:errcheck
	os.Unsetenv("TTAL_JOB_ID")            //nolint:errcheck

	target := captureSendTarget()
	if target != "astra" {
		t.Errorf("captureSendTarget() = %q, want \"astra\" for manager agent", target)
	}
}

// TestCaptureSendTarget_WorkerSession verifies captureSendTarget returns
// "jobID:agentName" when both TTAL_JOB_ID and TTAL_AGENT_NAME are set.
func TestCaptureSendTarget_WorkerSession(t *testing.T) {
	oldAgentName := os.Getenv("TTAL_AGENT_NAME")
	oldJobID := os.Getenv("TTAL_JOB_ID")
	t.Cleanup(func() {
		if oldAgentName == "" {
			os.Unsetenv("TTAL_AGENT_NAME") //nolint:errcheck
		} else {
			os.Setenv("TTAL_AGENT_NAME", oldAgentName) //nolint:errcheck
		}
		if oldJobID == "" {
			os.Unsetenv("TTAL_JOB_ID") //nolint:errcheck
		} else {
			os.Setenv("TTAL_JOB_ID", oldJobID) //nolint:errcheck
		}
	})
	os.Setenv("TTAL_AGENT_NAME", "coder") //nolint:errcheck
	os.Setenv("TTAL_JOB_ID", "abc12345")  //nolint:errcheck

	target := captureSendTarget()
	if target != "abc12345:coder" {
		t.Errorf("captureSendTarget() = %q, want \"abc12345:coder\"", target)
	}
}

// TestCaptureSendTarget_JobIDWithoutAgentName verifies captureSendTarget returns
// empty string when TTAL_JOB_ID is set but TTAL_AGENT_NAME is not (not routable).
func TestCaptureSendTarget_JobIDWithoutAgentName(t *testing.T) {
	oldAgentName := os.Getenv("TTAL_AGENT_NAME")
	oldJobID := os.Getenv("TTAL_JOB_ID")
	t.Cleanup(func() {
		if oldAgentName == "" {
			os.Unsetenv("TTAL_AGENT_NAME") //nolint:errcheck
		} else {
			os.Setenv("TTAL_AGENT_NAME", oldAgentName) //nolint:errcheck
		}
		if oldJobID == "" {
			os.Unsetenv("TTAL_JOB_ID") //nolint:errcheck
		} else {
			os.Setenv("TTAL_JOB_ID", oldJobID) //nolint:errcheck
		}
	})
	os.Unsetenv("TTAL_AGENT_NAME")       //nolint:errcheck
	os.Setenv("TTAL_JOB_ID", "abc12345") //nolint:errcheck

	target := captureSendTarget()
	if target != "" {
		t.Errorf("captureSendTarget() = %q, want empty string when TTAL_JOB_ID set but TTAL_AGENT_NAME not", target)
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
