package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tta-lab/einai/internal/config"
)

// writeMockClaude writes a shell script mock for the `claude` binary and returns
// the directory path. Callers must prepend it to PATH.
func writeMockClaude(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("write mock claude: %v", err)
	}
	return dir
}

func prependPATH(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) }) //nolint:errcheck
	os.Setenv("PATH", dir+":"+orig)               //nolint:errcheck
}

func TestRunClaudeCode_Success(t *testing.T) {
	result := map[string]interface{}{
		"type":        "result",
		"subtype":     "success",
		"is_error":    false,
		"result":      "hello from CC",
		"duration_ms": int64(1234),
	}
	payload, _ := json.Marshal(result)

	// Mock claude that prints valid JSON and exits 0
	mockDir := writeMockClaude(t, "printf '%s' '"+string(payload)+"'\nexit 0\n")
	prependPATH(t, mockDir)

	// Override data dir so session logs go to temp
	tmpData := t.TempDir()
	config.SetTestDataDir(tmpData)
	defer config.ClearTestDataDir()

	req := AgentRequest{
		Name:   "test-agent",
		Prompt: "do something",
	}
	resp, err := RunClaudeCode(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("RunClaudeCode() unexpected error: %v", err)
	}
	if resp.Result != "hello from CC" {
		t.Errorf("Result = %q, want %q", resp.Result, "hello from CC")
	}
	if resp.DurationMs != 1234 {
		t.Errorf("DurationMs = %d, want 1234", resp.DurationMs)
	}
	if resp.IsError() {
		t.Error("IsError() = true, want false")
	}
}

func TestRunClaudeCode_IsErrorTrue(t *testing.T) {
	result := map[string]interface{}{
		"type":     "result",
		"is_error": true,
		"result":   "something went wrong",
	}
	payload, _ := json.Marshal(result)

	mockDir := writeMockClaude(t, "printf '%s' '"+string(payload)+"'\nexit 0\n")
	prependPATH(t, mockDir)

	tmpData := t.TempDir()
	config.SetTestDataDir(tmpData)
	defer config.ClearTestDataDir()

	req := AgentRequest{Name: "test-agent", Prompt: "do something"}
	_, err := RunClaudeCode(context.Background(), req, nil)
	if err == nil {
		t.Fatal("RunClaudeCode() expected error for is_error:true, got nil")
	}
}

func TestRunClaudeCode_NonZeroExit(t *testing.T) {
	mockDir := writeMockClaude(t, "echo 'fatal error' >&2\nexit 1\n")
	prependPATH(t, mockDir)

	tmpData := t.TempDir()
	config.SetTestDataDir(tmpData)
	defer config.ClearTestDataDir()

	req := AgentRequest{Name: "test-agent", Prompt: "do something"}
	_, err := RunClaudeCode(context.Background(), req, nil)
	if err == nil {
		t.Fatal("RunClaudeCode() expected error for non-zero exit, got nil")
	}
}

func TestRunClaudeCode_NonJSONOutput_GracefulDegradation(t *testing.T) {
	mockDir := writeMockClaude(t, "echo 'not json at all'\nexit 0\n")
	prependPATH(t, mockDir)

	tmpData := t.TempDir()
	config.SetTestDataDir(tmpData)
	defer config.ClearTestDataDir()

	req := AgentRequest{Name: "test-agent", Prompt: "do something"}
	resp, err := RunClaudeCode(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("RunClaudeCode() unexpected error for non-JSON: %v", err)
	}
	// Falls back to raw output as result
	if resp.Result == "" {
		t.Error("Result should contain raw output on non-JSON")
	}
}
