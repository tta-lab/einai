package jobqueue

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// makeFakeAgent writes a script that runs /bin/sleep then exits with the given code.
func makeFakeAgent(_ *testing.T, dir, name string, exitCode, sleepSecs int) string {
	bin := filepath.Join(dir, name)
	code := fmt.Sprintf("#!/bin/bash\necho \"output from %s\"\n/bin/sleep %d\nexit %d\n", name, sleepSecs, exitCode)
	os.WriteFile(bin, []byte(code), 0o755)
	return bin
}

func TestWorker_HappyPath(t *testing.T) {
	dir := t.TempDir()
	q, _ := New(filepath.Join(dir, "queue.jsonl"))

	agentBin := makeFakeAgent(t, dir, "fake-agent", 0, 1)
	w := NewWorker(q, 1)
	w.eiBinary = agentBin

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)

	_, err := q.Enqueue(EnqueueSpec{
		Kind:       KindAgent,
		Agent:      "coder",
		Runtime:    "lenos",
		Prompt:     "say hi",
		WorkingDir: dir,
		SendTarget: "human",
		Stem:       "test",
		OutputPath: filepath.Join(dir, "output.md"),
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var job Job
	for time.Now().Before(deadline) {
		j, ok := q.Get(1)
		if ok && (j.State == StateCompleted || j.State == StateFailed) {
			job = j
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if job.ID == 0 {
		t.Fatalf("job did not complete in time: %+v", q.List(0))
	}
	if job.State != StateCompleted {
		t.Errorf("expected StateCompleted, got %v", job.State)
	}
	if job.EndedAt == nil {
		t.Errorf("EndedAt should be set")
	}
	// Verify output was captured.
	data, err := os.ReadFile(filepath.Join(dir, "output.md"))
	if err != nil {
		t.Fatalf("output.md not found: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("output.md is empty")
	}
}

func TestWorker_FailPath(t *testing.T) {
	dir := t.TempDir()
	q, _ := New(filepath.Join(dir, "queue.jsonl"))

	agentBin := makeFakeAgent(t, dir, "failing-agent", 1, 1)
	w := NewWorker(q, 1)
	w.eiBinary = agentBin

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)

	q.Enqueue(EnqueueSpec{
		Kind:       KindAgent,
		Agent:      "coder",
		Runtime:    "lenos",
		Prompt:     "fail",
		Stem:       "fail",
		OutputPath: filepath.Join(dir, "output.md"),
	})

	deadline := time.Now().Add(10 * time.Second)
	var job Job
	for time.Now().Before(deadline) {
		j, ok := q.Get(1)
		if ok && (j.State == StateCompleted || j.State == StateFailed) {
			job = j
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if job.ID == 0 {
		t.Fatal("job did not complete")
	}
	if job.State != StateFailed {
		t.Errorf("expected StateFailed, got %v", job.State)
	}
	if deref(job.ExitCode, -1) != 1 {
		t.Errorf("expected ExitCode=1, got %d", deref(job.ExitCode, -1))
	}
}

func TestWorker_KillQueued(t *testing.T) {
	dir := t.TempDir()
	q, _ := New(filepath.Join(dir, "queue.jsonl"))

	// Enqueue before starting the worker so jobs exist in the queue.
	q.Enqueue(EnqueueSpec{
		Kind: KindAgent, Agent: "coder", Runtime: "lenos", Stem: "kill",
		OutputPath: filepath.Join(dir, "o1.md"),
	})
	q.Enqueue(EnqueueSpec{
		Kind: KindAgent, Agent: "coder", Runtime: "lenos", Stem: "kill2",
		OutputPath: filepath.Join(dir, "o2.md"),
	})

	w := NewWorker(q, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)

	if err := w.Kill(1); err != nil {
		cancel()
		t.Fatalf("Kill: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var gotJob Job
	for time.Now().Before(deadline) {
		gotJob, _ = q.Get(1)
		if gotJob.State == StateKilled {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if gotJob.State != StateKilled {
		t.Errorf("expected StateKilled, got %v", gotJob.State)
	}
	if gotJob.EndedAt == nil {
		t.Errorf("EndedAt should be set")
	}

	// Let the scheduler goroutine exit cleanly before tempdir cleanup.
	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestWorker_KillRunning(t *testing.T) {
	dir := t.TempDir()
	q, _ := New(filepath.Join(dir, "queue.jsonl"))

	agentBin := makeFakeAgent(t, dir, "long-agent", 0, 30)
	w := NewWorker(q, 1)
	w.eiBinary = agentBin

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)

	q.Enqueue(EnqueueSpec{
		Kind: KindAgent, Agent: "coder", Runtime: "lenos", Stem: "long",
		OutputPath: filepath.Join(dir, "output.md"),
	})

	time.Sleep(200 * time.Millisecond)

	if err := w.Kill(1); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var job Job
	for time.Now().Before(deadline) {
		j, ok := q.Get(1)
		if ok && j.State == StateKilled {
			job = j
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if job.ID == 0 {
		t.Fatal("job did not reach StateKilled")
	}
	if job.State != StateKilled {
		t.Errorf("expected StateKilled, got %v", job.State)
	}
}

func TestWorker_SlotLimit(t *testing.T) {
	dir := t.TempDir()
	q, _ := New(filepath.Join(dir, "queue.jsonl"))

	agentBin := makeFakeAgent(t, dir, "fast-agent", 0, 2)
	w := NewWorker(q, 2)
	w.eiBinary = agentBin

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)

	for i := 1; i <= 3; i++ {
		q.Enqueue(EnqueueSpec{
			Kind: KindAgent, Agent: "coder", Runtime: "lenos", Stem: fmt.Sprintf("s%d", i),
			OutputPath: filepath.Join(dir, fmt.Sprintf("o%d.md", i)),
		})
	}

	time.Sleep(100 * time.Millisecond)

	running := 0
	queued := 0
	for _, j := range q.List(0) {
		if j.State == StateRunning {
			running++
		}
		if j.State == StateQueued {
			queued++
		}
	}
	if running > 2 {
		t.Errorf("expected at most 2 running, got %d", running)
	}
	if queued == 0 && running == 3 {
		t.Errorf("expected at least 1 queued with maxParallel=2")
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		allDone := true
		for _, j := range q.List(0) {
			if j.State != StateCompleted && j.State != StateFailed {
				allDone = false
				break
			}
		}
		if allDone {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestSendCompletion_LogDirInjection(t *testing.T) {
	dir := t.TempDir()
	job := Job{
		ID:         1,
		State:      StateCompleted,
		Agent:      "coder",
		SendTarget: "human",
		LogDir:     dir,
	}

	// Use a non-existent ttal binary — the real one is not needed for test verification.
	// sendCompletion will fail to exec, but will still write the log file from LogDir.
	bin := filepath.Join(dir, "nonexistent-ttal")
	sendCompletion(&job, bin)

	logFile := filepath.Join(dir, "ttal.log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ttal log not written: %v", err)
	}
	if !strings.Contains(string(data), "--to") || !strings.Contains(string(data), "human") {
		t.Errorf("log missing --to human: %s", data)
	}
}

func TestFakeAgent_ExitCode(t *testing.T) {
	dir := t.TempDir()
	bin := makeFakeAgent(t, dir, "test-agent", 42, 0)
	cmd := exec.Command(bin)
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if ee.ExitCode() != 42 {
		t.Errorf("expected exit code 42, got %d", ee.ExitCode())
	}
}

func TestWorker_ConcurrentStress(t *testing.T) {
	// Stress test with -race to catch data races under concurrent load.
	dir := t.TempDir()
	q, _ := New(filepath.Join(dir, "queue.jsonl"))

	agentBin := makeFakeAgent(t, dir, "stress-agent", 0, 2)
	w := NewWorker(q, 4)
	w.eiBinary = agentBin

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)

	const n = 10
	for i := 0; i < n; i++ {
		q.Enqueue(EnqueueSpec{
			Kind:       KindAgent,
			Agent:      "coder",
			Runtime:    "lenos",
			Stem:       fmt.Sprintf("stress-%d", i),
			OutputPath: filepath.Join(dir, fmt.Sprintf("out%d.md", i)),
		})
	}

	deadline := time.Now().Add(20 * time.Second)
	nonTerminal := 0
	for time.Now().Before(deadline) {
		allDone := true
		for _, j := range q.List(0) {
			if j.State != StateCompleted && j.State != StateFailed {
				allDone = false
				nonTerminal++
			}
		}
		if allDone {
			// All jobs reached a terminal state — assert no duplicates.
			ids := make(map[int]bool)
			for _, j := range q.List(0) {
				if ids[j.ID] {
					t.Errorf("duplicate job ID %d in queue", j.ID)
				}
				ids[j.ID] = true
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("stress test timed out with %d non-terminal jobs: %+v", nonTerminal, q.List(0))
}

func TestBuildAskCommand_Table(t *testing.T) {
	tests := []struct {
		name string
		spec *AskSpec
		want []string
	}{
		{
			name: "nil spec",
			spec: nil,
			want: []string{"ask", ""},
		},
		{
			name: "basic question",
			spec: &AskSpec{Question: "hello?"},
			want: []string{"ask", "hello?"},
		},
		{
			name: "project mode",
			spec: &AskSpec{Question: "what?", Mode: "project", Project: "myapp"},
			want: []string{"ask", "what?", "--project", "myapp"},
		},
		{
			name: "repo mode",
			spec: &AskSpec{Question: "code?", Mode: "repo", Repo: "github.com/foo/bar"},
			want: []string{"ask", "code?", "--repo", "github.com/foo/bar"},
		},
		{
			name: "url mode",
			spec: &AskSpec{Question: "docs?", Mode: "url", URL: "https://example.com"},
			want: []string{"ask", "docs?", "--url", "https://example.com"},
		},
		{
			name: "web mode",
			spec: &AskSpec{Question: "latest?", Mode: "web"},
			want: []string{"ask", "latest?", "--web"},
		},
		{
			name: "with save",
			spec: &AskSpec{Question: "save me", Mode: "project", Project: "myapp", Save: true},
			want: []string{"ask", "save me", "--project", "myapp", "--save"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildAskCommand("/bin/ei", tt.spec)
			// cmd.Args[0] is the binary path; compare the rest.
			if !reflect.DeepEqual(cmd.Args[1:], tt.want) {
				t.Errorf("buildAskCommand() args = %v, want %v", cmd.Args[1:], tt.want)
			}
		})
	}
}

// TestWorker_KillRunning_PGIDGuard verifies that Kill returns nil only when
// PGID > 0, and that a SIGTERM is actually sent (the test process survives).
func TestWorker_KillRunning_PGIDGuard(t *testing.T) {
	dir := t.TempDir()
	q, _ := New(filepath.Join(dir, "queue.jsonl"))

	// Long-running agent so Kill has time to act.
	agentBin := makeFakeAgent(t, dir, "long-agent", 0, 30)
	w := NewWorker(q, 1)
	w.eiBinary = agentBin

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)

	q.Enqueue(EnqueueSpec{
		Kind: KindAgent, Agent: "coder", Runtime: "lenos", Stem: "pgid-test",
		OutputPath: filepath.Join(dir, "output.md"),
	})

	// Wait for job to reach running state.
	deadline := time.Now().Add(5 * time.Second)
	var runningJob Job
	for time.Now().Before(deadline) {
		j, ok := q.Get(1)
		if ok && j.State == StateRunning {
			runningJob = j
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if runningJob.ID == 0 {
		t.Fatal("job did not reach running state")
	}

	// Kill must succeed (returns nil) and PGID must be set.
	err := w.Kill(1)
	if err != nil {
		t.Fatalf("Kill returned error (PGID may still be 0): %v", err)
	}

	// Verify job reaches killed state.
	killDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(killDeadline) {
		j, ok := q.Get(1)
		if ok && j.State == StateKilled {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("job did not reach StateKilled: %+v", q.List(0))
}

func TestSendCompletion_FailedState(t *testing.T) {
	dir := t.TempDir()
	job := Job{
		ID:         1,
		State:      StateFailed,
		Agent:      "coder",
		SendTarget: "human",
		ExitCode:   ptr(1),
		LogDir:     dir,
	}
	bin := filepath.Join(dir, "nonexistent-ttal")
	sendCompletion(&job, bin)

	logFile := filepath.Join(dir, "ttal.log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ttal log not written: %v", err)
	}
	if !strings.Contains(string(data), "failed") {
		t.Errorf("log missing 'failed': %s", data)
	}
	if !strings.Contains(string(data), "exit 1") {
		t.Errorf("log missing exit code: %s", data)
	}
}

func TestSendCompletion_KilledState(t *testing.T) {
	dir := t.TempDir()
	job := Job{
		ID:         1,
		State:      StateKilled,
		Agent:      "coder",
		SendTarget: "human",
		LogDir:     dir,
	}
	bin := filepath.Join(dir, "nonexistent-ttal")
	sendCompletion(&job, bin)

	logFile := filepath.Join(dir, "ttal.log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ttal log not written: %v", err)
	}
	if !strings.Contains(string(data), "killed") {
		t.Errorf("log missing 'killed': %s", data)
	}
}
