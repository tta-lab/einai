package jobqueue

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// Store provides append-only JSONL persistence for jobs.
type Store struct {
	path string
}

// NewStore returns a Store that persists to the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Append writes a single job as a JSON line and syncs to disk.
func (s *Store) Append(job Job) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil && filepath.Dir(s.path) != "" {
		return err
	}

	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// Load reads all jobs from the JSONL file and returns them along with the
// next ID to assign (max existing ID + 1, or 1 if empty).
// If the file does not exist it returns an empty slice and nextID=1.
func (s *Store) Load() ([]Job, int, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 1, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	var jobs []Job
	seen := make(map[int]bool)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var job Job
		if err := json.Unmarshal(line, &job); err != nil {
			log.Printf("store: skipping malformed line: %v", err)
			continue
		}
		// Deduplicate: keep last occurrence of each ID
		seen[job.ID] = true
		jobs = append(jobs, job)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	nextID := 1
	for _, j := range jobs {
		if j.ID >= nextID {
			nextID = j.ID + 1
		}
	}

	// Deduplicate: keep last occurrence of each ID
	last := make(map[int]Job)
	for _, j := range jobs {
		last[j.ID] = j
	}
	deduped := make([]Job, 0, len(last))
	for _, j := range last {
		deduped = append(deduped, j)
	}

	return deduped, nextID, nil
}
