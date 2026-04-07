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
	"github.com/tta-lab/einai/internal/prompt"
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

// writeAgentFixture creates an agent .md file at dir/<name>.md with the given
// frontmatter blocks. agentName sets the agent name in frontmatter.
func writeAgentFixture(t *testing.T, dir, name, agentName, blocks string) {
	t.Helper()
	content := fmt.Sprintf(`---
name: %s
description: "test agent"
%s
---
# %s agent
`, agentName, blocks, agentName)
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write agent fixture %s: %v", path, err)
	}
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

// postAsk sends a POST /ask request to the daemon handler
// and returns the response recorder.
func postAsk(t *testing.T, d *Daemon, req session.AskRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.handleAsk(w, r)
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

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
ttal:
  access: rw`)

	d := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})

	req := session.AgentRequest{
		Name:       "coder",
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

	roDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(roDir, 0o555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	testFile := filepath.Join(roDir, "probe")
	if f, err := os.Create(testFile); err == nil {
		f.Close()
		t.Skip("running as root — cannot enforce read-only directory")
	}

	config.SetTestDataDir(roDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
ttal:
  access: rw`)

	d := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})

	req := session.AgentRequest{
		Name:       "coder",
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

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
ttal:
  access: rw`)

	d := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})

	req := session.AgentRequest{
		Name:       "coder",
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

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
ttal:
  access: rw`)

	d := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})

	req := session.AgentRequest{
		Name:       "coder",
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
	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
ttal:
  access: rw`)

	d := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})

	req := session.AgentRequest{
		Name:       "nonexistent-agent",
		Prompt:     "hello",
		WorkingDir: t.TempDir(),
		Runtime:    "claude-code",
		Async:      false,
	}
	w := postAgentRun(t, d, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 for unknown agent in sync path, got 200")
	}
}

// TestHandleAgentRun_AsyncValidationFails verifies that an unknown agent name
// returns 500 with an error, no .sh is written, and no pueue submit is attempted.
func TestHandleAgentRun_AsyncValidationFails(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
ttal:
  access: rw`)

	d := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})

	req := session.AgentRequest{
		Name:       "nosuchagent",
		Prompt:     "hello",
		WorkingDir: tmpDir,
		Runtime:    "claude-code",
		Async:      true,
	}
	w := postAgentRun(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Errorf("response body %q does not mention 'not found'", w.Body.String())
	}

	// Verify no .sh was written
	jobsDir := filepath.Join(tmpDir, "jobs")
	entries, _ := os.ReadDir(jobsDir)
	for _, e := range entries {
		scripts, _ := os.ReadDir(filepath.Join(jobsDir, e.Name()))
		for _, s := range scripts {
			if strings.HasSuffix(s.Name(), ".sh") {
				t.Errorf("unexpected .sh file in jobs dir: %s/%s", e.Name(), s.Name())
			}
		}
	}
}

// TestHandleAgentRun_AsyncEiNativeMissingTtalBlockFails verifies that an agent
// with no ttal: block returns 500 when Runtime='ei-native'.
func TestHandleAgentRun_AsyncEiNativeMissingTtalBlockFails(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	agentDir := t.TempDir()
	// Agent with only claude-code block, no ttal block
	writeAgentFixture(t, agentDir, "cc_only", "cc_only", `claude-code:
  model: sonnet`)

	d := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})

	req := session.AgentRequest{
		Name:       "cc_only",
		Prompt:     "hello",
		WorkingDir: tmpDir,
		Runtime:    "ei-native",
		Async:      true,
	}
	w := postAgentRun(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no ttal: block") {
		t.Errorf("response body %q does not mention 'no ttal: block'", w.Body.String())
	}
}

// TestHandleAgentRun_AsyncBothProjectAndRepoFails verifies that setting both
// --project and --repo returns 500 due to mutual exclusion in resolveAgentCWD.
func TestHandleAgentRun_AsyncBothProjectAndRepoFails(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
ttal:
  access: rw`)

	d := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})

	req := session.AgentRequest{
		Name:       "coder",
		Prompt:     "hello",
		Project:    "some-project",
		Repo:       "some/repo",
		WorkingDir: tmpDir,
		Runtime:    "ei-native",
		Async:      true,
	}
	w := postAgentRun(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "mutually exclusive") {
		t.Errorf("response body %q does not mention 'mutually exclusive'", w.Body.String())
	}
}

// TestHandleAsk_AsyncWebModeSuccess verifies that an async ask in web mode
// (no resolution needed) returns 200 and writes the job script.
func TestHandleAsk_AsyncWebModeSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	d := New(&config.EinaiConfig{})

	// ModeWeb requires no resolution — no filesystem or network dependencies.
	req := session.AskRequest{
		Question:   "what is the weather today?",
		Mode:       prompt.ModeWeb,
		Async:      true,
		WorkingDir: tmpDir,
	}
	w := postAsk(t, d, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp session.AskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error in response: %s", resp.Error)
	}

	jobsDir := filepath.Join(tmpDir, "jobs", "ask")
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		t.Fatalf("read jobs dir %q: %v", jobsDir, err)
	}
	if len(entries) == 0 {
		t.Error("no job scripts written to jobs dir")
	}
}

// TestHandleAsk_AsyncValidationFails_Project verifies that an async ask with
// a nonexistent project alias returns 500 with an error and no .sh is written.
func TestHandleAsk_AsyncValidationFails_Project(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	d := New(&config.EinaiConfig{})

	req := session.AskRequest{
		Question:   "hello",
		Mode:       prompt.ModeProject,
		Project:    "definitely-not-a-project",
		Async:      true,
		WorkingDir: tmpDir,
	}
	w := postAsk(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not found") && !strings.Contains(w.Body.String(), "does not exist") {
		t.Errorf("response body %q does not mention project resolution error", w.Body.String())
	}

	jobsDir := filepath.Join(tmpDir, "jobs")
	entries, _ := os.ReadDir(jobsDir)
	for _, e := range entries {
		scripts, _ := os.ReadDir(filepath.Join(jobsDir, e.Name()))
		for _, s := range scripts {
			if strings.HasSuffix(s.Name(), ".sh") {
				t.Errorf("unexpected .sh file in jobs dir: %s/%s", e.Name(), s.Name())
			}
		}
	}
}

// TestHandleAsk_AsyncValidationFails_RepoRef verifies that an async ask with
// an invalid repo ref returns 500.
func TestHandleAsk_AsyncValidationFails_RepoRef(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	d := New(&config.EinaiConfig{})

	req := session.AskRequest{
		Question:   "hello",
		Mode:       prompt.ModeRepo,
		Repo:       "bad/ref/that/wont/parse",
		Async:      true,
		WorkingDir: tmpDir,
	}
	w := postAsk(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// TestHandleAsk_AsyncValidationFails_URLModeEmptyURL verifies that an async ask
// in URL mode with an empty URL returns 500.
func TestHandleAsk_AsyncValidationFails_URLModeEmptyURL(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	binDir := t.TempDir()
	writeFakePueue(t, binDir, "", 0)

	d := New(&config.EinaiConfig{})

	req := session.AskRequest{
		Question:   "hello",
		Mode:       prompt.ModeURL,
		URL:        "",
		Async:      true,
		WorkingDir: tmpDir,
	}
	w := postAsk(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "--url required") {
		t.Errorf("response body %q does not mention '--url required'", w.Body.String())
	}
}
