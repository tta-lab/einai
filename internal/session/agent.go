package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/repo"
	rt "github.com/tta-lab/einai/internal/runtime"
)

// AgentResponse is returned by RunAgent and its runtime-specific helpers.
// Invariant: exactly one of Result or Error is non-empty.
type AgentResponse struct {
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// IsError returns true if the response contains an error.
func (r *AgentResponse) IsError() bool { return r.Error != "" }

// AgentRequest is the wire type for POST /agent/run.
type AgentRequest struct {
	Name       string            `json:"name"`
	Prompt     string            `json:"prompt"`
	Project    string            `json:"project,omitempty"`
	Repo       string            `json:"repo,omitempty"`
	SandboxEnv map[string]string `json:"sandbox_env,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	Runtime    string            `json:"runtime,omitempty"`
	// Async, when true, instructs the daemon to enqueue the job for background execution
	// instead of running it synchronously. SendTarget is the ttal send target
	// for completion notification (empty = no callback).
	Async      bool   `json:"async,omitempty"`
	SendTarget string `json:"send_target,omitempty"`
}

// RunAgent dispatches to the appropriate runtime backend based on req.Runtime.
func RunAgent(ctx context.Context, req AgentRequest, cfg *config.EinaiConfig) (*AgentResponse, error) {
	resolved, err := resolveRuntime(req, cfg)
	if err != nil {
		return nil, err
	}

	// Validate that the agent supports the resolved runtime before spawning.
	a, err := agent.Find(req.Name, cfg.AgentsPaths)
	if err != nil {
		return nil, err
	}
	if _, err := agent.ValidateRuntime(a, req.Name, string(resolved)); err != nil {
		return nil, err
	}

	switch resolved {
	case rt.ClaudeCode:
		return RunClaudeCode(ctx, req, cfg)
	case rt.Lenos:
		return RunLenos(ctx, req, cfg)
	default:
		return nil, fmt.Errorf("unhandled runtime: %s", resolved)
	}
}

// ValidateAgentRequest validates agent existence, runtime support, access,
// and working directory resolution. It is used by the daemon's async handler
// for pre-flight validation before queueing — the sync path is validated
// in RunAgent at execution time.
func ValidateAgentRequest(ctx context.Context, req AgentRequest, cfg *config.EinaiConfig) error {
	a, err := agent.Find(req.Name, cfg.AgentsPaths)
	if err != nil {
		return err
	}

	resolved, err := resolveRuntime(req, cfg)
	if err != nil {
		return err
	}

	if resolved == rt.Lenos {
		if _, err := agent.ValidateAccess(a, req.Name); err != nil {
			return err
		}
		_, err = resolveAgentCWD(ctx, req, cfg)
		return err
	}
	return nil
}

// resolveRuntime resolves the effective runtime from flag > config > default.
func resolveRuntime(req AgentRequest, cfg *config.EinaiConfig) (rt.Runtime, error) {
	if req.Runtime != "" {
		return rt.Parse(req.Runtime)
	}
	if cfg.DefaultRuntime != "" {
		return rt.Parse(cfg.DefaultRuntime)
	}
	return rt.Default, nil
}

// SessionLogName returns the timestamped name for a session log file.
// Exported for external use (e.g. daemon test).
func SessionLogName(cwd, name string) string {
	return sessionLogName(cwd, name)
}

// sessionLogName builds a timestamped session log file name.
// Pattern: <YYYYMMDD-HHMMSS>-<agentName>-<project>[-<taskid>]
func sessionLogName(cwd, agentName string) string {
	ts := time.Now().Format("20060102-150405")
	info := resolveProjectInfo(cwd)
	name := ts
	if agentName != "" {
		name += "-" + agentName
	}
	if info.alias != "" {
		name += "-" + info.alias
	}
	if info.taskID != "" {
		name += "-" + info.taskID
	}
	return name
}

// projectInfo holds the project alias and task ID from ttal project resolve.
type projectInfo struct {
	alias  string
	taskID string
}

// resolveProjectInfo shells out to ttal project resolve <cwd> --json.
// Returns zero-value projectInfo on any error (best-effort for log naming).
func resolveProjectInfo(cwd string) projectInfo {
	if cwd == "" {
		return projectInfo{}
	}
	out, err := exec.Command("ttal", "project", "resolve", cwd, "--json").Output()
	if err != nil {
		return projectInfo{}
	}
	var result struct {
		Alias  string `json:"alias"`
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return projectInfo{}
	}
	return projectInfo{alias: result.Alias, taskID: result.TaskID}
}

// resolveAgentCWD resolves the working directory for an agent run.
// Returns the cwd and the effective access level.
func resolveAgentCWD(
	ctx context.Context, req AgentRequest, cfg *config.EinaiConfig,
) (string, error) {
	switch {
	case req.Project != "" && req.Repo != "":
		return "", fmt.Errorf("--project and --repo are mutually exclusive")
	case req.Project != "":
		p, err := resolveProjectPath(req.Project)
		if err != nil {
			return "", fmt.Errorf("resolve project: %w", err)
		}
		return p, nil
	case req.Repo != "":
		cloneURL, localPath, err := repo.ResolveRepoRef(req.Repo, cfg.AgentReferencesPath())
		if err != nil {
			return "", fmt.Errorf("resolve repo: %w", err)
		}
		if err := repo.EnsureRepo(ctx, cloneURL, localPath); err != nil {
			return "", fmt.Errorf("ensure repo: %w", err)
		}
		return localPath, nil
	default:
		if req.WorkingDir == "" {
			return "", fmt.Errorf("working_dir required when no --project/--repo specified")
		}
		return req.WorkingDir, nil
	}
}
