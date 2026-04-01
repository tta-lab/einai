package session

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/command"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/event"
	"github.com/tta-lab/einai/internal/provider"
	"github.com/tta-lab/einai/internal/repo"
	"github.com/tta-lab/einai/internal/retry"
	"github.com/tta-lab/einai/internal/sandbox"
	"github.com/tta-lab/logos"
)

// AgentRequest is the wire type for POST /agent/run.
type AgentRequest struct {
	Name       string            `json:"name"`
	Prompt     string            `json:"prompt"`
	Project    string            `json:"project,omitempty"`
	Repo       string            `json:"repo,omitempty"`
	MaxSteps   int               `json:"max_steps,omitempty"`
	MaxTokens  int               `json:"max_tokens,omitempty"`
	SandboxEnv map[string]string `json:"sandbox_env,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
}

// claudeMDInstruction is appended to every agent's system prompt.
const claudeMDInstruction = `

## Project Conventions

Before starting work, check for CLAUDE.md and AGENTS.md in the project root and subfolders. If found,
read them — they contain project conventions, architecture notes, and coding guidelines you must follow.`

// RunAgent executes an agent loop server-side.
func RunAgent(ctx context.Context, req AgentRequest, cfg *config.EinaiConfig, emit event.EventFunc) error {
	a, err := agent.Find(req.Name, cfg.AgentsPaths)
	if err != nil {
		return err
	}

	access, err := agent.ValidateAccess(a, req.Name)
	if err != nil {
		return err
	}

	logosCfg, err := buildAgentConfig(ctx, req, cfg, a, access)
	if err != nil {
		return err
	}

	var result *logos.RunResult
	retryErr := retry.WithRetry(ctx, func(msg string) {
		emit(event.Event{Type: event.EventStatus, Message: msg})
	}, func() error {
		var runErr error
		result, runErr = logos.Run(ctx, *logosCfg, nil, req.Prompt, buildLogosCallbacks(emit))
		return runErr
	})
	if retryErr != nil {
		errMsg := retryErr.Error()
		if strings.Contains(errMsg, "max steps") {
			errMsg += "\n\nTip: increase the limit with --max-steps"
		}
		log.Printf("[agent] logos.Run error: %v", retryErr)
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

func buildAgentConfig(
	ctx context.Context,
	req AgentRequest,
	cfg *config.EinaiConfig,
	a *agent.ParsedAgent,
	access string,
) (*logos.Config, error) {
	model := a.Frontmatter.Ttal.Model
	if model == "" {
		model = cfg.AgentModel()
	}

	prov, modelID, err := provider.Build(model)
	if err != nil {
		return nil, fmt.Errorf("build provider: %w", err)
	}

	cwd, effectiveAccess, err := resolveAgentCWD(ctx, req, cfg, access)
	if err != nil {
		return nil, err
	}

	systemPrompt, err := buildAgentSystemPrompt(a, cwd, effectiveAccess)
	if err != nil {
		return nil, err
	}

	tc, err := NewTemenosClient(ctx)
	if err != nil {
		return nil, err
	}

	maxSteps := req.MaxSteps
	if maxSteps == 0 {
		maxSteps = cfg.AgentMaxSteps()
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = cfg.AgentMaxTokens()
	}

	sb := config.LoadSandbox()
	allowedPaths := sandbox.BuildAgentSandboxPaths(sb, cwd, effectiveAccess, sandbox.CollectProjectGitDirs())

	return &logos.Config{
		Provider:     prov,
		Model:        modelID,
		SystemPrompt: systemPrompt,
		MaxSteps:     maxSteps,
		MaxTokens:    maxTokens,
		Temenos:      tc,
		SandboxEnv:   injectHomeEnv(req.SandboxEnv),
		AllowedPaths: allowedPaths,
	}, nil
}

func buildAgentSystemPrompt(a *agent.ParsedAgent, cwd, access string) (string, error) {
	var cmds []logos.CommandDoc
	if access == "rw" {
		cmds = command.RWCommands()
	} else {
		cmds = command.AllCommands()
	}

	promptData := logos.PromptData{
		WorkingDir: cwd,
		Platform:   runtime.GOOS,
		Date:       time.Now().Format("2006-01-02"),
		Commands:   cmds,
	}
	systemPrompt, err := logos.BuildSystemPrompt(promptData)
	if err != nil {
		return "", fmt.Errorf("build system prompt: %w", err)
	}
	if a.Body != "" {
		systemPrompt += "\n\n" + a.Body
	}
	systemPrompt += claudeMDInstruction
	return systemPrompt, nil
}

func injectHomeEnv(env map[string]string) map[string]string {
	result := make(map[string]string, len(env)+1)
	for k, v := range env {
		result[k] = v
	}
	if _, ok := result["HOME"]; !ok {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[agent] injectHomeEnv: os.UserHomeDir() failed: %v", err)
		} else {
			result["HOME"] = home
		}
	}
	return result
}

func resolveAgentCWD(ctx context.Context, req AgentRequest, cfg *config.EinaiConfig, agentAccess string) (
	cwd, effectiveAccess string, err error,
) {
	switch {
	case req.Project != "" && req.Repo != "":
		return "", "", fmt.Errorf("--project and --repo are mutually exclusive")
	case req.Project != "":
		// Use ttal project path (via project.GetProjectPath in session/ask.go)
		// Import project here to resolve
		p, err := resolveProjectPath(req.Project)
		if err != nil {
			return "", "", fmt.Errorf("resolve project: %w", err)
		}
		return p, agentAccess, nil
	case req.Repo != "":
		cloneURL, localPath, err := repo.ResolveRepoRef(req.Repo, cfg.AgentReferencesPath())
		if err != nil {
			return "", "", fmt.Errorf("resolve repo: %w", err)
		}
		if err := repo.EnsureRepo(ctx, cloneURL, localPath); err != nil {
			return "", "", fmt.Errorf("ensure repo: %w", err)
		}
		return localPath, "ro", nil
	default:
		if req.WorkingDir == "" {
			return "", "", fmt.Errorf("working_dir required when no --project/--repo specified")
		}
		return req.WorkingDir, agentAccess, nil
	}
}
