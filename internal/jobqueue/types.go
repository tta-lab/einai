package jobqueue

import (
	"errors"
	"time"
)

// JobState represents the lifecycle state of a job.
type JobState string

const (
	StateQueued    JobState = "queued"
	StateRunning   JobState = "running"
	StateCompleted JobState = "completed"
	StateFailed    JobState = "failed"
	StateKilled    JobState = "killed"
)

// JobKind distinguishes the two job entry-points.
type JobKind string

const (
	KindAgent JobKind = "agent"
	KindAsk   JobKind = "ask"
)

// IsTerminal returns true for completed, failed, or killed states.
func (s JobState) IsTerminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateKilled
}

// Errors.
var (
	ErrNotFound      = errors.New("job not found")
	ErrNotRunning    = errors.New("job not in running state")
	ErrTerminalState = errors.New("job already in terminal state")
	ErrStateMismatch = errors.New("job state changed before transition")
)

// AskSpec mirrors the fields needed to reconstruct an `ei ask` command.
type AskSpec struct {
	Question string `json:"question"`
	Mode     string `json:"mode"` // "project", "repo", "url", "web", "general"
	Project  string `json:"project,omitempty"`
	Repo     string `json:"repo,omitempty"`
	URL      string `json:"url,omitempty"`
	Save     bool   `json:"save"`
}

// Job represents a queued, running, or completed background job.
type Job struct {
	ID         int        `json:"id"`
	State      JobState   `json:"state"`
	Agent      string     `json:"agent"`
	Runtime    string     `json:"runtime"`
	Prompt     string     `json:"prompt"`
	WorkingDir string     `json:"working_dir,omitempty"`
	SendTarget string     `json:"send_target,omitempty"`
	Stem       string     `json:"stem"`
	OutputPath string     `json:"output_path"`
	Kind       JobKind    `json:"kind"`
	AskSpec    *AskSpec   `json:"ask_spec,omitempty"`
	PID        int        `json:"pid,omitempty"`
	PGID       int        `json:"pgid,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	// LogDir is the directory where sendCompletion writes its invocation log.
	// It is not persisted to JSON.
	LogDir string `json:"-"`
}

// EnqueueSpec carries the parameters needed to enqueue a new job.
type EnqueueSpec struct {
	Kind       JobKind  `json:"kind"`
	Agent      string   `json:"agent"`
	Runtime    string   `json:"runtime"`
	Prompt     string   `json:"prompt"`
	WorkingDir string   `json:"working_dir,omitempty"`
	SendTarget string   `json:"send_target,omitempty"`
	Stem       string   `json:"stem"`
	OutputPath string   `json:"output_path"`
	AskSpec    *AskSpec `json:"ask_spec,omitempty"`
	// LogDir is forwarded to sendCompletion for test injection. Not persisted.
	LogDir string `json:"-"`
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// deref returns *p if p is non-nil, otherwise returns zero.
func deref[T any](p *T, zero T) T {
	if p == nil {
		return zero
	}
	return *p
}
