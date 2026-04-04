package pueue

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCheckPueue_NotOnPath verifies a clear error is returned when pueue is not on PATH.
func TestCheckPueue_NotOnPath(t *testing.T) {
	// Temporarily clear PATH so pueue lookup fails.
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", "")                            //nolint:errcheck

	err := checkPueue()
	if err == nil {
		t.Fatal("expected error when pueue not on PATH, got nil")
	}
	if !strings.Contains(err.Error(), "pueue not found") {
		t.Errorf("error %q does not contain 'pueue not found'", err.Error())
	}
}

// TestEnsureGroup_NotOnPath verifies EnsureGroup propagates the PATH error.
func TestEnsureGroup_NotOnPath(t *testing.T) {
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", "")                            //nolint:errcheck

	err := EnsureGroup("einai", 2)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "pueue not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSubmit_NotOnPath verifies Submit propagates the PATH error.
func TestSubmit_NotOnPath(t *testing.T) {
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", "")                            //nolint:errcheck

	_, err := Submit(SubmitOpts{Group: "einai", ScriptPath: "/tmp/test.sh"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "pueue not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSubmit_ParsesJobID verifies Submit correctly parses an integer job ID
// from pueue stdout. Uses a fake pueue script.
func TestSubmit_ParsesJobID(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	// Write a fake pueue script that prints "42" and exits 0.
	dir := t.TempDir()
	fakePueue := dir + "/pueue"
	if err := os.WriteFile(fakePueue, []byte("#!/bin/sh\necho 42\n"), 0o755); err != nil {
		t.Fatalf("write fake pueue: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", dir+":"+oldPath)               //nolint:errcheck

	jobID, err := Submit(SubmitOpts{Group: "einai", ScriptPath: "/tmp/test.sh"})
	if err != nil {
		t.Fatalf("Submit() error: %v", err)
	}
	if jobID != 42 {
		t.Errorf("jobID = %d, want 42", jobID)
	}
}

// TestSubmit_NonIntegerOutput verifies Submit returns an error for non-integer stdout.
func TestSubmit_NonIntegerOutput(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	dir := t.TempDir()
	fakePueue := dir + "/pueue"
	if err := os.WriteFile(fakePueue, []byte("#!/bin/sh\necho 'not a number'\n"), 0o755); err != nil {
		t.Fatalf("write fake pueue: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", dir+":"+oldPath)               //nolint:errcheck

	_, err := Submit(SubmitOpts{Group: "einai", ScriptPath: "/tmp/test.sh"})
	if err == nil {
		t.Fatal("expected error for non-integer output, got nil")
	}
	if !strings.Contains(err.Error(), "parse pueue job id") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSubmit_NonZeroExit verifies Submit returns an error when pueue exits non-zero.
func TestSubmit_NonZeroExit(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	dir := t.TempDir()
	fakePueue := dir + "/pueue"
	if err := os.WriteFile(fakePueue, []byte("#!/bin/sh\necho 'group error' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake pueue: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", dir+":"+oldPath)               //nolint:errcheck

	_, err := Submit(SubmitOpts{Group: "einai", ScriptPath: "/tmp/test.sh"})
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
}

// TestEnsureGroup_ParallelFails verifies EnsureGroup propagates a failure from
// the pueue parallel command.
func TestEnsureGroup_ParallelFails(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	dir := t.TempDir()
	fakePueue := dir + "/pueue"
	script := `#!/bin/sh
if [ "$1" = "parallel" ]; then
  echo "parallel error" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(fakePueue, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pueue: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", dir+":"+oldPath)               //nolint:errcheck

	err := EnsureGroup("einai", 2)
	if err == nil {
		t.Fatal("expected error when pueue parallel fails, got nil")
	}
	if !strings.Contains(err.Error(), "pueue parallel") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestEnsureGroup_AlreadyExists verifies EnsureGroup tolerates "already exists" output.
func TestEnsureGroup_AlreadyExists(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	dir := t.TempDir()
	fakePueue := dir + "/pueue"
	// First invocation (group add) exits 1 with "already exists" — second (parallel) exits 0.
	script := `#!/bin/sh
if [ "$1" = "group" ]; then
  echo "Group 'einai' already exists" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(fakePueue, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pueue: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", dir+":"+oldPath)               //nolint:errcheck

	if err := EnsureGroup("einai", 2); err != nil {
		t.Errorf("EnsureGroup() returned unexpected error: %v", err)
	}
}

// TestSubmit_IncludesLabel verifies the --label arg is included when Label is set.
func TestSubmit_IncludesLabel(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	dir := t.TempDir()
	argsFile := dir + "/args.txt"
	fakePueue := dir + "/pueue"
	// Write args to file so we can inspect them, then echo job ID.
	script := `#!/bin/sh
echo "$@" > ` + argsFile + `
echo 7
`
	if err := os.WriteFile(fakePueue, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pueue: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) }) //nolint:errcheck
	os.Setenv("PATH", dir+":"+oldPath)               //nolint:errcheck

	_, err := Submit(SubmitOpts{Group: "einai", ScriptPath: "/tmp/test.sh", Label: "coder"})
	if err != nil {
		t.Fatalf("Submit() error: %v", err)
	}

	argsData, readErr := os.ReadFile(argsFile)
	if readErr != nil {
		t.Fatalf("read args file: %v", readErr)
	}
	args := string(argsData)
	if !strings.Contains(args, "--label") || !strings.Contains(args, "coder") {
		t.Errorf("expected --label coder in args, got: %s", args)
	}
}
