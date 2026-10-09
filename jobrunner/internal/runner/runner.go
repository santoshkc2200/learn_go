// Package runner executes a fixed batch of cooperative jobs.
package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Job is defined by its consumer: the runner needs only this one behavior.
// Implementations must honor ctx to permit prompt shutdown. The runner does
// not recover panics; a panic is a programming error, not an ordinary failure.
type Job interface{ Run(context.Context) error }

// JobFunc adapts a closure to Job through a value-receiver method. No explicit
// "implements" declaration is needed. A nil function is not a valid job.
type JobFunc func(context.Context) error

func (f JobFunc) Run(ctx context.Context) error { return f(ctx) }

var (
	ErrInvalidWorkers = errors.New("workers must be positive")
	ErrNilJob         = errors.New("job is nil")
)

// Result distinguishes a successful job from one that never started.
type Result struct {
	Index    int
	Started  bool
	Attempts int
	Err      error
}

// Run borrows jobs until it returns; the caller must not mutate the slice.
// ctx must be non-nil. Job errors live in results; the batch error is reserved
// for invalid arguments or cancellation. A typed nil can satisfy Job: its
// implementation must support that receiver or the caller must reject it.
func Run(ctx context.Context, workers int, jobs []Job) ([]Result, error) {
	return RunWithOptions(ctx, workers, jobs, Options{})
}

// RunWithOptions adds policy without requiring the simple Run caller to
// understand retry/deadline configuration. A per-job timeout does not cancel
// unrelated jobs; the batch context remains the shared parent.
func RunWithOptions(ctx context.Context, workers int, jobs []Job, o Options) ([]Result, error) {
	if workers <= 0 {
		return nil, ErrInvalidWorkers
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	for i, job := range jobs {
		if job == nil {
			return nil, fmt.Errorf("job %d: %w", i, ErrNilJob)
		}
	}
	results := make([]Result, len(jobs))
	for i := range results {
		results[i].Index = i
	}
	// The sender owns closing work. Unbuffered admission provides backpressure:
	// a send waits until a worker can receive the next index.
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(workers, len(jobs)) {
		wg.Add(1) // Register BEFORE starting the goroutine, not inside it.
		go func() {
			defer wg.Done() // Runs on every ordinary return from this worker.
			for i := range work {
				// select does not prioritize cancellation. A send and cancellation can
				// both be ready, so check again before starting the admitted job.
				if ctx.Err() != nil {
					continue
				}
				result := executeJob(ctx, jobs[i], o)
				result.Index = i
				results[i] = result
			}
		}()
	}
	gate := admission{interval: o.AdmissionInterval}
dispatch:
	for i := range jobs {
		if gate.wait(ctx) != nil {
			break
		}
		select {
		case work <- i:
			gate.sent()
		case <-ctx.Done():
			break dispatch
		}
	}
	close(work)
	// Separate slice elements have separate writers. No append/reslice occurs
	// during work. Wait joins workers and makes their writes visible before the
	// caller reads results; it does not protect shared state inside jobs.
	wg.Wait()
	err := contextError(ctx)
	if err != nil {
		for i := range results {
			if !results[i].Started {
				results[i].Err = err
			}
		}
	}
	return results, err
}
