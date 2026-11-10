package app

import (
	"context"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/store"
)

// Job is a long-running task (install, game update) the UI follows by polling.
type Job struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	ServerID string    `json:"serverId,omitempty"`
	Stage    string    `json:"stage"`
	Done     int64     `json:"done"`
	Total    int64     `json:"total"`
	Finished bool      `json:"finished"`
	Error    string    `json:"error,omitempty"`
	Result   any       `json:"result,omitempty"`
	Started  time.Time `json:"startedAt"`

	mu sync.Mutex
}

func (j *Job) Set(stage string, done, total int64) {
	j.mu.Lock()
	j.Stage, j.Done, j.Total = stage, done, total
	j.mu.Unlock()
}

// Snapshot is a copy safe to encode.
func (j *Job) Snapshot() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Job{ID: j.ID, Kind: j.Kind, ServerID: j.ServerID, Stage: j.Stage, Done: j.Done, Total: j.Total, Finished: j.Finished, Error: j.Error, Result: j.Result, Started: j.Started}
}

type Jobs struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func newJobs() *Jobs { return &Jobs{jobs: map[string]*Job{}} }

// Start runs fn in the background. Only one job per server at a time.
func (js *Jobs) Start(kind, serverID string, fn func(ctx context.Context, j *Job) (any, error)) (*Job, error) {
	js.mu.Lock()
	for id, j := range js.jobs {
		s := j.Snapshot()
		if s.Finished && time.Since(s.Started) > time.Hour {
			delete(js.jobs, id)
			continue
		}
		if serverID != "" && s.ServerID == serverID && !s.Finished {
			js.mu.Unlock()
			return nil, errBusy
		}
	}
	j := &Job{ID: store.NewID(), Kind: kind, ServerID: serverID, Stage: "queued", Started: time.Now()}
	js.jobs[j.ID] = j
	js.mu.Unlock()
	go func() {
		res, err := fn(context.Background(), j)
		j.mu.Lock()
		j.Finished, j.Result = true, res
		if err != nil {
			j.Error = err.Error()
		}
		j.mu.Unlock()
	}()
	return j, nil
}

func (js *Jobs) Get(id string) (*Job, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	j, ok := js.jobs[id]
	return j, ok
}

// Active returns the unfinished job for a server, if any.
func (js *Jobs) Active(serverID string) *Job {
	js.mu.Lock()
	defer js.mu.Unlock()
	for _, j := range js.jobs {
		if s := j.Snapshot(); s.ServerID == serverID && !s.Finished {
			return j
		}
	}
	return nil
}
