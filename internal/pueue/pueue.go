package pueue

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SubmitOpts configures a pueue job submission.
type SubmitOpts struct {
	// Group is the pueue group name to submit to.
	Group string
	// ScriptPath is the path to the .sh file to execute.
	ScriptPath string
	// Label is an optional human-readable label for the job.
	Label string
}

// EnsureGroup creates the pueue group (idempotent) and sets parallelism.
// It runs `pueue group add <group>` then `pueue parallel <n> --group <group>`.
func EnsureGroup(group string, parallel int) error {
	if err := checkPueue(); err != nil {
		return err
	}

	// pueue group add is idempotent — ignore "already exists" errors
	out, err := exec.Command("pueue", "group", "add", group).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "already exists") {
		return fmt.Errorf("pueue group add %q: %w: %s", group, err, strings.TrimSpace(string(out)))
	}

	// Set parallelism for the group
	out, err = exec.Command("pueue", "parallel", strconv.Itoa(parallel), "--group", group).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pueue parallel %d --group %q: %w: %s", parallel, group, err, strings.TrimSpace(string(out)))
	}

	return nil
}

// Submit enqueues a script as a pueue job and returns the assigned job ID.
func Submit(opts SubmitOpts) (int, error) {
	if err := checkPueue(); err != nil {
		return 0, err
	}

	args := []string{"add", "--group", opts.Group, "--print-task-id"}
	if opts.Label != "" {
		args = append(args, "--label", opts.Label)
	}
	args = append(args, "--", "bash", opts.ScriptPath)

	out, err := exec.Command("pueue", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return 0, fmt.Errorf("pueue add: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return 0, fmt.Errorf("pueue add: %w", err)
	}

	idStr := strings.TrimSpace(string(out))
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("parse pueue job id %q: %w", idStr, err)
	}
	return id, nil
}

// checkPueue verifies pueue is available on PATH.
func checkPueue() error {
	if _, err := exec.LookPath("pueue"); err != nil {
		return fmt.Errorf("pueue not found on PATH: install pueue (https://github.com/Nukesor/pueue)")
	}
	return nil
}
