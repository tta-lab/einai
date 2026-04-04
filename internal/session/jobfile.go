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
	callbackBlock := ""
	if opts.TmuxTarget != "" {
		callbackBlock = `
if [ -n "$EINAI_TMUX_TARGET" ]; then
  if [ "$rc" -eq 0 ]; then
    tmux send-keys -t "$EINAI_TMUX_TARGET" \
      "# ✅ $EINAI_AGENT finished. Read result: cat $EINAI_OUTPUT" Enter
  else
    tmux send-keys -t "$EINAI_TMUX_TARGET" \
      "# ❌ $EINAI_AGENT failed (exit $rc). Read result: cat $EINAI_OUTPUT" Enter
  fi
fi`
	}

	// The heredoc delimiter is unquoted so we can embed it safely.
	// Prompts containing "EINAI_PROMPT_EOF" on its own line would break
	// the heredoc — this is an accepted limitation.
	const hereDoc = "EINAI_PROMPT_EOF"

	script := fmt.Sprintf(`#!/usr/bin/env bash
EINAI_TMUX_TARGET=%s
EINAI_OUTPUT=%s
EINAI_AGENT=%s
set +e

ei agent run %s --runtime %s > "$EINAI_OUTPUT" 2>&1 <<%s
%s
%s
rc=$?
%s
exit $rc
`,
		shellQuote(opts.TmuxTarget),
		shellQuote(opts.OutputPath),
		shellQuote(opts.AgentName),
		shellQuote(opts.AgentName),
		shellQuote(opts.Runtime),
		hereDoc,
		opts.Prompt,
		hereDoc,
		callbackBlock,
	)

	if err = os.WriteFile(path, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("write job script: %w", err)
	}
	return path, nil
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
