package jobqueue

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		Runtime:    "ei-native",
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
		Runtime:    "ei-native",
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
		Kind: KindAgent, Agent: "coder", Runtime: "ei-native", Stem: "kill",
		OutputPath: filepath.Join(dir, "o1.md"),
	})
	q.Enqueue(EnqueueSpec{
		Kind: KindAgent, Agent: "coder", Runtime: "ei-native", Stem: "kill2",
		OutputPath: filepath.Join(dir, "o2.md"),
	})

	w := NewWorker(q, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)

	if err := w.Kill(1); err != nil {
		cancel()
		t.Fatalf("Kill: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var gotJob Job
	for time.Now().Before(deadline) {
		gotJob, _ = q.Get(1)
		if gotJob.State == StateKilled {
			break
		}
		time.Sleep(10 * time.Millisecond)
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
		Kind: KindAgent, Agent: "coder", Runtime: "ei-native", Stem: "long",
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
			Kind: KindAgent, Agent: "coder", Runtime: "ei-native", Stem: fmt.Sprintf("s%d", i),
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
