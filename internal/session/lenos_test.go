package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/config"
)

func TestBuildLenosArgs_Basic(t *testing.T) {
	req := AgentRequest{Name: "debugger", Prompt: "say hi", WorkingDir: "/wd"}
	a := &agent.ParsedAgent{
		Frontmatter: agent.Frontmatter{Lenos: &agent.LenosAgentConfig{}},
	}
	args := buildLenosArgs(req, a, "/wd")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "run") {
		t.Errorf("expected 'run' in args, got %q", got)
	}
	if !strings.Contains(got, "--agent debugger") {
		t.Errorf("expected --agent debugger, got %q", got)
	}
	if !strings.Contains(got, "--cwd /wd") {
		t.Errorf("expected --cwd /wd, got %q", got)
	}
	if !strings.Contains(got, "--quiet") {
		t.Errorf("expected --quiet, got %q", got)
	}
	if !strings.Contains(got, "-- say hi") {
		t.Errorf("expected '-- say hi', got %q", got)
	}
	// No --model (Lenos block has no model)
	if strings.Contains(got, "--model ") {
		t.Errorf("unexpected --model flag in %q", got)
	}
	if !strings.Contains(got, "--small-model") {
		t.Errorf("expected --small-model in %q", got)
	}
}

func TestBuildLenosArgs_WithModel(t *testing.T) {
	req := AgentRequest{Name: "debugger", Prompt: "hi", WorkingDir: "/wd"}
	a := &agent.ParsedAgent{
		Frontmatter: agent.Frontmatter{Lenos: &agent.LenosAgentConfig{Model: "claude-sonnet-4-6"}},
	}
	args := buildLenosArgs(req, a, "/other")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "--model claude-sonnet-4-6") {
		t.Errorf("expected --model claude-sonnet-4-6, got %q", got)
	}
	if !strings.Contains(got, "--cwd /other") {
		t.Errorf("expected --cwd /other, got %q", got)
	}
	if !strings.Contains(got, "--small-model") {
		t.Errorf("expected --small-model in %q", got)
	}
}

func TestBuildLenosArgs_EmptyPrompt(t *testing.T) {
	req := AgentRequest{Name: "debugger", Prompt: "", WorkingDir: "/wd"}
	a := &agent.ParsedAgent{
		Frontmatter: agent.Frontmatter{Lenos: &agent.LenosAgentConfig{}},
	}
	args := buildLenosArgs(req, a, "/wd")
	got := strings.Join(args, " ")
	if strings.Contains(got, "-- ") {
		t.Errorf("expected no '--' separator for empty prompt, got %q", got)
	}
	if !strings.Contains(got, "--small-model") {
		t.Errorf("expected --small-model in %q", got)
	}
}

func TestBuildLenosArgs_NilLenosBlock(t *testing.T) {
	req := AgentRequest{Name: "debugger", Prompt: "test", WorkingDir: "/wd"}
	a := &agent.ParsedAgent{
		Frontmatter: agent.Frontmatter{},
	}
	args := buildLenosArgs(req, a, "/wd")
	got := strings.Join(args, " ")
	if strings.Contains(got, "--model ") {
		t.Errorf("expected no --model with nil Lenos block, got %q", got)
	}
	if !strings.Contains(got, "--small-model") {
		t.Errorf("expected --small-model in %q", got)
	}
}

func TestBuildLenosArgs_Access(t *testing.T) {
	tests := []struct {
		name   string
		access string
		wantRO bool
	}{
		{"access=ro appends --readonly", "ro", true},
		{"access=rw omits --readonly", "rw", false},
		{"access=empty omits --readonly", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := AgentRequest{Name: "debugger", Prompt: "hi", WorkingDir: "/wd"}
			a := &agent.ParsedAgent{
				Frontmatter: agent.Frontmatter{Lenos: &agent.LenosAgentConfig{Access: tt.access}},
			}
			args := buildLenosArgs(req, a, "/wd")
			got := strings.Join(args, " ")
			if tt.wantRO && !strings.Contains(got, "--readonly") {
				t.Errorf("expected --readonly, got %q", got)
			}
			if !tt.wantRO && strings.Contains(got, "--readonly") {
				t.Errorf("expected NO --readonly, got %q", got)
			}
			if !strings.Contains(got, "--small-model") {
				t.Errorf("expected --small-model in %q", got)
			}
		})
	}
}

// TestRunLenos_Success verifies a successful lenos run returns the stdout output.
func TestRunLenos_Success(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lenos")
	script := "#!/bin/sh\necho \"$@\""
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	agentDir := t.TempDir()
	agentFile := filepath.Join(agentDir, "debugger.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: debugger\nlenos:\n  access: ro\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.EinaiConfig{AgentsPaths: []string{agentDir}}
	req := AgentRequest{Name: "debugger", Prompt: "do it", WorkingDir: tmpDir}

	resp, err := RunLenos(context.Background(), req, cfg)
	if err != nil {
		t.Fatalf("RunLenos() unexpected error: %v", err)
	}
	if !strings.Contains(resp.Result, "--small-model") {
		t.Errorf("expected --small-model in lenos argv, got %q", resp.Result)
	}
	// DurationMs may be 0 on fast CI runners — non-negative is sufficient.
	if resp.DurationMs < 0 {
		t.Errorf("DurationMs = %d, want >= 0", resp.DurationMs)
	}
}

// TestRunLenos_NonZeroExit verifies error on non-zero exit from lenos.
func TestRunLenos_NonZeroExit(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lenos")
	script := "#!/bin/sh\necho 'partial' >&2\nexit 1"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	agentDir := t.TempDir()
	agentFile := filepath.Join(agentDir, "debugger.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: debugger\nlenos:\n  access: ro\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.EinaiConfig{AgentsPaths: []string{agentDir}}
	req := AgentRequest{Name: "debugger", Prompt: "do it", WorkingDir: tmpDir}

	_, err := RunLenos(context.Background(), req, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "lenos exited 1") {
		t.Errorf("error = %q, want to contain 'lenos exited 1'", err.Error())
	}
}

// TestRunLenos_NotOnPATH verifies error when lenos binary is not on PATH.
func TestRunLenos_NotOnPATH(t *testing.T) {
	tmpDir := t.TempDir()
	// Set PATH to empty, no lenos available
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	agentDir := t.TempDir()
	agentFile := filepath.Join(agentDir, "debugger.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: debugger\nlenos:\n  access: ro\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.EinaiConfig{AgentsPaths: []string{agentDir}}
	req := AgentRequest{Name: "debugger", Prompt: "hi", WorkingDir: tmpDir}

	_, err := RunLenos(context.Background(), req, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// exec.ErrNotFound is a specific error type; check for relevant message
	if !strings.Contains(err.Error(), "executable file not found") &&
		!strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, expected 'not found' message", err.Error())
	}
}

// TestRunLenos_EmptyStdout verifies empty output is valid.
func TestRunLenos_EmptyStdout(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lenos")
	script := "#!/bin/sh\nexit 0"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	agentDir := t.TempDir()
	agentFile := filepath.Join(agentDir, "debugger.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: debugger\nlenos:\n  access: ro\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.EinaiConfig{AgentsPaths: []string{agentDir}}
	req := AgentRequest{Name: "debugger", Prompt: "hi", WorkingDir: tmpDir}

	resp, err := RunLenos(context.Background(), req, cfg)
	if err != nil {
		t.Fatalf("RunLenos() unexpected error: %v", err)
	}
	if resp.Result != "" {
		t.Errorf("Result = %q, want empty string", resp.Result)
	}
}

// TestRunLenos_SetsEnv verifies LENOS_AGENTS_DIR is set on the subprocess.
func TestRunLenos_SetsEnv(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lenos")
	script := "#!/bin/sh\necho \"LENOS_AGENTS_DIR=$LENOS_AGENTS_DIR\""
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	agentDir := t.TempDir()
	agentFile := filepath.Join(agentDir, "debugger.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: debugger\nlenos:\n  access: ro\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.EinaiConfig{AgentsPaths: []string{agentDir}}
	req := AgentRequest{Name: "debugger", Prompt: "hi", WorkingDir: tmpDir}

	resp, err := RunLenos(context.Background(), req, cfg)
	if err != nil {
		t.Fatalf("RunLenos() unexpected error: %v", err)
	}
	if !strings.Contains(resp.Result, "LENOS_AGENTS_DIR=") {
		t.Errorf("expected LENOS_AGENTS_DIR in output, got %q", resp.Result)
	}
}
