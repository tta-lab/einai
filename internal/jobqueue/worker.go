package jobqueue

import (
	"context"
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
func (w *Worker) Start(ctx context.Context) {
	for {
		// Acquire a slot, blocking if none are free.
		select {
		case <-ctx.Done():
			return
		case w.slots <- struct{}{}:
			// got a slot
		}

		w.stoppedMu.Lock()
		stopped := w.stopped
		w.stoppedMu.Unlock()
		if stopped {
			<-w.slots
			return
		}

		// Pick next queued job.
		jobs := w.q.List(0)
		var next Job
		found := false
		for _, j := range jobs {
			if j.State == StateQueued {
				next = j
				found = true
				break
			}
		}
		if !found {
			// No jobs; put slot back and wait.
			<-w.slots
			time.Sleep(10 * time.Millisecond)
			continue
		}

		w.wg.Add(1)
		go func(job Job) {
			defer w.wg.Done()
			defer func() { <-w.slots }()
			w.runJob(&job)
		}(next)
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
func (w *Worker) Kill(id int) error {
	job, ok := w.q.Get(id)
	if !ok {
		return ErrNotFound
	}

	switch job.State {
	case StateQueued:
		w.q.Update(id, func(j *Job) {
			j.State = StateKilled
			j.EndedAt = ptr(time.Now().UTC())
		})
		return nil
	case StateRunning:
		// nothing
	default:
		return ErrNotRunning
	}

	w.killReq.Store(id, true)
	_ = syscall.Kill(-job.PGID, syscall.SIGTERM)

	go func() {
		time.Sleep(5 * time.Second)
		j, ok := w.q.Get(id)
		if ok && j.State == StateRunning {
			_ = syscall.Kill(-j.PGID, syscall.SIGKILL)
		}
	}()

	return nil
}

func (w *Worker) runJob(job *Job) {
	// Resolve ei binary. Tests inject a custom path; production uses "ei".
	eiBin := w.eiBinary
	if eiBin == "ei" {
		if path, err := exec.LookPath("ei"); err == nil {
			eiBin = path
		}
	}

	// Transition to Running.
	now := ptr(time.Now().UTC())
	w.q.Update(job.ID, func(j *Job) {
		j.State = StateRunning
		j.StartedAt = now
	})
	job, _ = w.q.Get(job.ID)

	var cmd *exec.Cmd
	switch job.Kind {
	case "agent":
		cmd = exec.Command(eiBin, "agent", "run", job.Agent, "--runtime", job.Runtime)
		cmd.Dir = job.WorkingDir
		cmd.Stdin = strings.NewReader(job.Prompt)
	case "ask":
		cmd = buildAskCommand(eiBin, job.AskSpec)
	default:
		w.q.Update(job.ID, func(j *Job) {
			j.State = StateFailed
			j.EndedAt = ptr(time.Now().UTC())
		})
		return
	}

	if job.OutputPath != "" {
		os.MkdirAll(filepath.Dir(job.OutputPath), 0o755)
		out, err := os.OpenFile(job.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			w.q.Update(job.ID, func(j *Job) {
				j.State = StateFailed
				j.EndedAt = ptr(time.Now().UTC())
			})
			return
		}
		cmd.Stdout, cmd.Stderr = out, out
		defer out.Close()
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		w.q.Update(job.ID, func(j *Job) {
			j.State = StateFailed
			j.EndedAt = ptr(time.Now().UTC())
		})
		return
	}

	w.q.Update(job.ID, func(j *Job) {
		if j.ID == job.ID {
			j.PID = cmd.Process.Pid
			j.PGID = cmd.Process.Pid
		}
	})

	waitErr := cmd.Wait()
	ended := ptr(time.Now().UTC())
	_, killed := w.killReq.LoadAndDelete(job.ID)

	var finalState JobState
	var exitCode *int
	if killed {
		finalState = StateKilled
	} else if waitErr == nil {
		finalState = StateCompleted
		exitCode = ptr(0)
	} else {
		finalState = StateFailed
		if ee, ok := waitErr.(*exec.ExitError); ok {
			code := ee.ExitCode()
			exitCode = &code
		}
	}

	w.q.Update(job.ID, func(j *Job) {
		if j.ID == job.ID {
			j.State = finalState
			j.EndedAt = ended
			j.ExitCode = exitCode
		}
	})

	j, ok := w.q.Get(job.ID)
	if ok {
		// Preserve LogDir from the original job.
		j.LogDir = job.LogDir
		go sendCompletion(j, w.ttalBinary)
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
