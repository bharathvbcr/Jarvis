package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/serve"
)

const maxJobs = 16
const maxJobResultBytes = 16 << 20
const maxRetainedJobBytes = 64 << 20
const jobRetention = 5 * time.Minute

type JobView struct {
	JobID     string          `json:"job_id"`
	Operation string          `json:"operation"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}
type backgroundJob struct {
	view     JobView
	ctx      context.Context
	finished time.Time
	running  bool
}
type jobRegistry struct {
	mu       sync.Mutex
	jobs     map[string]*backgroundJob
	retained int
}

func (j *jobRegistry) prune(now time.Time) {
	for id, job := range j.jobs {
		if !job.running && !job.finished.IsZero() && now.Sub(job.finished) >= jobRetention {
			j.retained -= len(job.view.Result)
			delete(j.jobs, id)
		}
	}
}
func (j *jobRegistry) evictOldest(except string) bool {
	var selected string
	var oldest time.Time
	for id, job := range j.jobs {
		if id != except && !job.running && (selected == "" || job.finished.Before(oldest)) {
			selected = id
			oldest = job.finished
		}
	}
	if selected == "" {
		return false
	}
	j.retained -= len(j.jobs[selected].view.Result)
	delete(j.jobs, selected)
	return true
}
func (j *jobRegistry) start(parent context.Context, operation string, limit time.Duration, work func(context.Context) (json.RawMessage, error)) (JobView, error) {
	if parent == nil || work == nil || limit <= 0 || limit > 30*time.Second {
		return JobView{}, errors.New("invalid background job context or bounds")
	}
	if err := parent.Err(); err != nil {
		return JobView{}, err
	}
	id, err := identifier()
	if err != nil {
		return JobView{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := parent.Err(); err != nil {
		return JobView{}, err
	}
	if j.jobs == nil {
		j.jobs = map[string]*backgroundJob{}
	}
	j.prune(time.Now())
	for len(j.jobs) >= maxJobs {
		if !j.evictOldest("") {
			return JobView{}, errors.New("16 background jobs are still active; wait before admitting another")
		}
	}
	ctx, cancel := context.WithTimeout(parent, limit)
	initial := JobView{JobID: id, Operation: operation, Status: "pending"}
	job := &backgroundJob{view: initial, ctx: ctx, running: true}
	j.jobs[id] = job
	go func() {
		defer cancel()
		j.mu.Lock()
		job.view.Status = "running"
		j.mu.Unlock()
		var result json.RawMessage
		var failure error
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					failure = errors.New("background operation panicked")
				}
			}()
			if err := ctx.Err(); err != nil {
				failure = err
				return
			}
			result, failure = work(ctx)
		}()
		j.mu.Lock()
		defer j.mu.Unlock()
		job.running = false
		job.finished = time.Now()
		if err := ctx.Err(); err != nil {
			failure = err
		}
		if failure == nil && (len(result) > maxJobResultBytes || !json.Valid(result)) {
			failure = errors.New("background result is invalid or exceeds 16 MiB")
		}
		if failure != nil {
			job.view.Status = "failed"
			if errors.Is(failure, context.Canceled) {
				job.view.Status = "cancelled"
			}
			job.view.Error = failure.Error()
			return
		}
		for j.retained+len(result) > maxRetainedJobBytes {
			if !j.evictOldest(id) {
				job.view.Status = "failed"
				job.view.Error = "background result retention limit reached"
				return
			}
		}
		job.view.Status = "succeeded"
		job.view.Result = bytes.Clone(result)
		j.retained += len(result)
	}()
	return initial, nil
}
func (j *jobRegistry) get(id string) (JobView, error) {
	if len(id) != 32 {
		return JobView{}, errors.New("invalid job ID")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.prune(time.Now())
	job := j.jobs[id]
	if job == nil {
		return JobView{}, errors.New("unknown or expired job")
	}
	view := job.view
	if job.running {
		if err := job.ctx.Err(); err != nil {
			view.Status = "failed"
			if errors.Is(err, context.Canceled) {
				view.Status = "cancelled"
			}
			view.Error = err.Error()
		}
	}
	view.Result = bytes.Clone(view.Result)
	return view, nil
}
func asyncHandler[P, R interface{}](a *App, operation string, limit time.Duration, fn func(context.Context, P) (R, error)) serve.Handler {
	return handler(func(_ context.Context, p P) (JobView, error) {
		return a.jobs.start(a.ctx, operation, limit, func(ctx context.Context) (json.RawMessage, error) {
			value, err := fn(ctx, p)
			if err != nil {
				return nil, err
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("encode background result: %w", err)
			}
			return raw, nil
		})
	})
}
