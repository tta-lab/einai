package jobqueue

import (
	"encoding/json"
	"testing"
	"time"
)

func TestJob_JSONRoundTrip(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	start := now.Add(10 * time.Second)
	end := now.Add(30 * time.Second)
	exitCode := 0

	j := Job{
		ID:         1,
		State:      StateCompleted,
		Agent:      "coder",
		Runtime:    "ei-native",
		Prompt:     "say hello",
		WorkingDir: "/tmp",
		SendTarget: "human",
		Stem:       "session-abc123",
		OutputPath: "/tmp/output.md",
		Kind:       "agent",
		PID:        12345,
		PGID:       12345,
		CreatedAt:  now,
		StartedAt:  ptr(start),
		EndedAt:    ptr(end),
		ExitCode:   &exitCode,
	}

	data, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Job
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != j.ID || got.State != j.State || got.Agent != j.Agent {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Runtime != j.Runtime || got.Prompt != j.Prompt {
		t.Errorf("string fields mismatch")
	}
	if got.WorkingDir != j.WorkingDir || got.SendTarget != j.SendTarget {
		t.Errorf("optional string fields mismatch")
	}
	if got.Stem != j.Stem || got.OutputPath != j.OutputPath {
		t.Errorf("path fields mismatch")
	}
	if got.Kind != j.Kind || got.PID != j.PID || got.PGID != j.PGID {
		t.Errorf("runtime fields mismatch")
	}
	if !got.CreatedAt.Equal(j.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v want %v", got.CreatedAt, j.CreatedAt)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(start) {
		t.Errorf("StartedAt mismatch")
	}
	if got.EndedAt == nil || !got.EndedAt.Equal(end) {
		t.Errorf("EndedAt mismatch")
	}
	if deref(got.ExitCode, -1) != exitCode {
		t.Errorf("ExitCode mismatch")
	}
}

func TestJob_AskSpecRoundTrip(t *testing.T) {
	j := Job{
		ID:    2,
		State: StateQueued,
		Agent: "athena",
		Kind:  "ask",
		AskSpec: &AskSpec{
			Question: "what is the meaning of life?",
			Mode:     "project",
			Project:  "myapp",
			Save:     true,
		},
	}

	data, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Job
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.AskSpec == nil {
		t.Fatalf("AskSpec is nil after unmarshal")
	}
	if got.AskSpec.Question != j.AskSpec.Question {
		t.Errorf("AskSpec.Question mismatch")
	}
	if got.AskSpec.Mode != j.AskSpec.Mode || got.AskSpec.Project != j.AskSpec.Project {
		t.Errorf("AskSpec fields mismatch")
	}
	if !got.AskSpec.Save {
		t.Errorf("AskSpec.Save should be true")
	}
}

func TestJob_MinimalFields(t *testing.T) {
	j := Job{
		ID:    3,
		State: StateQueued,
		Agent: "coder",
		Kind:  "agent",
	}

	data, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Job
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != j.ID || got.State != j.State || got.Agent != j.Agent {
		t.Errorf("basic fields mismatch")
	}
	if got.WorkingDir != "" || got.SendTarget != "" {
		t.Errorf("omitempty fields should be empty")
	}
	if got.AskSpec != nil {
		t.Errorf("AskSpec should be nil")
	}
}
