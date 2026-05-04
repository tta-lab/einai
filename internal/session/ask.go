package session

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"os/exec"

	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/project"
	"github.com/tta-lab/einai/internal/repo"
)

// AskResponse is returned by RunAsk.
// Invariant: exactly one of Result or Error is non-empty.
type AskResponse struct {
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// IsError returns true if the response contains an error.
func (r *AskResponse) IsError() bool { return r.Error != "" }

// AskRequest is the wire type for POST /ask.
type AskRequest struct {
	Question   string `json:"question"`
	Mode       Mode   `json:"mode"`
	Project    string `json:"project,omitempty"`
	Repo       string `json:"repo,omitempty"`
	URL        string `json:"url,omitempty"`
	Save       bool   `json:"save,omitempty"`
	WorkingDir string `json:"working_dir,omitempty"`
	// Async, when true, instructs the daemon to enqueue the job for background execution
	// instead of running it synchronously. SendTarget is the ttal send target
	// for completion notification (empty = no callback).
	Async      bool   `json:"async,omitempty"`
	SendTarget string `json:"send_target,omitempty"`
}

// RunAsk executes the ask agent by spawning `lenos run --agent ask-<mode> ...`.
func RunAsk(ctx context.Context, req AskRequest, cfg *config.EinaiConfig) (*AskResponse, error) {
	start := time.Now()

	params, err := ResolveAskParams(ctx, req, cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve params: %w", err)
	}

	agentName := "ask-" + string(req.Mode)

	// Render context file for this ask session.
	ctxContent := renderAskContext(req, params)
	ctxFile, err := os.CreateTemp("", "ei-ask-ctx-*.md")
	if err != nil {
		return nil, fmt.Errorf("create context file: %w", err)
	}
	if _, err := ctxFile.WriteString(ctxContent); err != nil {
		os.Remove(ctxFile.Name())
		return nil, fmt.Errorf("write context file: %w", err)
	}
	ctxFile.Close()
	defer os.Remove(ctxFile.Name())

	cwd := params.WorkingDir
	if cwd == "" {
		cwd = os.TempDir()
	}

	args := []string{
		"run",
		"--quiet",
		"--agent", agentName,
		"--cwd", cwd,
		"-f", ctxFile.Name(),
	}
	if req.Question != "" {
		args = append(args, "--", req.Question)
	}

	cmd := exec.CommandContext(ctx, "lenos", args...)
	cmd.Dir = cwd

	out, err := cmd.Output()
	elapsed := time.Since(start)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			msg := strings.TrimSpace(string(exitErr.Stderr))
			if len(out) > 0 {
				msg = strings.TrimSpace(string(out)) + "\n" + msg
			}
			return nil, fmt.Errorf("lenos exited %d: %s",
				exitErr.ExitCode(), msg)
		}
		return nil, fmt.Errorf("lenos subprocess: %w", err)
	}

	result := string(out)

	// Write output file for async persist (if configured).
	stem := SessionLogName(req.WorkingDir, agentName)
	if err := WriteOutputFile(result, "lenos", stem); err != nil {
		slog.Error("could not write lenos output file", "error", err)
	}

	return &AskResponse{
		Result:     result,
		DurationMs: elapsed.Milliseconds(),
	}, nil
}

// renderAskContext builds the markdown context file content for ask agents.
// Keys are lowercase snake_case values that the agent reads from its context-files.
func renderAskContext(req AskRequest, params ModeParams) string {
	var b strings.Builder
	b.WriteString("# Ask Context\n\n")
	fmt.Fprintf(&b, "- mode: %s\n", string(req.Mode))
	fmt.Fprintf(&b, "- working_dir: %s\n", params.WorkingDir)
	if params.ProjectPath != "" {
		fmt.Fprintf(&b, "- project_path: %s\n", params.ProjectPath)
	}
	if params.RepoLocalPath != "" {
		fmt.Fprintf(&b, "- repo_local_path: %s\n", params.RepoLocalPath)
	}
	if req.URL != "" {
		fmt.Fprintf(&b, "- url: %s\n", req.URL)
	}
	b.WriteString("\n")
	return b.String()
}

// ResolveAskParams validates an AskRequest and returns the resolved ModeParams.
// It is used by RunAsk and by the daemon's async handler for pre-flight validation.
func ResolveAskParams(
	ctx context.Context,
	req AskRequest,
	cfg *config.EinaiConfig,
) (ModeParams, error) {
	params := ModeParams{
		WorkingDir: req.WorkingDir,
		Question:   req.Question,
		RawURL:     req.URL,
	}

	switch req.Mode {
	case ModeProject:
		if req.Project == "" {
			return params, fmt.Errorf("--project alias required")
		}
		projectPath, err := project.GetProjectPath(req.Project)
		if err != nil {
			return params, err
		}
		if _, err := os.Stat(projectPath); err != nil {
			return params, fmt.Errorf("project path %q does not exist: %w", projectPath, err)
		}
		params.ProjectPath = projectPath
		params.WorkingDir = projectPath
	case ModeRepo:
		if req.Repo == "" {
			return params, fmt.Errorf("--repo reference required")
		}
		cloneURL, localPath, err := repo.ResolveRepoRef(req.Repo, cfg.AgentReferencesPath())
		if err != nil {
			return params, err
		}
		slog.Info("updating repo", "repo", req.Repo)
		if err := repo.EnsureRepo(ctx, cloneURL, localPath); err != nil {
			return params, err
		}
		params.RepoLocalPath = localPath
		params.WorkingDir = localPath
	case ModeURL:
		if req.URL == "" {
			return params, fmt.Errorf("--url required")
		}
	case ModeWeb:
		// no resolution needed
	case ModeGeneral:
		if params.WorkingDir == "" {
			return params, fmt.Errorf("working_dir required for general mode")
		}
	default:
		return params, fmt.Errorf("unknown mode: %s", req.Mode)
	}

	return params, nil
}
