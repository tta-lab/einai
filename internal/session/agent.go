package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/command"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/project"
	"github.com/tta-lab/einai/internal/provider"
	"github.com/tta-lab/einai/internal/repo"
	"github.com/tta-lab/einai/internal/retry"
	rt "github.com/tta-lab/einai/internal/runtime"
	"github.com/tta-lab/einai/internal/sandbox"
	"github.com/tta-lab/logos"
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
	// Async, when true, instructs the daemon to submit the job to pueue
	// instead of running it synchronously. SendTarget is the ttal send target
	// for completion notification (empty = no callback).
	Async      bool   `json:"async,omitempty"`
	SendTarget string `json:"send_target,omitempty"`
}

// claudeMDInstruction is appended to every agent's system prompt.
const claudeMDInstruction = `

## Project Conventions

Before starting work, check for CLAUDE.md and AGENTS.md in the project root and subfolders. If found,
read them — they contain project conventions, architecture notes, and coding guidelines you must follow.`

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
	case rt.EiNative:
		return RunEiNative(ctx, req, cfg)
	default:
		return nil, fmt.Errorf("unhandled runtime: %s", resolved)
	}
}

// ValidateAgentRequest validates agent existence, runtime support, access,
// and working directory resolution. It is exported so the daemon can invoke
// validation before async dispatch, without running the full agent loop.
func ValidateAgentRequest(ctx context.Context, req AgentRequest, cfg *config.EinaiConfig) error {
	resolved, err := resolveRuntime(req, cfg)
	if err != nil {
		return err
	}

	a, err := agent.Find(req.Name, cfg.AgentsPaths)
	if err != nil {
		return err
	}

	if _, err := agent.ValidateRuntime(a, req.Name, string(resolved)); err != nil {
		return err
	}

	if resolved == rt.EiNative {
		access, err := agent.ValidateAccess(a, req.Name)
		if err != nil {
			return err
		}
		_, _, err = resolveAgentCWD(ctx, req, cfg, access)
		return err
	}

	return nil
}

// resolveRuntime determines the effective runtime: flag > config > default.
func resolveRuntime(req AgentRequest, cfg *config.EinaiConfig) (rt.Runtime, error) {
	raw := req.Runtime
	if raw == "" {
		raw = cfg.AgentDefaultRuntime()
	}
	return rt.Parse(raw)
}

// RunEiNative executes the agent using the logos+temenos loop.
func RunEiNative(ctx context.Context, req AgentRequest, cfg *config.EinaiConfig) (*AgentResponse, error) {
	start := time.Now()

	a, err := agent.Find(req.Name, cfg.AgentsPaths)
	if err != nil {
		return nil, err
	}

	access, err := agent.ValidateAccess(a, req.Name)
	if err != nil {
		return nil, err
	}

	logosCfg, err := buildAgentConfig(ctx, req, cfg, a, access)
	if err != nil {
		return nil, err
	}

	var result *logos.RunResult
	retryErr := retry.WithRetry(ctx, func(msg string) {
		slog.Info("agent retry", "message", msg, "agent", req.Name)
	}, func() error {
		var runErr error
		result, runErr = logos.Run(ctx, *logosCfg, nil, req.Prompt, logos.Callbacks{
			OnRetry: func(reason string, step int) {
				slog.Info("agent step retry", "reason", reason, "step", step)
			},
		})
		return runErr
	})
	if retryErr != nil {
		slog.Error("agent logos.Run failed", "error", retryErr, "agent", req.Name)
		writeEiErrorLog(req, retryErr)
		errMsg := retryErr.Error()
		if strings.Contains(errMsg, "max steps") {
			return nil, fmt.Errorf("agent run: %w\n\nTip: increase max_steps in ~/.config/einai/config.toml", retryErr)
		}
		return nil, fmt.Errorf("agent run: %w", retryErr)
	}

	elapsed := time.Since(start)

	logName := sessionLogName(req.WorkingDir, req.Name)

	response := ""
	if result != nil {
		response = result.Response
		saveEiSessionLog(req, result, logName)
	}

	// Write output file for async consumers
	if err := WriteOutputFile(response, string(rt.EiNative), logName); err != nil {
		slog.Error("could not write ei output file", "error", err)
	}

	return &AgentResponse{
		Result:     response,
		DurationMs: elapsed.Milliseconds(),
	}, nil
}

// SessionLogName returns the timestamped name for a session log file.
// Pattern: <YYYYMMDD-HHMMSS>-<project>[-<taskid>]
// Exported for use by the CLI async path.
func SessionLogName(cwd, agentName string) string {
	return sessionLogName(cwd, agentName)
}

// sessionLogName is the internal implementation.
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
	cmd := exec.Command("ttal", "project", "resolve", cwd, "--json")
	out, err := cmd.Output()
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

// saveEiSessionLog saves the run result as JSONL to ~/.einai/sessions/ei-native/.
// logName is the pre-computed stem (timestamp-agent-project[-taskid]) to use for the file name.
func saveEiSessionLog(_ AgentRequest, result *logos.RunResult, logName string) {
	dir := eiSessionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("could not create ei session dir", "error", err)
		return
	}
	name := logName + ".jsonl"
	path := dir + "/" + name
	f, err := os.Create(path)
	if err != nil {
		slog.Warn("could not create ei session log", "error", err)
		return
	}
	defer f.Close()

	now := time.Now().Format(time.RFC3339)
	for _, step := range result.Steps {
		data, _ := json.Marshal(map[string]string{
			"role":      string(step.Role),
			"content":   step.Content,
			"timestamp": now,
		})
		_, _ = fmt.Fprintln(f, string(data))
	}
}

// writeEiErrorLog writes error details to ~/.einai/errors/ei-native/.
func writeEiErrorLog(req AgentRequest, runErr error) {
	dir := eiErrorDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := sessionLogName(req.WorkingDir, req.Name) + ".jsonl"
	path := dir + "/" + name
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	logger := slog.New(slog.NewJSONHandler(f, nil))
	logger.Error("agent run failed", "agent", req.Name, "error", runErr.Error())
}

func eiSessionDir() string {
	return config.DefaultDataDir() + "/sessions/" + string(rt.EiNative)
}

func eiErrorDir() string {
	return config.DefaultDataDir() + "/errors/" + string(rt.EiNative)
}

// buildAgentConfig builds the logos config for an ei-native agent run.
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

	var additionalReadOnlyPaths []string
	allProjects, err := project.List()
	if err != nil {
		slog.Warn("failed to list projects for cross-project access", "error", err)
	} else {
		for _, p := range allProjects {
			if p.Path != "" && p.Path != cwd {
				additionalReadOnlyPaths = append(additionalReadOnlyPaths, p.Path)
			}
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
			slog.Warn("injectHomeEnv: os.UserHomeDir() failed", "error", err)
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

// SessionMessage represents one message in the session history.
type SessionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// SessionHistory holds conversation history.
type SessionHistory struct {
	AgentName string           `json:"agent_name"`
	Messages  []SessionMessage `json:"messages"`
}

// ToFantasyMessages converts SessionMessages to []fantasy.Message for logos.Run.
func (h *SessionHistory) ToFantasyMessages() []fantasy.Message {
	messages := make([]fantasy.Message, 0, len(h.Messages))
	for _, msg := range h.Messages {
		role := fantasy.MessageRoleUser
		switch msg.Role {
		case "assistant":
			role = fantasy.MessageRoleAssistant
		case "system":
			role = fantasy.MessageRoleSystem
		}
		messages = append(messages, fantasy.Message{
			Role:    role,
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: msg.Content}},
		})
	}
	return messages
}

// ConvertFromStepMessages converts logos.StepMessage to SessionMessage for persistence.
func ConvertFromStepMessages(steps []logos.StepMessage) []SessionMessage {
	messages := make([]SessionMessage, 0, len(steps))
	now := time.Now().Format(time.RFC3339)
	for _, step := range steps {
		messages = append(messages, SessionMessage{
			Role:      string(step.Role),
			Content:   step.Content,
			Reasoning: step.Reasoning,
			Timestamp: now,
		})
	}
	return messages
}
