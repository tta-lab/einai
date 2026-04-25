package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Worker manages concurrent job execution backed by a Queue.
type Worker struct {
	q          *Queue
	slots      chan struct{}
	killReq    sync.Map
	wg         sync.WaitGroup
	eiBinary   string
	ttalBinary string
	stopped    bool
	stoppedMu  sync.Mutex
}

// NewWorker creates a Worker with the given queue and parallelism limit.
func NewWorker(q *Queue, maxParallel int) *Worker {
	return &Worker{
		q:          q,
		slots:      make(chan struct{}, maxParallel),
		eiBinary:   "ei",
		ttalBinary: "ttal",
	}
}

// Start begins the scheduler goroutine that processes queued jobs.
// It uses an atomic Transition to claim jobs so two schedulers under
// MaxParallel can never dispatch the same job.
func (w *Worker) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case w.slots <- struct{}{}:
		}

		w.stoppedMu.Lock()
		stopped := w.stopped
		w.stoppedMu.Unlock()
		if stopped {
			<-w.slots
			return
		}

		// Atomic claim: try to find and transition a queued job to running.
		// CAS loop retries if another scheduler claims the job first.
		jobs := w.q.List(0)
		var claimed Job
		for _, j := range jobs {
			if j.State != StateQueued {
				continue
			}
			err := w.q.Transition(j.ID, StateQueued, func(jb *Job) {
				jb.State = StateRunning
				jb.StartedAt = ptr(timeNow())
			})
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if errors.Is(err, ErrStateMismatch) {
				// Someone else claimed it; keep searching.
				continue
			}
			// err is nil: we got it.
			claimed, _ = w.q.Get(j.ID)
			break
		}

		if claimed.ID == 0 {
			// No queued jobs found; release slot and wait.
			<-w.slots
			time.Sleep(10 * time.Millisecond)
			continue
		}

		w.wg.Add(1)
		go func(job Job) {
			defer w.wg.Done()
			defer func() { <-w.slots }()
			w.runJob(job)
		}(claimed)
	}
}

// Slots exposes the semaphore channel so the daemon can inspect slot state.
func (w *Worker) Slots() <-chan struct{} {
	return w.slots
}

// Stop signals the scheduler to stop. In-flight jobs are left to finish.
func (w *Worker) Stop() {
	w.stoppedMu.Lock()
	w.stopped = true
	w.stoppedMu.Unlock()
	w.wg.Wait()
}

// Kill sends SIGTERM to a running job (or marks a queued job as killed).
// A bounded retry loop handles the queued→running race window.
func (w *Worker) Kill(id int) error {
	for attempt := 0; attempt < 2; attempt++ {
		job, ok := w.q.Get(id)
		if !ok {
			return ErrNotFound
		}

		switch job.State {
		case StateQueued:
			err := w.q.Transition(id, StateQueued, func(j *Job) {
				j.State = StateKilled
				j.EndedAt = ptr(timeNow())
			})
			if errors.Is(err, ErrStateMismatch) {
				continue // raced to running; retry.
			}
			return err

		case StateRunning:
			// PGID==0 means the PID+PGID Update hasn't run yet.
			// Return error so caller can retry after the Update completes.
			if job.PGID == 0 {
				return ErrNotRunning
			}
			w.killReq.Store(id, true)
			_ = syscall.Kill(-job.PGID, syscall.SIGTERM)
			go w.escalateKill(id)
			return nil

		default:
			return ErrNotRunning
		}
	}
	return ErrNotRunning
}

func (w *Worker) escalateKill(id int) {
	time.Sleep(5 * time.Second)
	j, ok := w.q.Get(id)
	if ok && j.State == StateRunning && j.PGID != 0 {
		_ = syscall.Kill(-j.PGID, syscall.SIGKILL)
	}
}

// failJob marks a job as failed with the given reason and persists it.
func (w *Worker) failJob(id int, reason error) {
	if err := w.q.Update(id, func(j *Job) {
		j.State = StateFailed
		j.EndedAt = ptr(timeNow())
	}); err != nil {
		slog.Error("failJob: queue update failed", "job_id", id, "reason", reason, "update_error", err)
	} else {
		slog.Warn("job failed", "job_id", id, "reason", reason)
	}
}

// resolveEiBinary returns the resolved path to the ei binary.
func (w *Worker) resolveEiBinary() (string, error) {
	if w.eiBinary != "ei" {
		return w.eiBinary, nil
	}
	path, err := exec.LookPath("ei")
	if err != nil {
		return "", fmt.Errorf("ei binary not found in PATH: %v", err)
	}
	return path, nil
}

// openOutputFile opens (or creates) the output file for the job.
func openOutputFile(outputPath string) (*os.File, error) {
	if outputPath == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %v", err)
	}
	return os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
}

// buildAgentCommand builds the ei agent run command.
func buildAgentCommand(eiBin, agent, runtime, workingDir, prompt string) *exec.Cmd {
	cmd := exec.Command(eiBin, "agent", "run", agent, "--runtime", runtime)
	cmd.Dir = workingDir
	cmd.Stdin = strings.NewReader(prompt)
	return cmd
}

// runJob executes a job that has already been transitioned to Running
// by the scheduler. Scheduler owns the Running transition.
func (w *Worker) runJob(job Job) {
	eiBin, err := w.resolveEiBinary()
	if err != nil {
		w.failJob(job.ID, err)
		return
	}

	job, _ = w.q.Get(job.ID)

	var cmd *exec.Cmd
	switch job.Kind {
	case KindAgent:
		cmd = buildAgentCommand(eiBin, job.Agent, job.Runtime, job.WorkingDir, job.Prompt)
	case KindAsk:
		cmd = buildAskCommand(eiBin, job.AskSpec)
	default:
		w.failJob(job.ID, fmt.Errorf("unknown kind: %q", job.Kind))
		return
	}

	out, err := openOutputFile(job.OutputPath)
	if err != nil {
		w.failJob(job.ID, err)
		return
	}
	if out != nil {
		cmd.Stdout, cmd.Stderr = out, out
		defer out.Close()
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		w.failJob(job.ID, fmt.Errorf("start process: %w", err))
		return
	}

	// Resolve PID and PGID. Use Getpgid as belt-and-braces.
	pid := cmd.Process.Pid
	pgid, pgidErr := syscall.Getpgid(pid)
	if pgidErr != nil {
		_ = cmd.Process.Kill()
		w.failJob(job.ID, fmt.Errorf("getpgid: %w", pgidErr))
		return
	}

	if err := w.q.Update(job.ID, func(j *Job) {
		j.PID = pid
		j.PGID = pgid
	}); err != nil {
		slog.Warn("queue update failed", "error", err, "job_id", job.ID, "pid", pid, "pgid", pgid)
	}

	waitErr := cmd.Wait()
	ended := ptr(timeNow())
	_, killed := w.killReq.LoadAndDelete(job.ID)

	finalState := StateFailed
	var exitCode *int
	if killed {
		finalState = StateKilled
	} else if waitErr == nil {
		finalState = StateCompleted
		ec := 0
		exitCode = &ec
	} else if errors.Is(waitErr, exec.ErrNotFound) {
		ec := -1
		exitCode = &ec
	} else if ee, ok := waitErr.(*exec.ExitError); ok {
		code := ee.ExitCode()
		exitCode = &code
	}

	if err := w.q.Update(job.ID, func(j *Job) {
		j.State = finalState
		j.EndedAt = ended
		j.ExitCode = exitCode
	}); err != nil {
		slog.Warn("queue update failed", "error", err, "job_id", job.ID)
	}

	updatedJob, ok := w.q.Get(job.ID)
	if ok {
		updatedJob.LogDir = job.LogDir
		go sendCompletion(&updatedJob, w.ttalBinary)
	}
}

func buildAskCommand(eiBin string, spec *AskSpec) *exec.Cmd {
	if spec == nil {
		return exec.Command(eiBin, "ask", "")
	}

	args := []string{"ask", spec.Question}
	switch spec.Mode {
	case "project":
		args = append(args, "--project", spec.Project)
	case "repo":
		args = append(args, "--repo", spec.Repo)
	case "url":
		args = append(args, "--url", spec.URL)
	case "web":
		args = append(args, "--web")
	}
	if spec.Save {
		args = append(args, "--save")
	}

	return exec.Command(eiBin, args...)
}
