package session

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/project"
	"github.com/tta-lab/einai/internal/prompt"
	"github.com/tta-lab/einai/internal/provider"
	"github.com/tta-lab/einai/internal/repo"
	"github.com/tta-lab/einai/internal/retry"
	"github.com/tta-lab/logos"
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
	Question   string      `json:"question"`
	Mode       prompt.Mode `json:"mode"`
	Project    string      `json:"project,omitempty"`
	Repo       string      `json:"repo,omitempty"`
	URL        string      `json:"url,omitempty"`
	Save       bool        `json:"save,omitempty"`
	WorkingDir string      `json:"working_dir,omitempty"`
	// Async, when true, instructs the daemon to submit the job to pueue
	// instead of running it synchronously. SendTarget is the ttal send target
	// for completion notification (empty = no callback).
	Async      bool   `json:"async,omitempty"`
	SendTarget string `json:"send_target,omitempty"`
}

// RunAsk executes the ask agent loop and returns a blocking response.
func RunAsk(ctx context.Context, req AskRequest, cfg *config.EinaiConfig) (*AskResponse, error) {
	start := time.Now()

	params, err := ResolveAskParams(ctx, req, cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve params: %w", err)
	}

	systemPrompt, _, err := prompt.BuildSystemPromptForMode(req.Mode, params)
	if err != nil {
		return nil, fmt.Errorf("build system prompt: %w", err)
	}

	prov, modelID, err := provider.Build(cfg.AgentModel())
	if err != nil {
		return nil, fmt.Errorf("build provider: %w", err)
	}

	tc, err := NewTemenosClient(ctx)
	if err != nil {
		return nil, err
	}

	if err := preWarmURL(ctx, tc, req); err != nil {
		return nil, err
	}

	logosCfg := logos.Config{
		Provider:     prov,
		Model:        modelID,
		SystemPrompt: systemPrompt,
		MaxSteps:     cfg.AgentMaxSteps(),
		MaxTokens:    cfg.AgentMaxTokens(),
		Temenos:      tc,
		AllowedPaths: buildAllowedPaths(req.Mode, params),
	}

	question := req.Question
	if req.Mode == prompt.ModeURL && req.URL != "" {
		question = fmt.Sprintf("URL: %s\n\nQuestion: %s", req.URL, req.Question)
	}

	var result *logos.RunResult
	retryErr := retry.WithRetry(ctx, func(msg string) {
		slog.Info("ask retry", "message", msg)
	}, func() error {
		var runErr error
		result, runErr = logos.Run(ctx, logosCfg, nil, question, logos.Callbacks{
			OnRetry: func(reason string, step int) {
				slog.Info("ask step retry", "reason", reason, "step", step)
			},
		})
		return runErr
	})
	if retryErr != nil {
		slog.Error("ask logos.Run failed", "error", retryErr)
		if strings.Contains(retryErr.Error(), "max steps") {
			return nil, fmt.Errorf("ask: %w\n\nTip: increase max_steps in ~/.config/einai/config.toml", retryErr)
		}
		return nil, fmt.Errorf("ask: %w", retryErr)
	}

	response := ""
	if result != nil {
		response = result.Response
	}

	elapsed := time.Since(start)
	return &AskResponse{
		Result:     response,
		DurationMs: elapsed.Milliseconds(),
	}, nil
}

func preWarmURL(ctx context.Context, tc logos.CommandRunner, req AskRequest) error {
	if req.Mode != prompt.ModeURL || req.URL == "" {
		return nil
	}
	slog.Info("fetching URL", "url", req.URL)
	quotedURL := "'" + strings.ReplaceAll(req.URL, "'", "'\\''") + "'"
	resp, err := tc.Run(ctx, logos.RunRequest{Command: "url " + quotedURL})
	if err != nil {
		return fmt.Errorf("pre-fetch %s: %w", req.URL, err)
	}
	if resp.ExitCode != 0 {
		return fmt.Errorf("pre-fetch %s failed (exit %d): %s",
			req.URL, resp.ExitCode, strings.TrimSpace(resp.Stderr))
	}
	return nil
}

func buildAllowedPaths(mode prompt.Mode, params prompt.ModeParams) []logos.AllowedPath {
	switch mode {
	case prompt.ModeProject:
		return []logos.AllowedPath{{Path: params.ProjectPath, ReadOnly: true}}
	case prompt.ModeRepo:
		return []logos.AllowedPath{{Path: params.RepoLocalPath, ReadOnly: true}}
	case prompt.ModeGeneral:
		if params.WorkingDir != "" {
			return []logos.AllowedPath{{Path: params.WorkingDir, ReadOnly: true}}
		}
	}
	return nil
}

// ResolveAskParams validates an AskRequest and returns the resolved ModeParams.
// It is exported so the daemon can invoke validation before async dispatch,
// without running the full agent loop.
func ResolveAskParams(
	ctx context.Context,
	req AskRequest,
	cfg *config.EinaiConfig,
) (prompt.ModeParams, error) {
	params := prompt.ModeParams{
		WorkingDir: req.WorkingDir,
		Question:   req.Question,
		RawURL:     req.URL,
	}

	switch req.Mode {
	case prompt.ModeProject:
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
	case prompt.ModeRepo:
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
	case prompt.ModeURL:
		if req.URL == "" {
			return params, fmt.Errorf("--url required")
		}
	case prompt.ModeWeb:
		// no resolution needed
	case prompt.ModeGeneral:
		if params.WorkingDir == "" {
			return params, fmt.Errorf("working_dir required for general mode")
		}
	default:
		return params, fmt.Errorf("unknown mode: %s", req.Mode)
	}

	return params, nil
}
