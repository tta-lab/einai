package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tta-lab/einai/internal/config"
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

// TestHandleAgentRun_AsyncEnsureGroupFails is no longer applicable
// since we no longer use pueue. Removed per jobqueue rewire.
// TestHandleAgentRun_AsyncSubmitFails is no longer applicable
// since we no longer use pueue. Removed per jobqueue rewire.
// TestHandleAgentRun_SyncPathUnchanged verifies that non-async requests still
// follow the blocking path (no pueue involvement), returning an error when the
// agent runtime is unreachable.
func TestHandleAgentRun_SyncPathUnchanged(t *testing.T) {
	d, err := New(&config.EinaiConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := session.AgentRequest{
		Name:       "nonexistent-agent",
		Prompt:     "hello",
		WorkingDir: t.TempDir(),
		Runtime:    "claude-code",
	}
	w := postAgentRun(t, d, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 for unknown agent in sync path, got 200")
	}
}

