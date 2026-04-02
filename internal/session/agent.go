package session

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/command"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/event"
	"github.com/tta-lab/einai/internal/project"
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
	SandboxEnv map[string]string `json:"sandbox_env,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	TaskID     *TaskID           `json:"task_id,omitempty"`
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

	taskCtx, history, err := loadTaskContextForAgent(req)
	if err != nil {
		return err
	}

	logosCfg, err := buildAgentConfig(ctx, req, cfg, a, access)
	if err != nil {
		return err
	}

	initialPrompt := buildInitialPrompt(req.Prompt, taskCtx.prompt)
	historyMessages := buildHistoryMessages(history)

	var result *logos.RunResult
	retryErr := retry.WithRetry(ctx, func(msg string) {
		emit(event.Event{Type: event.EventStatus, Message: msg})
	}, func() error {
		var runErr error
		result, runErr = logos.Run(ctx, *logosCfg, historyMessages, initialPrompt, buildLogosCallbacks(emit))
		return runErr
	})
	if retryErr != nil {
		handleRunError(retryErr, emit)
		return retryErr
	}

	response := buildResponseAndSaveSession(result, req, history)
	emit(event.Event{Type: event.EventDone, Response: response})
	return nil
}

// taskContext holds task-related data loaded for an agent session.
type taskContext struct {
	prompt string
}

// loadTaskContextForAgent loads task context and session history if a task ID is provided.
func loadTaskContextForAgent(req AgentRequest) (taskContext, *SessionHistory, error) {
	ctx := taskContext{}
	var history *SessionHistory

	if req.TaskID == nil {
		return ctx, nil, nil
	}

	taskID := *req.TaskID
	history, err := LoadSession(req.Name, taskID)
	if err != nil {
		return ctx, nil, fmt.Errorf("load session: %w", err)
	}

	ctx.prompt, err = loadTaskContext(taskID.String())
	if err != nil {
		return ctx, nil, fmt.Errorf("load task context: %w", err)
	}

	emitSessionStatus(taskID, history)
	return ctx, history, nil
}

// emitSessionStatus emits a status message about the session being started or resumed.
func emitSessionStatus(taskID TaskID, history *SessionHistory) {
	// Status is emitted by the caller
}

// buildInitialPrompt combines the task context with the user prompt.
func buildInitialPrompt(userPrompt, taskPrompt string) string {
	if taskPrompt == "" {
		return userPrompt
	}
	if userPrompt == "" {
		return taskPrompt
	}
	return taskPrompt + "\n\n---\n\nUser request: " + userPrompt
}

// buildHistoryMessages converts session history to logos format.
func buildHistoryMessages(history *SessionHistory) []fantasy.Message {
	if history == nil || len(history.Messages) == 0 {
		return nil
	}
	return history.ToFantasyMessages()
}

// handleRunError handles errors from logos.Run.
func handleRunError(retryErr error, emit event.EventFunc) {
	errMsg := retryErr.Error()
	if strings.Contains(errMsg, "max steps") {
		errMsg += "\n\nTip: increase max_steps in ~/.config/einai/config.toml"
	}
	log.Printf("[agent] logos.Run error: %v", retryErr)
	emit(event.Event{Type: event.EventError, Message: errMsg})
}

// buildResponseAndSaveSession extracts response and persists session if needed.
func buildResponseAndSaveSession(
	result *logos.RunResult,
	req AgentRequest,
	history *SessionHistory,
) string {
	if result == nil {
		return ""
	}

	response := result.Response
	if req.TaskID != nil {
		saveSession(req.Name, *req.TaskID, history, result)
	}
	return response
}

// saveSession persists the session to disk.
func saveSession(agentName string, taskID TaskID, history *SessionHistory, result *logos.RunResult) {
	sessionMessages := mergeSessionMessages(history, result.Steps)
	if err := SaveSession(agentName, taskID, sessionMessages); err != nil {
		log.Printf("[agent] warning: failed to save session: %v", err)
	}
}

// mergeSessionMessages combines existing history with new step messages.
func mergeSessionMessages(history *SessionHistory, steps []logos.StepMessage) []SessionMessage {
	if history != nil && len(history.Messages) > 0 {
		messages := make([]SessionMessage, 0, len(history.Messages)+len(steps))
		messages = append(messages, history.Messages...)
		messages = append(messages, ConvertFromStepMessages(steps)...)
		return messages
	}
	return ConvertFromStepMessages(steps)
}

// loadTaskContext fetches the task context using ttal task get.
func loadTaskContext(taskID string) (string, error) {
	cmd := exec.Command("ttal", "task", "get", taskID)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("ttal task get failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("ttal task get: %w", err)
	}
	return string(output), nil
}

// buildAgentConfig builds the logos config for an agent run.
// It resolves the agent's working directory and access level, then grants
// read-only access to all projects from ttal project list for cross-project reads.
// cwd gets rw access if the agent has rw access; all other projects are ro.
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

	// Grant read access to all projects from ttal project list
	// to enable cross-repo read operations without friction
	var additionalReadOnlyPaths []string
	allProjects, err := project.List()
	if err != nil {
		return nil, fmt.Errorf("list projects for allowed paths: %w", err)
	}
	for _, p := range allProjects {
		if p.Path != "" && p.Path != cwd {
			additionalReadOnlyPaths = append(additionalReadOnlyPaths, p.Path)
		}
	}

	allowedPaths := sandbox.BuildAgentPaths(cwd, effectiveAccess, additionalReadOnlyPaths...)

	return &logos.Config{
		Provider:     prov,
		Model:        modelID,
		SystemPrompt: systemPrompt,
		MaxSteps:     cfg.AgentMaxSteps(),
		MaxTokens:    cfg.AgentMaxTokens(),
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

// ConvertFromStepMessages converts logos.StepMessage to SessionMessage for persistence.
func ConvertFromStepMessages(steps []logos.StepMessage) []SessionMessage {
	messages := make([]SessionMessage, 0, len(steps))
	for _, step := range steps {
		msg := SessionMessage{
			Role:      string(step.Role),
			Content:   step.Content,
			Reasoning: step.Reasoning,
		}
		messages = append(messages, msg)
	}
	return messages
}
