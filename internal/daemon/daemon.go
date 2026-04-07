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
	"github.com/tta-lab/einai/internal/prompt"
	"github.com/tta-lab/einai/internal/pueue"
	"github.com/tta-lab/einai/internal/ratelimit"
	rt "github.com/tta-lab/einai/internal/runtime"
	"github.com/tta-lab/einai/internal/session"
)

const socketPerms = 0o600

// Daemon is the einai unix-socket HTTP server.
type Daemon struct {
	cfg        *config.EinaiConfig
	socketPath string
	server     *http.Server
	limiter    *ratelimit.Limiter
}

// New creates a new Daemon instance.
func New(cfg *config.EinaiConfig) *Daemon {
	socketPath := filepath.Join(config.DefaultDataDir(), "daemon.sock")
	limiter := ratelimit.New(ratelimit.Config{
		RequestsPerMinute:  cfg.RateLimitRequestsPerMinute(),
		ConcurrentSessions: cfg.RateLimitConcurrentSessions(),
	})
	d := &Daemon{
		cfg:        cfg,
		socketPath: socketPath,
		limiter:    limiter,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.handleHealth)
	mux.HandleFunc("POST /ask", d.handleAsk)
	mux.HandleFunc("POST /agent/run", d.handleAgentRun)
	d.server = &http.Server{
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		// WriteTimeout is intentionally omitted: agent runs can take many minutes.
		// The async path returns quickly (<1s) so slow writes there indicate a
		// real problem. Use a shorter timeout on the async handler itself instead.
	}
	return d
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

// checkRateLimit verifies rate and concurrency limits.
func (d *Daemon) checkRateLimit(w http.ResponseWriter) bool {
	if allowed, retryAfter := d.limiter.Allow(); !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return false
	}
	if !d.limiter.Acquire() {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "too many concurrent sessions", http.StatusTooManyRequests)
		return false
	}
	return true
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
// A Flush is performed after encoding so clients do not wait for the keep-alive
// interval before receiving a short response (e.g. the empty {} for async success).
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
	if !d.checkRateLimit(w) {
		d.limiter.Release() // early exit: checkRateLimit acquired a slot
		return
	}
	// Always release on exit, regardless of which branch handles the request.
	defer d.limiter.Release()

	var req session.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Async {
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
	if !d.checkRateLimit(w) {
		d.limiter.Release() // early exit: checkRateLimit acquired a slot
		return
	}
	// Always release on exit, regardless of which branch handles the request.
	defer d.limiter.Release()

	var req session.AgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Async {
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

// handleAgentRunAsync writes a pueue job script and submits it, returning
// immediately. The job will notify via ttal send on completion.
func (d *Daemon) handleAgentRunAsync(req session.AgentRequest) error {
	slog.Info("async agent run request received", "agent", req.Name, "runtime", req.Runtime)

	rawRuntime := req.Runtime
	if rawRuntime == "" {
		rawRuntime = d.cfg.AgentDefaultRuntime()
	}
	resolved, err := rt.Parse(rawRuntime)
	if err != nil {
		slog.Error("failed to resolve runtime for async agent", "error", err, "agent", req.Name)
		return fmt.Errorf("resolve runtime: %w", err)
	}
	runtimeStr := string(resolved)

	stem := session.SessionLogName(req.WorkingDir, req.Name)
	outputPath := filepath.Join(config.DefaultDataDir(), "outputs", runtimeStr, stem+".md")

	scriptPath, err := session.WriteJobScript(session.JobScriptOpts{
		Prompt:     req.Prompt,
		AgentName:  req.Name,
		Runtime:    runtimeStr,
		Stem:       stem,
		OutputPath: outputPath,
		SendTarget: req.SendTarget,
		WorkingDir: req.WorkingDir,
	})
	if err != nil {
		slog.Error("failed to write agent job script", "error", err, "agent", req.Name)
		return fmt.Errorf("write job script: %w", err)
	}

	return d.submitAsync(scriptPath, req.Name, outputPath)
}

// handleAskAsync writes a pueue job script and submits it, returning
// immediately. The job will notify via ttal send on completion.
func (d *Daemon) handleAskAsync(req session.AskRequest) error {
	slog.Info("async ask request received", "mode", req.Mode, "working_dir", req.WorkingDir)

	stem := session.SessionLogName(req.WorkingDir, "ask")
	outputPath := filepath.Join(config.DefaultDataDir(), "outputs", "ask", stem+".md")

	scriptPath, err := session.WriteAskJobScript(session.AskScriptOpts{
		Question:   req.Question,
		Stem:       stem,
		OutputPath: outputPath,
		SendTarget: req.SendTarget,
		WorkingDir: req.WorkingDir,
		Mode:       modeToString(req.Mode),
		Project:    req.Project,
		Repo:       req.Repo,
		URL:        req.URL,
		Save:       req.Save,
	})
	if err != nil {
		slog.Error("failed to write ask job script", "error", err, "working_dir", req.WorkingDir)
		return fmt.Errorf("write ask job script: %w", err)
	}

	return d.submitAsync(scriptPath, "ask", outputPath)
}

// submitAsync ensures the pueue group exists and submits the job script.
func (d *Daemon) submitAsync(scriptPath, label, outputPath string) error {
	slog.Info("submitting async job", "label", label, "script", scriptPath)

	group := d.cfg.PueueGroup()
	if err := pueue.EnsureGroup(group, d.cfg.PueueParallel()); err != nil {
		slog.Error("failed to ensure pueue group", "error", err, "group", group)
		return fmt.Errorf("ensure pueue group: %w", err)
	}

	jobID, err := pueue.Submit(pueue.SubmitOpts{
		Group:      group,
		ScriptPath: scriptPath,
		Label:      label,
	})
	if err != nil {
		slog.Error("failed to submit pueue job", "error", err, "label", label)
		return fmt.Errorf("submit pueue job: %w", err)
	}

	slog.Info("async job queued", "label", label, "job_id", jobID, "output", outputPath)
	return nil
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
