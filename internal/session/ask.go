package session

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/event"
	"github.com/tta-lab/einai/internal/project"
	"github.com/tta-lab/einai/internal/prompt"
	"github.com/tta-lab/einai/internal/provider"
	"github.com/tta-lab/einai/internal/repo"
	"github.com/tta-lab/einai/internal/retry"
	"github.com/tta-lab/logos"
)

// AskRequest is the wire type for POST /ask.
type AskRequest struct {
	Question   string      `json:"question"`
	Mode       prompt.Mode `json:"mode"`
	Project    string      `json:"project,omitempty"`
	Repo       string      `json:"repo,omitempty"`
	URL        string      `json:"url,omitempty"`
	MaxSteps   int         `json:"max_steps,omitempty"`
	MaxTokens  int         `json:"max_tokens,omitempty"`
	Save       bool        `json:"save,omitempty"`
	WorkingDir string      `json:"working_dir,omitempty"`
}

// RunAsk executes the ask agent loop.
func RunAsk(ctx context.Context, req AskRequest, cfg *config.EinaiConfig, emit event.EventFunc) error {
	params, err := resolveAskParams(ctx, req, cfg, emit)
	if err != nil {
		return fmt.Errorf("resolve params: %w", err)
	}

	systemPrompt, _, err := prompt.BuildSystemPromptForMode(req.Mode, params)
	if err != nil {
		return fmt.Errorf("build system prompt: %w", err)
	}

	prov, modelID, err := provider.Build(cfg.AgentModel())
	if err != nil {
		return fmt.Errorf("build provider: %w", err)
	}

	tc, err := NewTemenosClient(ctx)
	if err != nil {
		return err
	}

	if err := preWarmURL(ctx, tc, req, emit); err != nil {
		return err
	}

	maxSteps := req.MaxSteps
	if maxSteps == 0 {
		maxSteps = cfg.AgentMaxSteps()
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = cfg.AgentMaxTokens()
	}

	logosCfg := logos.Config{
		Provider:     prov,
		Model:        modelID,
		SystemPrompt: systemPrompt,
		MaxSteps:     maxSteps,
		MaxTokens:    maxTokens,
		Temenos:      tc,
		AllowedPaths: buildAllowedPaths(req.Mode, params),
	}

	question := req.Question
	if req.Mode == prompt.ModeURL && req.URL != "" {
		question = fmt.Sprintf("URL: %s\n\nQuestion: %s", req.URL, req.Question)
	}

	var result *logos.RunResult
	retryErr := retry.WithRetry(ctx, func(msg string) {
		emit(event.Event{Type: event.EventStatus, Message: msg})
	}, func() error {
		var runErr error
		result, runErr = logos.Run(ctx, logosCfg, nil, question, buildLogosCallbacks(emit))
		return runErr
	})
	if retryErr != nil {
		errMsg := retryErr.Error()
		if strings.Contains(errMsg, "max steps") {
			errMsg += "\n\nTip: increase the limit with --max-steps"
		}
		log.Printf("[ask] logos.Run error: %v", retryErr)
		emit(event.Event{Type: event.EventError, Message: errMsg})
		return nil
	}

	response := ""
	if result != nil {
		response = result.Response
	}
	emit(event.Event{Type: event.EventDone, Response: response})
	return nil
}

func preWarmURL(ctx context.Context, tc logos.BlockRunner, req AskRequest, emit event.EventFunc) error {
	if req.Mode != prompt.ModeURL || req.URL == "" {
		return nil
	}
	emit(event.Event{Type: event.EventStatus, Message: "Fetching " + req.URL + "..."})
	quotedURL := "'" + strings.ReplaceAll(req.URL, "'", "'\\''") + "'"
	resp, err := tc.RunBlock(ctx, logos.RunBlockRequest{Block: "url " + quotedURL})
	if err != nil {
		return fmt.Errorf("pre-fetch %s: %w", req.URL, err)
	}
	for _, r := range resp.Results {
		if r.ExitCode != 0 {
			return fmt.Errorf("pre-fetch %s failed (exit %d): %s",
				req.URL, r.ExitCode, strings.TrimSpace(r.Stderr))
		}
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

func buildLogosCallbacks(emit event.EventFunc) logos.Callbacks {
	return logos.Callbacks{
		OnDelta: func(text string) {
			emit(event.Event{Type: event.EventDelta, Text: text})
		},
		OnCommandResult: func(command, output string, exitCode int) {
			emit(event.Event{Type: event.EventCommandResult, Command: command, Output: output, ExitCode: exitCode})
		},
		OnRetry: func(reason string, step int) {
			emit(event.Event{Type: event.EventRetry, Reason: reason, Step: step})
		},
	}
}

func resolveAskParams(
	ctx context.Context,
	req AskRequest,
	cfg *config.EinaiConfig,
	emit event.EventFunc,
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
		emit(event.Event{Type: event.EventStatus, Message: "Updating " + req.Repo + "..."})
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
