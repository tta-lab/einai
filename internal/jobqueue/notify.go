package jobqueue

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func sendCompletion(job *Job, ttalBin string) {
	if job.SendTarget == "" {
		return
	}

	icon := "✅"
	status := "finished"
	if job.State == StateFailed {
		icon = "❌"
		status = fmt.Sprintf("failed (exit %d)", deref(job.ExitCode, -1))
	}
	if job.State == StateKilled {
		icon = "🛑"
		status = "killed"
	}

	msg := fmt.Sprintf("%s %s %s (job %d). Read: ei job log %d", icon, job.Agent, status, job.ID, job.ID)

	bin := ttalBin
	if bin == "" {
		bin = "ttal"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "send", "--to", job.SendTarget, msg)
	_ = cmd.Run()

	// Test injection: if LogDir is set, write to a log file so tests can verify.
	if job.LogDir != "" {
		logFile := filepath.Join(job.LogDir, "ttal.log")
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		fmt.Fprintf(f, "--to %s\n%s\n", job.SendTarget, msg)
		f.Close()
	}
}
