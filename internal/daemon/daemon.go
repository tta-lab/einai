package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/jobqueue"
	"github.com/tta-lab/einai/internal/prompt"
	rt "github.com/tta-lab/einai/internal/runtime"
	"github.com/tta-lab/einai/internal/session"
)

const socketPerms = 0o600

// Daemon is the einai unix-socket HTTP server.
type Daemon struct {
	cfg        *config.EinaiConfig
	socketPath string
	server     *http.Server
	queue      *jobqueue.Queue
	worker     *jobqueue.Worker
}

// New creates a new Daemon instance.
func New(cfg *config.EinaiConfig) (*Daemon, error) {
	socketPath := filepath.Join(config.DefaultDataDir(), "daemon.sock")

	queuePath := filepath.Join(config.DefaultDataDir(), "queue.jsonl")
	q, err := jobqueue.New(queuePath)
	if err != nil {
		return nil, fmt.Errorf("create job queue: %w", err)
	}

	// TODO: use cfg.MaxParallel() after config subtask lands
	maxParallel := 4
	w := jobqueue.NewWorker(q, maxParallel)

	d := &Daemon{
		cfg:        cfg,
		socketPath: socketPath,
		queue:      q,
		worker:     w,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.handleHealth)
	mux.HandleFunc("POST /ask", d.handleAsk)
	mux.HandleFunc("POST /agent/run", d.handleAgentRun)
	mux.HandleFunc("GET /job/list", d.handleJobList)
	mux.HandleFunc("GET /job/log", d.handleJobLog)
	mux.HandleFunc("POST /job/kill", d.handleJobKill)
	d.server = &http.Server{
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return d, nil
}

// Run starts the daemon and blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(d.socketPath), 0o755); err != nil {
		return fmt.Errorf("create daemon dir: %w", err)
	}
	if err := os.Remove(d.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.socketPath, err)
	}
	if err := os.Chmod(d.socketPath, socketPerms); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}

	slog.Info("daemon listening", "socket", d.socketPath)

	// Start the worker scheduler.
	workerCtx, workerCancel := context.WithCancel(ctx)
	go d.worker.Start(workerCtx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		// Stop accepting new jobs.
		workerCancel()
		// Graceful shutdown: wait for running jobs (up to 30s).
		done := make(chan struct{})
		go func() {
			d.worker.Stop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			slog.Warn("shutdown timeout: some jobs may still be running")
		}
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return d.server.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

func (d *Daemon) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (d *Daemon) handleAsk(w http.ResponseWriter, r *http.Request) {
	var req session.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Async {
		if _, err := session.ResolveAskParams(r.Context(), req, d.cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, session.AskResponse{Error: err.Error()})
			return
		}
		if err := d.handleAskAsync(req); err != nil {
			writeJSON(w, http.StatusInternalServerError, session.AskResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, session.AskResponse{})
		return
	}

	d.runAsk(w, r, req)
}

func (d *Daemon) handleAgentRun(w http.ResponseWriter, r *http.Request) {
	var req session.AgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Async {
		if err := session.ValidateAgentRequest(r.Context(), req, d.cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, session.AgentResponse{Error: err.Error()})
			return
		}
		if err := d.handleAgentRunAsync(req); err != nil {
			writeJSON(w, http.StatusInternalServerError, session.AgentResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, session.AgentResponse{})
		return
	}

	d.runAgent(w, r, req)
}

func (d *Daemon) runAsk(w http.ResponseWriter, r *http.Request, req session.AskRequest) {
	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.AgentMaxRunTimeout())
	defer cancel()

	resp, err := session.RunAsk(ctx, req, d.cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, session.AskResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (d *Daemon) runAgent(w http.ResponseWriter, r *http.Request, req session.AgentRequest) {
	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.AgentMaxRunTimeout())
	defer cancel()

	resp, err := session.RunAgent(ctx, req, d.cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, session.AgentResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAgentRunAsync enqueues an agent run job and returns immediately.
func (d *Daemon) handleAgentRunAsync(req session.AgentRequest) error {
	slog.Info("async agent run request received", "agent", req.Name, "runtime", req.Runtime)

	rawRuntime := req.Runtime
	if rawRuntime == "" {
		rawRuntime = d.cfg.AgentDefaultRuntime()
	}
	resolved, err := rt.Parse(rawRuntime)
	if err != nil {
		return fmt.Errorf("resolve runtime: %w", err)
	}
	runtimeStr := string(resolved)

	stem := session.SessionLogName(req.WorkingDir, req.Name)
	outputPath := filepath.Join(config.DefaultDataDir(), "outputs", runtimeStr, stem+".md")

	_, err = d.queue.Enqueue(jobqueue.EnqueueSpec{
		Kind:       "agent",
		Agent:      req.Name,
		Runtime:    runtimeStr,
		Prompt:     req.Prompt,
		WorkingDir: req.WorkingDir,
		SendTarget: req.SendTarget,
		Stem:       stem,
		OutputPath: outputPath,
	})
	return err
}

// handleAskAsync enqueues an ask job and returns immediately.
func (d *Daemon) handleAskAsync(req session.AskRequest) error {
	slog.Info("async ask request received", "mode", req.Mode, "working_dir", req.WorkingDir)

	stem := session.SessionLogName(req.WorkingDir, "ask")
	outputPath := filepath.Join(config.DefaultDataDir(), "outputs", "ask", stem+".md")

	_, err := d.queue.Enqueue(jobqueue.EnqueueSpec{
		Kind:       "ask",
		Agent:      "ask",
		Runtime:    "ei-native",
		Prompt:     req.Question,
		WorkingDir: req.WorkingDir,
		SendTarget: req.SendTarget,
		Stem:       stem,
		OutputPath: outputPath,
		AskSpec: &jobqueue.AskSpec{
			Question: req.Question,
			Mode:     modeToString(req.Mode),
			Project:  req.Project,
			Repo:     req.Repo,
			URL:      req.URL,
			Save:     req.Save,
		},
	})
	return err
}

// modeToString converts a prompt.Mode to its string representation.
func modeToString(m prompt.Mode) string {
	switch m {
	case prompt.ModeProject:
		return "project"
	case prompt.ModeRepo:
		return "repo"
	case prompt.ModeURL:
		return "url"
	case prompt.ModeWeb:
		return "web"
	default:
		return "general"
	}
}
