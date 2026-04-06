package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/session"
)

// writeFakePueue writes a fake pueue script to dir and prepends dir to PATH.
// The script echoes jobID for "add" commands and exits exitCode for other commands.
// For error tests, pass exitCode=1 and set which subcommand should fail via failCmd.
func writeFakePueue(t *testing.T, dir, failCmd string, failCode int) {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "add" ]; then
  if [ "%s" = "add" ]; then echo "pueue add failed" >&2; exit %d; fi
  echo 7
  exit 0
fi
if [ "$1" = "parallel" ]; then
  if [ "%s" = "parallel" ]; then echo "pueue parallel failed" >&2; exit %d; fi
  exit 0
fi
exit 0
`, failCmd, failCode, failCmd, failCode)
	fakePueue := filepath.Join(dir, "pueue")
	if err := os.WriteFile(fakePueue, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pueue: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", dir+":"+oldPath)               //nolint:errcheck
}

// postAgentRun sends a POST /agent/run request to the daemon handler
// and returns the response recorder.
func postAgentRun(t *testing.T, d *Daemon, req session.AgentRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/agent/run", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.handleAgentRun(w, r)
	return w
}

// TestHandleAgentRun_AsyncSuccess verifies the async path returns 200 with an
// empty AgentResponse and writes the job script to disk.
func TestHandleAgentRun_AsyncSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	d := New(&config.EinaiConfig{})

	req := session.AgentRequest{
		Name:       "testagent",
		Prompt:     "do something",
		WorkingDir: tmpDir,
		Runtime:    "claude-code",
		Async:      true,
		SendTarget: "",
	}
	w := postAgentRun(t, d, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp session.AgentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error in response: %s", resp.Error)
	}

	// Verify the job script was written under the jobs directory.
	jobsDir := filepath.Join(tmpDir, "jobs", "claude-code")
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		t.Fatalf("read jobs dir %q: %v", jobsDir, err)
	}
	if len(entries) == 0 {
		t.Error("no job scripts written to jobs dir")
	}
}

// TestHandleAgentRun_AsyncWriteJobScriptFails verifies that a write failure
// (e.g. read-only job dir) returns a 500 with an error body.
func TestHandleAgentRun_AsyncWriteJobScriptFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Make the jobs dir read-only so WriteJobScript fails.
	roDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(roDir, 0o555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Skip if running as root (can't enforce permissions).
	testFile := filepath.Join(roDir, "probe")
	if f, err := os.Create(testFile); err == nil {
		f.Close()
		t.Skip("running as root — cannot enforce read-only directory")
	}

	config.SetTestDataDir(roDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	d := New(&config.EinaiConfig{})

	req := session.AgentRequest{
		Name:       "testagent",
		Prompt:     "hello",
		WorkingDir: tmpDir,
		Runtime:    "claude-code",
		Async:      true,
	}
	w := postAgentRun(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "write job script") {
		t.Errorf("response body %q does not mention 'write job script'", w.Body.String())
	}
}

// TestHandleAgentRun_AsyncEnsureGroupFails verifies that a pueue parallel
// failure propagates as a 500 error.
func TestHandleAgentRun_AsyncEnsureGroupFails(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "parallel", 1)

	d := New(&config.EinaiConfig{})

	req := session.AgentRequest{
		Name:       "testagent",
		Prompt:     "hello",
		WorkingDir: tmpDir,
		Runtime:    "claude-code",
		Async:      true,
	}
	w := postAgentRun(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// TestHandleAgentRun_AsyncSubmitFails verifies that a pueue add failure
// propagates as a 500 error.
func TestHandleAgentRun_AsyncSubmitFails(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "add", 1)

	d := New(&config.EinaiConfig{})

	req := session.AgentRequest{
		Name:       "testagent",
		Prompt:     "hello",
		WorkingDir: tmpDir,
		Runtime:    "claude-code",
		Async:      true,
	}
	w := postAgentRun(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// TestHandleAgentRun_SyncPathUnchanged verifies that non-async requests still
// follow the blocking path (no pueue involvement), returning an error when the
// agent runtime is unreachable.
func TestHandleAgentRun_SyncPathUnchanged(t *testing.T) {
	d := New(&config.EinaiConfig{})

	req := session.AgentRequest{
		Name:       "nonexistent-agent",
		Prompt:     "hello",
		WorkingDir: t.TempDir(),
		Runtime:    "claude-code",
		Async:      false,
	}
	w := postAgentRun(t, d, req)

	// Sync path fails because agent doesn't exist — the key assertion is that
	// it went through the blocking path (not async), which is evidenced by
	// attempting agent discovery rather than touching pueue.
	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 for unknown agent in sync path, got 200")
	}
}
