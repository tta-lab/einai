package session

import (
	"os"
	"strings"
	"testing"

	"github.com/tta-lab/einai/internal/config"
)

func TestWriteJobScript_Basic(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	opts := JobScriptOpts{
		Prompt:     "implement the auth module",
		AgentName:  "coder",
		Runtime:    "claude-code",
		Stem:       "20260101-120000-myproj",
		OutputPath: dir + "/outputs/claude-code/20260101-120000-myproj.md",
		TmuxTarget: "",
	}

	path, err := WriteJobScript(opts)
	if err != nil {
		t.Fatalf("WriteJobScript() error: %v", err)
	}

	// Script file must exist and be executable.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("script file not found: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("script file is not executable, mode: %v", info.Mode())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	content := string(data)

	// Must contain shebang.
	if !strings.HasPrefix(content, "#!/usr/bin/env bash") {
		t.Error("script does not start with bash shebang")
	}

	// Must embed the prompt as heredoc.
	if !strings.Contains(content, "implement the auth module") {
		t.Error("script does not contain embedded prompt")
	}
	if !strings.Contains(content, "EINAI_PROMPT_EOF") {
		t.Error("script does not contain heredoc delimiter EINAI_PROMPT_EOF")
	}

	// Must redirect output to OutputPath.
	if !strings.Contains(content, opts.OutputPath) {
		t.Errorf("script does not reference output path %q", opts.OutputPath)
	}
	if !strings.Contains(content, "2>&1") {
		t.Error("script does not contain 2>&1 redirect")
	}

	// Must invoke the agent.
	if !strings.Contains(content, "ei agent run") {
		t.Error("script does not contain 'ei agent run'")
	}
	if !strings.Contains(content, "claude-code") {
		t.Error("script does not reference runtime")
	}
}

// TestWriteJobScript_NoCallbackWhenNoTmuxTarget verifies that no tmux block is
// generated when TmuxTarget is empty.
func TestWriteJobScript_NoCallbackWhenNoTmuxTarget(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	opts := JobScriptOpts{
		Prompt:     "hello",
		AgentName:  "coder",
		Runtime:    "claude-code",
		Stem:       "20260101-120000",
		OutputPath: dir + "/out.md",
		TmuxTarget: "",
	}

	path, err := WriteJobScript(opts)
	if err != nil {
		t.Fatalf("WriteJobScript() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "tmux send-keys") {
		t.Error("script contains tmux callback but TmuxTarget was empty")
	}
}

// TestWriteJobScript_CallbackIncludesTarget verifies the tmux callback block is
// present and references the tmux target when TmuxTarget is set.
func TestWriteJobScript_CallbackIncludesTarget(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	opts := JobScriptOpts{
		Prompt:     "hello",
		AgentName:  "coder",
		Runtime:    "claude-code",
		Stem:       "20260101-120000",
		OutputPath: dir + "/out.md",
		TmuxTarget: "mysession:mywindow",
	}

	path, err := WriteJobScript(opts)
	if err != nil {
		t.Fatalf("WriteJobScript() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "tmux send-keys") {
		t.Error("script missing tmux send-keys callback")
	}
	if !strings.Contains(content, "mysession:mywindow") {
		t.Error("script does not reference tmux target")
	}
	// Must have conditional on $rc for success vs failure.
	if !strings.Contains(content, `"$rc"`) && !strings.Contains(content, "$rc") {
		t.Error("callback does not reference exit code variable")
	}
}

// TestWriteOutputFile_RoundTrip verifies write then read returns the same content.
func TestWriteOutputFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	content := "# Agent Result\n\nSome output here."
	if err := WriteOutputFile(content, "cc", "20260101-120000-myproj"); err != nil {
		t.Fatalf("WriteOutputFile() error: %v", err)
	}

	got, err := ReadOutputFile("cc", "20260101-120000-myproj")
	if err != nil {
		t.Fatalf("ReadOutputFile() error: %v", err)
	}
	if got != content {
		t.Errorf("ReadOutputFile() = %q, want %q", got, content)
	}
}

// TestWriteOutputFile_CreatesDirectory verifies missing output directories
// are created automatically and the file is actually written.
func TestWriteOutputFile_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	// Directory does not exist yet — WriteOutputFile should create it.
	if err := WriteOutputFile("hello", "ei", "new-stem"); err != nil {
		t.Fatalf("WriteOutputFile() error: %v", err)
	}

	// Verify the file was actually written with the expected content.
	got, err := ReadOutputFile("ei", "new-stem")
	if err != nil {
		t.Fatalf("ReadOutputFile() error: %v", err)
	}
	if got != "hello" {
		t.Errorf("ReadOutputFile() = %q, want %q", got, "hello")
	}
}

// TestReadOutputFile_NotFound verifies a clear error is returned for missing files.
func TestReadOutputFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	_, err := ReadOutputFile("cc", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing output file, got nil")
	}
}

// TestShellQuote verifies the function produces correctly quoted shell strings.
func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "''"},
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's", `'it'\''s'`},
		{"don't stop", `'don'\''t stop'`},
		{"/path/to/file", "'/path/to/file'"},
		{"it's a 'test'", `'it'\''s a '\''test'\'''`},
		{"no-special-chars", "'no-special-chars'"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSessionLogName_Format verifies the generated stem has the expected prefix format.
func TestSessionLogName_Format(t *testing.T) {
	name := SessionLogName("")
	// Must start with YYYYMMDD-HHMMSS (14 digits + dash)
	if len(name) < 15 {
		t.Errorf("SessionLogName() = %q is too short", name)
	}
	// Check date part is all digits.
	for i, ch := range name[:8] {
		if ch < '0' || ch > '9' {
			t.Errorf("SessionLogName()[%d] = %q, want digit", i, string(ch))
		}
	}
	if name[8] != '-' {
		t.Errorf("SessionLogName()[8] = %q, want '-'", string(name[8]))
	}
}

// TestWriteJobScript_BadPath verifies an error is returned when the job script
// directory cannot be created (e.g. data dir is read-only).
func TestWriteJobScript_BadPath(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	// Use a read-only directory as the job dir to force failure.
	roDir := dir + "/readonly"
	if err := os.MkdirAll(roDir, 0o555); err != nil {
		t.Fatalf("mkdir readonly: %v", err)
	}

	// If running as root, MkdirAll on readonly dir will still succeed — skip.
	testFile := roDir + "/testwrite"
	if f, err := os.Create(testFile); err == nil {
		f.Close()
		t.Skip("running as root, cannot test read-only dir restriction")
	}

	opts := JobScriptOpts{
		Prompt:     "hello",
		AgentName:  "coder",
		Runtime:    "claude-code",
		Stem:       "stem",
		OutputPath: dir + "/out.md",
		TmuxTarget: "",
	}

	// Override jobDir by using a stem that puts the script under the readonly dir.
	// We achieve this by redirecting DefaultDataDir to a read-only path.
	config.SetTestDataDir(roDir)

	_, err := WriteJobScript(opts)
	if err == nil {
		t.Error("expected error writing to read-only directory, got nil")
	}
}

// TestWriteJobScript_WorkingDirCDIncluded verifies that when WorkingDir is set,
// the script contains a cd line before the ei agent run invocation.
func TestWriteJobScript_WorkingDirCDIncluded(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	opts := JobScriptOpts{
		Prompt:     "do work",
		AgentName:  "coder",
		Runtime:    "claude-code",
		Stem:       "20260101-120000-myproj",
		OutputPath: dir + "/outputs/claude-code/20260101-120000-myproj.md",
		TmuxTarget: "",
		WorkingDir: "/home/neil/Code/myproject",
	}

	path, err := WriteJobScript(opts)
	if err != nil {
		t.Fatalf("WriteJobScript() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "cd '/home/neil/Code/myproject' || exit 1") {
		t.Errorf("script does not contain expected cd line with exit guard; got:\n%s", content)
	}
}

// TestWriteJobScript_NoWorkingDirNoCDLine verifies that when WorkingDir is empty,
// no cd line is written to the script.
func TestWriteJobScript_NoWorkingDirNoCDLine(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	opts := JobScriptOpts{
		Prompt:     "do work",
		AgentName:  "coder",
		Runtime:    "claude-code",
		Stem:       "20260101-120000-myproj",
		OutputPath: dir + "/outputs/claude-code/20260101-120000-myproj.md",
		TmuxTarget: "",
		WorkingDir: "",
	}

	path, err := WriteJobScript(opts)
	if err != nil {
		t.Fatalf("WriteJobScript() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "\ncd ") {
		t.Errorf("script contains unexpected cd line; got:\n%s", content)
	}
}

// TestWriteJobScript_WorkingDirWithTmuxTarget verifies that WorkingDir and
// TmuxTarget interact correctly: the cd line appears before ei agent run and
// the callback block is still present.
func TestWriteJobScript_WorkingDirWithTmuxTarget(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	opts := JobScriptOpts{
		Prompt:     "do work",
		AgentName:  "coder",
		Runtime:    "claude-code",
		Stem:       "20260101-120000-myproj",
		OutputPath: dir + "/outputs/claude-code/20260101-120000-myproj.md",
		TmuxTarget: "%session:0.1",
		WorkingDir: "/home/neil/Code/myproject",
	}

	path, err := WriteJobScript(opts)
	if err != nil {
		t.Fatalf("WriteJobScript() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	content := string(data)

	// cd line must be present with exit guard.
	if !strings.Contains(content, "cd '/home/neil/Code/myproject' || exit 1") {
		t.Errorf("script missing cd line; got:\n%s", content)
	}

	// Callback block must still be present.
	if !strings.Contains(content, "EINAI_TMUX_TARGET") {
		t.Errorf("script missing tmux callback block; got:\n%s", content)
	}
	if !strings.Contains(content, "tmux send-keys") {
		t.Errorf("script missing tmux send-keys; got:\n%s", content)
	}

	// cd must appear before ei agent run.
	cdIdx := strings.Index(content, "cd '/home/neil/Code/myproject'")
	runIdx := strings.Index(content, "ei agent run")
	if cdIdx == -1 || runIdx == -1 || cdIdx > runIdx {
		t.Errorf("cd line does not precede ei agent run (cdIdx=%d, runIdx=%d)", cdIdx, runIdx)
	}
}

// TestWriteJobScript_RelativeWorkingDirReturnsError verifies that a relative
// WorkingDir is rejected at construction time.
func TestWriteJobScript_RelativeWorkingDirReturnsError(t *testing.T) {
	dir := t.TempDir()
	config.SetTestDataDir(dir)
	t.Cleanup(config.ClearTestDataDir)

	opts := JobScriptOpts{
		Prompt:     "do work",
		AgentName:  "coder",
		Runtime:    "claude-code",
		Stem:       "20260101-120000-myproj",
		OutputPath: dir + "/outputs/claude-code/20260101-120000-myproj.md",
		WorkingDir: "relative/path",
	}

	_, err := WriteJobScript(opts)
	if err == nil {
		t.Fatal("expected error for relative WorkingDir, got nil")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("error message should mention absolute path, got: %v", err)
	}
}
