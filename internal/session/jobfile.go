package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tta-lab/einai/internal/config"
)

// jobDir returns the directory for job scripts: ~/.einai/jobs/<runtime>/
func jobDir(runtime string) string {
	return filepath.Join(config.DefaultDataDir(), "jobs", runtime)
}

// outputDir returns the directory for output files: ~/.einai/outputs/<runtime>/
func outputDir(runtime string) string {
	return filepath.Join(config.DefaultDataDir(), "outputs", runtime)
}

// JobScriptOpts configures the job script to be written.
type JobScriptOpts struct {
	// Prompt is the agent prompt, embedded as a heredoc in the script.
	Prompt string
	// AgentName is the name of the agent to run.
	AgentName string
	// Runtime is the runtime backend (e.g. "claude-code" or "ei-native").
	Runtime string
	// Stem is the log name stem (timestamp-project) used for naming files.
	Stem string
	// OutputPath is the full path where the agent run output will be written.
	// It is embedded in the script redirect and the callback message.
	OutputPath string
	// TmuxTarget is the tmux pane target for the completion callback.
	// Empty string means no callback.
	TmuxTarget string
	// WorkingDir is the caller's working directory. When non-empty, the script
	// will cd to this directory before running ei agent run so that the job
	// inherits the correct cwd rather than pueued's default (typically /).
	// Must be an absolute path — relative paths would resolve against pueued's
	// cwd, not the caller's, producing the same broken behaviour this field is
	// designed to fix.
	WorkingDir string
}

// WriteJobScript writes a self-contained shell script that:
//  1. Runs `ei agent run <name> --runtime <rt>` with prompt via heredoc,
//     redirecting all output (stdout + stderr) to OutputPath.
//  2. Captures the exit code.
//  3. Sends a conditional tmux notification (if TmuxTarget is non-empty):
//     ✅ on success, ❌ with exit code on failure.
//  4. Exits with the captured code.
//
// Returns the path to the written script.
func WriteJobScript(opts JobScriptOpts) (path string, err error) {
	// Validate required fields.
	if opts.AgentName == "" {
		return "", fmt.Errorf("agentName: cannot be empty")
	}
	if opts.Runtime == "" {
		return "", fmt.Errorf("runtime: cannot be empty")
	}
	if opts.Prompt == "" {
		return "", fmt.Errorf("prompt: cannot be empty")
	}

	scriptOpts := scriptBuildOpts{
		TmuxTarget:      opts.TmuxTarget,
		OutputPath:      opts.OutputPath,
		WorkingDir:      opts.WorkingDir,
		CommandTemplate: "ei agent run %s --runtime %s",
		Args:            []string{opts.AgentName, opts.Runtime},
		Content:         opts.Prompt,
		Label:           opts.AgentName,
		Stem:            opts.Stem,
	}

	return writeJobScript(scriptOpts)
}

// writeJobScript is the shared implementation for both agent and ask scripts.
func writeJobScript(opts scriptBuildOpts) (path string, err error) {
	if opts.WorkingDir != "" && !filepath.IsAbs(opts.WorkingDir) {
		return "", fmt.Errorf("WorkingDir must be an absolute path: %s", opts.WorkingDir)
	}

	// Determine runtime dir from the command if possible, default to "ask".
	runtimeDir := "ask"
	if len(opts.Args) >= 2 && opts.Args[1] != "" {
		runtimeDir = opts.Args[1]
	}

	dir := jobDir(runtimeDir)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create job dir: %w", err)
	}

	path = filepath.Join(dir, opts.Stem+".sh")

	if err = os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// Build cd line if WorkingDir is set.
	cdLine := ""
	if opts.WorkingDir != "" {
		cdLine = "cd " + shellQuote(opts.WorkingDir) + " || exit 1\n"
	}

	// Build the command with args.
	command := fmt.Sprintf(opts.CommandTemplate, shellQuote(opts.Args[0]), shellQuote(opts.Args[1]))

	// Build tmux callback block.
	callback := callbackBlock(opts.Label, opts.TmuxTarget)

	// The heredoc delimiter is single-quoted to prevent shell expansion.
	hereDoc := "EINAI_EOF"

	script := fmt.Sprintf(`#!/usr/bin/env bash
EINAI_TMUX_TARGET=%s
EINAI_OUTPUT=%s
set +e
%s
%s > "$EINAI_OUTPUT" 2>&1 <<'%s'
%s
%s
rc=$?
%s
exit $rc
`,
		shellQuote(opts.TmuxTarget),
		shellQuote(opts.OutputPath),
		cdLine,
		command,
		hereDoc,
		opts.Content,
		hereDoc,
		callback,
	)

	if err = os.WriteFile(path, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("write job script: %w", err)
	}
	return path, nil
}

// scriptBuildOpts holds common options for building job scripts.
type scriptBuildOpts struct {
	TmuxTarget      string
	OutputPath      string
	WorkingDir      string
	CommandTemplate string // e.g., "ei agent run %s --runtime %s"
	Args            []string
	Content         string
	Label           string
	Stem            string
}

// callbackBlock returns the conditional tmux notification block.
// Variables are assigned at the top of the script so we avoid quoting issues.
func callbackBlock(name, tmuxTarget string) string {
	if tmuxTarget == "" {
		return ""
	}
	return fmt.Sprintf(`
if [ -n "$EINAI_TMUX_TARGET" ]; then
  if [ "$rc" -eq 0 ]; then
    tmux send-keys -t "$EINAI_TMUX_TARGET" \
      "# ✅ %s finished. Read result: cat $EINAI_OUTPUT" Enter
  else
    tmux send-keys -t "$EINAI_TMUX_TARGET" \
      "# ❌ %s failed (exit $rc). Read result: cat $EINAI_OUTPUT" Enter
  fi
fi`, name, name)
}

// WriteOutputFile writes the agent result to ~/.einai/outputs/<runtime>/<stem>.md.
func WriteOutputFile(result, runtime, stem string) error {
	dir := outputDir(runtime)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	path := filepath.Join(dir, stem+".md")
	if err := os.WriteFile(path, []byte(result), 0o644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

// ReadOutputFile reads the agent result from ~/.einai/outputs/<runtime>/<stem>.md.
func ReadOutputFile(runtime, stem string) (string, error) {
	path := filepath.Join(outputDir(runtime), stem+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read output file: %w", err)
	}
	return string(data), nil
}

// AskScriptOpts configures the ask job script.
type AskScriptOpts struct {
	Question   string
	Mode       string // "project", "repo", "url", "web", or "general"
	Project    string
	Repo       string
	URL        string
	Save       bool
	Stem       string
	OutputPath string
	TmuxTarget string
	WorkingDir string
}

// WriteAskJobScript writes a self-contained shell script that runs `ei ask`,
// redirects all output to OutputPath, and (if TmuxTarget is set) sends a
// tmux notification when complete. Returns the path to the written script.
func WriteAskJobScript(opts AskScriptOpts) (path string, err error) {
	// Validate Mode is one of the accepted values.
	switch opts.Mode {
	case "project", "repo", "url", "web", "general", "":
		// valid
	default:
		return "", fmt.Errorf("mode: invalid value %q: must be one of project, repo, url, web, general", opts.Mode)
	}

	// Validate Question is non-empty.
	if opts.Question == "" {
		return "", fmt.Errorf("question: cannot be empty")
	}

	// Build mode flag.
	modeFlag := ""
	switch opts.Mode {
	case "project":
		modeFlag = " --project " + shellQuote(opts.Project)
	case "repo":
		modeFlag = " --repo " + shellQuote(opts.Repo)
	case "url":
		modeFlag = " --url " + shellQuote(opts.URL)
	case "web":
		modeFlag = " --web"
	}

	// Build save flag for the ei ask command.
	saveFlag := ""
	if opts.Save {
		saveFlag = " --save"
	}

	scriptOpts := scriptBuildOpts{
		TmuxTarget:      opts.TmuxTarget,
		OutputPath:      opts.OutputPath,
		WorkingDir:      opts.WorkingDir,
		CommandTemplate: "ei ask%s%s",
		Args:            []string{modeFlag, saveFlag},
		Content:         opts.Question,
		Label:           "ask",
		Stem:            opts.Stem,
	}

	return writeJobScript(scriptOpts)
}

// shellQuote wraps a string in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	escaped := ""
	for _, ch := range s {
		if ch == '\'' {
			escaped += `'\''`
		} else {
			escaped += string(ch)
		}
	}
	return "'" + escaped + "'"
}
