package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/jobqueue"
)

func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	tmpDir := t.TempDir()
	config.SetTestDataDir(tmpDir)
	t.Cleanup(config.ClearTestDataDir)

	// Delete any existing queue to ensure test isolation.
	qp := filepath.Join(tmpDir, "queue.jsonl")
	os.Remove(qp)

	cfg := &config.EinaiConfig{}
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

func TestHandleJobList(t *testing.T) {
	d := newTestDaemon(t)
	q := d.queue

	q.Enqueue(jobqueueTestSpec("agent", "coder"))
	q.Enqueue(jobqueueTestSpec("agent", "athena"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /job/list", d.handleJobList)

	req := httptest.NewRequest("GET", "/job/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	jobs, ok := resp["jobs"].([]any)
	if !ok {
		t.Fatalf("jobs is not a list")
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestHandleJobList_Limit(t *testing.T) {
	d := newTestDaemon(t)
	q := d.queue

	for i := 0; i < 5; i++ {
		q.Enqueue(jobqueueTestSpec("agent", fmt.Sprintf("agent%d", i)))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /job/list", d.handleJobList)

	req := httptest.NewRequest("GET", "/job/list?limit=2", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	jobs := resp["jobs"].([]any)
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs with limit=2, got %d", len(jobs))
	}
}

func TestHandleJobLog_NotFound(t *testing.T) {
	d := newTestDaemon(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /job/log", d.handleJobLog)

	req := httptest.NewRequest("GET", "/job/log?id=999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleJobLog_MissingID(t *testing.T) {
	d := newTestDaemon(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /job/log", d.handleJobLog)

	req := httptest.NewRequest("GET", "/job/log", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleJobKill_NotFound(t *testing.T) {
	d := newTestDaemon(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /job/kill", d.handleJobKill)

	req := httptest.NewRequest("POST", "/job/kill?id=999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleJobKill_QueuedJob(t *testing.T) {
	d := newTestDaemon(t)
	q := d.queue

	job, err := q.Enqueue(jobqueueTestSpec("agent", "coder"))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /job/kill", d.handleJobKill)

	req := httptest.NewRequest("POST", "/job/kill?id="+fmt.Sprintf("%d", job.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	j, _ := q.Get(job.ID)
	if j.State != jobqueue.StateKilled {
		t.Errorf("expected StateKilled, got %v", j.State)
	}
}

func TestHandleJobKill_MissingID(t *testing.T) {
	d := newTestDaemon(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /job/kill", d.handleJobKill)

	req := httptest.NewRequest("POST", "/job/kill", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func jobqueueTestSpec(kind, agent string) jobqueue.EnqueueSpec {
	return jobqueue.EnqueueSpec{
		Kind:       kind,
		Agent:      agent,
		Runtime:    "ei-native",
		Prompt:     "test prompt",
		Stem:       "test-stem",
		OutputPath: filepath.Join(os.TempDir(), "test-output-"+agent+".md"),
	}
}
