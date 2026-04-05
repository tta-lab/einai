package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/tta-lab/einai/internal/config"
)

// ccResult is the JSON output schema from `claude -p --output-format json`.
type ccResult struct {
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	IsError      bool    `json:"is_error"`
	Result       string  `json:"result"`
	DurationMs   int64   `json:"duration_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	SessionID    string  `json:"session_id"`
	StopReason   string  `json:"stop_reason"`
}

// RunClaudeCode executes the agent by spawning `claude -p --output-format json`.
// It passes stdin through to CC and the positional prompt as a CC argument.
// Full JSON output is saved to ~/.einai/sessions/cc/<timestamp>-<project>.json.
func RunClaudeCode(ctx context.Context, req AgentRequest, _ *config.EinaiConfig) (*AgentResponse, error) {
	start := time.Now()

	// Build claude command: claude -p --agent <name> --output-format json --dangerously-skip-permissions
	args := []string{
		"-p",
		"--agent", req.Name,
		"--output-format", "json",
		"--dangerously-skip-permissions",
	}
	if req.Prompt != "" {
		args = append(args, req.Prompt)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)

	// Set working directory
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	// Pass stdin through to CC subprocess if available
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		cmd.Stdin = os.Stdin
	}

	// Capture stdout for JSON parsing
	out, err := cmd.Output()
	elapsed := time.Since(start)

	// Build session log name (best-effort)
	logName := sessionLogName(req.WorkingDir)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			writeCCErrorLog(req, logName, out, exitErr, stderr)
			return nil, fmt.Errorf("claude exited %d: %s", exitErr.ExitCode(), stderr)
		}
		return nil, fmt.Errorf("claude subprocess: %w", err)
	}

	// Save raw JSON to session log
	saveCCSessionLog(logName, out)

	// Parse CC JSON output
	var result ccResult
	if parseErr := json.Unmarshal(out, &result); parseErr != nil {
		slog.Warn("could not parse claude JSON output", "error", parseErr)
		// Return raw output as result
		return &AgentResponse{
			Result:     string(out),
			DurationMs: elapsed.Milliseconds(),
		}, nil
	}

	if result.IsError {
		writeCCErrorLog(req, logName, out, nil, result.Result)
		return nil, fmt.Errorf("claude error: %s", result.Result)
	}

	// Prefer duration from CC if available
	durationMs := elapsed.Milliseconds()
	if result.DurationMs > 0 {
		durationMs = result.DurationMs
	}

	// Write output file for async consumers
	if err := WriteOutputFile(result.Result, "cc", logName); err != nil {
		slog.Error("could not write cc output file", "error", err)
	}

	return &AgentResponse{
		Result:     result.Result,
		DurationMs: durationMs,
	}, nil
}

// ccSessionDir returns the CC session log directory (uses DefaultDataDir for consistency).
func ccSessionDir() string {
	return config.DefaultDataDir() + "/sessions/cc"
}

// ccErrorDir returns the CC error log directory (uses DefaultDataDir for consistency).
func ccErrorDir() string {
	return config.DefaultDataDir() + "/errors/cc"
}

// saveCCSessionLog writes the raw CC JSON output to ~/.einai/sessions/cc/<name>.json.
func saveCCSessionLog(logName string, data []byte) {
	dir := ccSessionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("could not create cc session dir", "error", err)
		return
	}
	path := dir + "/" + logName + ".json"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		slog.Warn("could not write cc session log", "error", err, "path", path)
	}
}

// writeCCErrorLog writes error details to ~/.einai/errors/cc/<name>.jsonl.
func writeCCErrorLog(req AgentRequest, logName string, rawOut []byte, exitErr *exec.ExitError, errMsg string) {
	dir := ccErrorDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := dir + "/" + logName + ".jsonl"
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	logger := slog.New(slog.NewJSONHandler(f, nil))
	if exitErr != nil {
		logger.Error("claude exited non-zero",
			"agent", req.Name,
			"exit_code", exitErr.ExitCode(),
			"stderr", errMsg,
		)
	} else {
		logger.Error("claude error response",
			"agent", req.Name,
			"error", errMsg,
		)
	}
	// Also save raw output if available
	if len(rawOut) > 0 {
		io.WriteString(f, string(rawOut)+"\n") //nolint:errcheck
	}
}
