package jobqueue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")

	// Touch an empty file (simulating empty JSONL)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	f.Close()

	s := NewStore(path)
	jobs, nextID, err := s.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
	if nextID != 1 {
		t.Errorf("expected nextID=1, got %d", nextID)
	}
}

func TestStore_AppendAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")
	s := NewStore(path)

	j := Job{
		ID:        1,
		State:     StateQueued,
		Agent:     "coder",
		Kind:      "agent",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	if err := s.Append(j); err != nil {
		t.Fatalf("Append: %v", err)
	}

	jobs, nextID, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].ID != j.ID || jobs[0].State != j.State {
		t.Errorf("job mismatch: %+v", jobs[0])
	}
	if nextID != 2 {
		t.Errorf("expected nextID=2, got %d", nextID)
	}

	// Append another, check deduplication (last write wins)
	j2 := Job{
		ID:        1,
		State:     StateRunning,
		Agent:     "coder",
		Kind:      "agent",
		CreatedAt: j.CreatedAt,
	}
	if err := s.Append(j2); err != nil {
		t.Fatalf("Append j2: %v", err)
	}

	jobs, _, err = s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("expected 1 job after dedup, got %d", len(jobs))
	}
	if jobs[0].State != StateRunning {
		t.Errorf("expected StateRunning, got %v", jobs[0].State)
	}
}

func TestStore_MalformedTrailingLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")

	j := Job{
		ID:        1,
		State:     StateQueued,
		Agent:     "coder",
		Kind:      "agent",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	validLine, _ := json.Marshal(j)

	// Write valid line + malformed trailing line (torn write simulation)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	f.Write(validLine)
	f.Write([]byte("\n"))
	f.Write([]byte("{invalid json\n"))
	f.Close()

	s := NewStore(path)
	jobs, _, err := s.Load()
	if err != nil {
		t.Fatalf("Load should not error on malformed trailing line: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("expected 1 valid job, got %d", len(jobs))
	}
}

func TestStore_NextIDDerivesFromMax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")

	// Write two jobs with IDs 5 and 7
	j1 := Job{ID: 5, State: StateCompleted, CreatedAt: time.Now().UTC()}
	j2 := Job{ID: 7, State: StateCompleted, CreatedAt: time.Now().UTC()}
	f, _ := os.Create(path)
	enc := json.NewEncoder(f)
	enc.Encode(j1)
	enc.Encode(j2)
	f.Close()

	s := NewStore(path)
	_, nextID, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if nextID != 8 {
		t.Errorf("expected nextID=8, got %d", nextID)
	}
}
