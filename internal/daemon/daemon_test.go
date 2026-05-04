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
	"github.com/tta-lab/einai/internal/jobqueue"
	"github.com/tta-lab/einai/internal/session"
)

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

// TestHandleAgentRun_AsyncSuccess verifies the async path returns 200 and
// enqueues a job in StateQueued.
func TestHandleAgentRun_AsyncSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
lenos:
  access: rw`)

	d, err := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

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

	// Verify job was queued.
	jobs := d.queue.List(0)
	if len(jobs) != 1 {
		t.Errorf("expected 1 queued job, got %d", len(jobs))
	}
	if jobs[0].State != jobqueue.StateQueued {
		t.Errorf("expected StateQueued, got %v", jobs[0].State)
	}
	if jobs[0].Agent != req.Name {
		t.Errorf("expected Agent=%s, got %s", req.Name, jobs[0].Agent)
	}
	if jobs[0].OutputPath == "" {
		t.Error("OutputPath should be set")
	}
}

// TestHandleAgentRun_AsyncEnqueueFailure verifies that a queue write failure
// (e.g. read-only data dir) returns a 500 with an error body.
func TestHandleAgentRun_AsyncEnqueueFailure(t *testing.T) {
	tmpDir := t.TempDir()

	roDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(roDir, 0o555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Skip if we can't enforce read-only (likely root).
	if f, err := os.Create(filepath.Join(roDir, "probe")); err == nil {
		f.Close()
		os.Remove(filepath.Join(roDir, "probe"))
		t.Skip("running as root — cannot enforce read-only directory")
	}

	config.SetTestDataDir(roDir)
	t.Cleanup(config.ClearTestDataDir)

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
lenos:
  access: rw`)

	d, err := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

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

// TestHandleAgentRun_AsyncEnsureGroupFails is no longer applicable
// since we no longer use pueue. Removed per jobqueue rewire.
// TestHandleAgentRun_AsyncSubmitFails is no longer applicable
// since we no longer use pueue. Removed per jobqueue rewire.
// TestHandleAgentRun_SyncPathUnchanged verifies that non-async requests still
// follow the blocking path (no pueue involvement), returning an error when the
// agent runtime is unreachable.
func TestHandleAgentRun_SyncPathUnchanged(t *testing.T) {
	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
lenos:
  access: rw`)

	d, err := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

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

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
lenos:
  access: rw`)

	d, err := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

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

// TestHandleAgentRun_AsyncLenosMissingLenosBlockFails verifies that an agent
// with no lenos: block returns 500 when Runtime='lenos'.
func TestHandleAgentRun_AsyncLenosMissingLenosBlockFails(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	agentDir := t.TempDir()
	// Agent with only claude-code block, no ttal block
	writeAgentFixture(t, agentDir, "cc_only", "cc_only", `claude-code:
  model: sonnet`)

	d, err := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := session.AgentRequest{
		Name:       "cc_only",
		Prompt:     "hello",
		WorkingDir: tmpDir,
		Runtime:    "lenos",
		Async:      true,
	}
	w := postAgentRun(t, d, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no lenos: block") {
		t.Errorf("response body %q does not mention 'no lenos: block'", w.Body.String())
	}
}

// TestHandleAgentRun_AsyncBothProjectAndRepoFails verifies that setting both
// --project and --repo returns 500 due to mutual exclusion in resolveAgentCWD.
func TestHandleAgentRun_AsyncBothProjectAndRepoFails(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	agentDir := t.TempDir()
	writeAgentFixture(t, agentDir, "coder", "coder", `claude-code:
  model: sonnet
lenos:
  access: rw`)

	d, err := New(&config.EinaiConfig{AgentsPaths: []string{agentDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := session.AgentRequest{
		Name:       "coder",
		Prompt:     "hello",
		Project:    "some-project",
		Repo:       "some/repo",
		WorkingDir: tmpDir,
		Runtime:    "lenos",
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

	d, err := New(&config.EinaiConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// ModeWeb requires no resolution — no filesystem or network dependencies.
	req := session.AskRequest{
		Question:   "what is the weather today?",
		Mode:       session.ModeWeb,
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

	// Verify job was queued.
	jobs := d.queue.List(0)
	if len(jobs) != 1 {
		t.Errorf("expected 1 queued job, got %d", len(jobs))
	}
	if len(jobs) > 0 && jobs[0].State != jobqueue.StateQueued {
		t.Errorf("expected StateQueued, got %v", jobs[0].State)
	}
}

// TestHandleAsk_AsyncValidationFails_Project verifies that an async ask with
// a nonexistent project alias returns 500 with an error and no .sh is written.
func TestHandleAsk_AsyncValidationFails_Project(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	d, err := New(&config.EinaiConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := session.AskRequest{
		Question:   "hello",
		Mode:       session.ModeProject,
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

	d, err := New(&config.EinaiConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := session.AskRequest{
		Question:   "hello",
		Mode:       session.ModeRepo,
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

	d, err := New(&config.EinaiConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := session.AskRequest{
		Question:   "hello",
		Mode:       session.ModeURL,
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
