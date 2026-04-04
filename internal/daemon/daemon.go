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
	"github.com/tta-lab/einai/internal/ratelimit"
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
		Handler:     mux,
		ReadTimeout: 30 * time.Second,
		// No WriteTimeout — agent runs can take many minutes.
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
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func (d *Daemon) handleAsk(w http.ResponseWriter, r *http.Request) {
	if !d.checkRateLimit(w) {
		return
	}
	defer d.limiter.Release()

	var req session.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.AgentMaxRunTimeout())
	defer cancel()

	resp, err := session.RunAsk(ctx, req, d.cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, session.AskResponse{
			Error: err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (d *Daemon) handleAgentRun(w http.ResponseWriter, r *http.Request) {
	if !d.checkRateLimit(w) {
		return
	}
	defer d.limiter.Release()

	var req session.AgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.AgentMaxRunTimeout())
	defer cancel()

	resp, err := session.RunAgent(ctx, req, d.cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, session.AgentResponse{
			Error: err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
