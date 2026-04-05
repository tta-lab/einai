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
	if opts.WorkingDir != "" && !filepath.IsAbs(opts.WorkingDir) {
		return "", fmt.Errorf("WorkingDir must be an absolute path: %s", opts.WorkingDir)
	}

	dir := jobDir(opts.Runtime)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create job dir: %w", err)
	}

	path = filepath.Join(dir, opts.Stem+".sh")

	// Ensure output directory exists so the redirect can write there.
	if err = os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// Build the callback block. Variables are assigned at the top of the
	// script so we avoid quoting issues in the conditional block.
	callback := callbackBlock(opts.AgentName, opts.TmuxTarget)

	// The heredoc delimiter is single-quoted to prevent shell expansion,
	// allowing prompts containing <>, $(), and other special chars.
	const hereDoc = "EINAI_PROMPT_EOF"

	// Build optional cd line so the job inherits the caller's working directory.
	// || exit 1 ensures a failed cd aborts the job rather than running in the
	// wrong directory (set +e is active, so bare cd would fail silently).
	cdLine := ""
	if opts.WorkingDir != "" {
		cdLine = "cd " + shellQuote(opts.WorkingDir) + " || exit 1\n"
	}

	script := fmt.Sprintf(`#!/usr/bin/env bash
EINAI_TMUX_TARGET=%s
EINAI_OUTPUT=%s
EINAI_AGENT=%s
set +e
%s
ei agent run %s --runtime %s > "$EINAI_OUTPUT" 2>&1 <<'%s'
%s
%s
rc=$?
%s
exit $rc
`,
		shellQuote(opts.TmuxTarget),
		shellQuote(opts.OutputPath),
		shellQuote(opts.AgentName),
		cdLine,
		shellQuote(opts.AgentName),
		shellQuote(opts.Runtime),
		hereDoc,
		opts.Prompt,
		hereDoc,
		callback,
	)

	if err = os.WriteFile(path, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("write job script: %w", err)
	}
	return path, nil
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

// WriteAskJobScript writes a self-contained shell script that runs `ei ask`
// asynchronously, redirects output, and sends a tmux callback on completion.
// Returns the path to the written script.
func WriteAskJobScript(opts AskScriptOpts) (path string, err error) {
	if opts.WorkingDir != "" && !filepath.IsAbs(opts.WorkingDir) {
		return "", fmt.Errorf("WorkingDir must be an absolute path: %s", opts.WorkingDir)
	}

	dir := jobDir("ask")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create job dir: %w", err)
	}

	path = filepath.Join(dir, opts.Stem+".sh")

	if err = os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// Build tmux callback block using shared helper.
	callback := callbackBlock("ask", opts.TmuxTarget)

	// The heredoc delimiter is single-quoted to prevent shell expansion,
	// allowing questions containing <>, $(), and other special chars.
	const hereDoc = "EINAI_ASK_EOF"

	cdLine := ""
	if opts.WorkingDir != "" {
		cdLine = "cd " + shellQuote(opts.WorkingDir) + " || exit 1\n"
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

	script := fmt.Sprintf(`#!/usr/bin/env bash
EINAI_TMUX_TARGET=%s
EINAI_OUTPUT=%s
set +e
%s
ei ask%s%s > "$EINAI_OUTPUT" 2>&1 <<'%s'
%s
%s
rc=$?
%s
%s
exit $rc
`,
		shellQuote(opts.TmuxTarget),
		shellQuote(opts.OutputPath),
		cdLine,
		modeFlag,
		saveFlag,
		hereDoc,
		opts.Question,
		hereDoc,
		"",
		callback,
	)

	if err = os.WriteFile(path, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("write ask job script: %w", err)
	}
	return path, nil
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
