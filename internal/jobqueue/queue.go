package jobqueue

import (
	"sort"
	"sync"
	"time"
)

// Queue manages in-memory job state backed by JSONL persistence.
type Queue struct {
	store  *Store
	mu     sync.Mutex
	jobs   map[int]*Job
	nextID int
}

// New loads an existing queue from the given JSONL path, or creates a new one.
func New(path string) (*Queue, error) {
	store := NewStore(path)
	jobs, nextID, err := store.Load()
	if err != nil {
		return nil, err
	}

	jobMap := make(map[int]*Job)
	for i := range jobs {
		jobMap[jobs[i].ID] = &jobs[i]
	}

	q := &Queue{
		store:  store,
		jobs:   jobMap,
		nextID: nextID,
	}

	// Recover any jobs that were Running when the daemon crashed.
	now := ptr(timeNow())
	for _, j := range jobMap {
		if j.State == StateRunning {
			if err := q.Update(j.ID, func(j *Job) {
				j.State = StateFailed
				j.EndedAt = now
			}); err != nil {
				return nil, err
			}
		}
	}

	return q, nil
}

// Enqueue adds a new job to the queue and persists it.
func (q *Queue) Enqueue(spec EnqueueSpec) (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	job := &Job{
		ID:         q.nextID,
		State:      StateQueued,
		Agent:      spec.Agent,
		Runtime:    spec.Runtime,
		Prompt:     spec.Prompt,
		WorkingDir: spec.WorkingDir,
		SendTarget: spec.SendTarget,
		Stem:       spec.Stem,
		OutputPath: spec.OutputPath,
		Kind:       spec.Kind,
		AskSpec:    spec.AskSpec,
		CreatedAt:  timeNow(),
		LogDir:     spec.LogDir,
	}
	q.nextID++

	if err := q.store.Append(*job); err != nil {
		return nil, err
	}
	q.jobs[job.ID] = job
	return job, nil
}

// List returns jobs sorted by CreatedAt ascending (FIFO). limit=0 means all.
func (q *Queue) List(limit int) []Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	jobs := make([]Job, 0, len(q.jobs))
	for _, j := range q.jobs {
		jobs = append(jobs, *j)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	if limit > 0 && limit < len(jobs) {
		jobs = jobs[:limit]
	}
	return jobs
}

// Get looks up a job by ID. Returns zero-value Job, false if not found.
func (q *Queue) Get(id int) (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// Transition atomically transitions a job from fromState to a new state via mut.
// It returns ErrNotFound if the job does not exist, ErrStateMismatch if the job
// is not in fromState, and nil on success.
func (q *Queue) Transition(id int, fromState JobState, mut func(*Job)) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if job.State != fromState {
		return ErrStateMismatch
	}

	mut(job)

	if err := q.store.Append(*job); err != nil {
		return err
	}
	return nil
}

// Update applies a mutation to the job with the given ID and persists it.
// It returns ErrNotFound if the job does not exist and ErrTerminalState if
// the job is already in a terminal state.
func (q *Queue) Update(id int, mut func(*Job)) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return ErrNotFound
	}

	if job.State.IsTerminal() {
		return ErrTerminalState
	}

	mut(job)

	if err := q.store.Append(*job); err != nil {
		return err
	}
	return nil
}

// timeNow is a variable so tests can override it.
var timeNow = func() time.Time {
	return time.Now().UTC()
}
