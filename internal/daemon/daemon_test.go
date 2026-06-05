package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/session"
)

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
