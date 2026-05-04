package session

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/config"
)

// RunLenos executes the agent by spawning `lenos run --agent <name> --cwd <wd>
// [--model <m>] -- <prompt>`. Output is plain text from lenos's stdout
// (no JSON output format on lenos run as of 2026-05-04).
func RunLenos(ctx context.Context, req AgentRequest, cfg *config.EinaiConfig) (*AgentResponse, error) {
	start := time.Now()

	a, err := agent.Find(req.Name, cfg.AgentsPaths)
	if err != nil {
		return nil, err
	}

	// Lenos handles its own allowed-paths via BuildAllowedPaths(cwd, access).
	// We only resolve the cwd here; access semantics are lenos-side.
	cwd, err := resolveAgentCWD(ctx, req, cfg)
	if err != nil {
		return nil, err
	}

	args := buildLenosArgs(req, a, cwd)
	cmd := exec.CommandContext(ctx, "lenos", args...)
	cmd.Dir = cwd

	// Set LENOS_AGENTS_DIR so lenos can discover the agent without pre-configured agent_paths.
	cmd.Env = append(os.Environ(), "LENOS_AGENTS_DIR="+a.SourceDir)

	// Pass stdin through if piped (mirror ccrun.go).
	stat, statErr := os.Stdin.Stat()
	if statErr == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		cmd.Stdin = os.Stdin
	}

	out, err := cmd.Output()
	elapsed := time.Since(start)

	logName := sessionLogName(req.WorkingDir, req.Name)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			writeLenosErrorLog(req, logName, out, exitErr, stderr)
			return nil, fmt.Errorf("lenos exited %d: %s", exitErr.ExitCode(), stderr)
		}
		return nil, fmt.Errorf("lenos subprocess: %w", err)
	}

	result := string(out)
	if err := WriteOutputFile(result, "lenos", logName); err != nil {
		slog.Error("could not write lenos output file", "error", err)
	}

	return &AgentResponse{
		Result:     result,
		DurationMs: elapsed.Milliseconds(),
	}, nil
}

// buildLenosArgs constructs the `lenos run` argv. Extracted for unit testing.
func buildLenosArgs(req AgentRequest, a *agent.ParsedAgent, cwd string) []string {
	args := []string{
		"run",
		"--quiet",
		"--agent", req.Name,
		"--cwd", cwd,
	}
	if a.Frontmatter.Lenos != nil && a.Frontmatter.Lenos.Access == "ro" {
		args = append(args, "--readonly")
	}
	if a.Frontmatter.Lenos != nil && a.Frontmatter.Lenos.Model != "" {
		args = append(args, "--model", a.Frontmatter.Lenos.Model)
	}
	if req.Prompt != "" {
		args = append(args, "--", req.Prompt)
	}
	return args
}

// writeLenosErrorLog mirrors writeCCErrorLog. Path: ~/.einai/errors/lenos/<stem>.jsonl
func writeLenosErrorLog(req AgentRequest, logName string, rawOut []byte, exitErr *exec.ExitError, errMsg string) {
	dir := config.DefaultDataDir() + "/errors/lenos"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("lenos error: cannot create error log dir", "error", err, "dir", dir)
		return
	}
	path := dir + "/" + logName + ".jsonl"
	f, err := os.Create(path)
	if err != nil {
		slog.Warn("lenos error: cannot create error log file", "error", err, "path", path)
		return
	}
	defer f.Close()
	logger := slog.New(slog.NewJSONHandler(f, nil))
	if exitErr != nil {
		logger.Error("lenos exited non-zero",
			"agent", req.Name,
			"exit_code", exitErr.ExitCode(),
			"stderr", errMsg,
		)
	}
	if len(rawOut) > 0 {
		_, _ = f.WriteString(string(rawOut) + "\n")
	}
}
