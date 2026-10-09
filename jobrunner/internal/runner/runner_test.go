package runner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunValidation(t *testing.T) {
	for _, workers := range []int{0, -1} {
		if _, err := Run(context.Background(), workers, nil); !errors.Is(err, ErrInvalidWorkers) {
			t.Fatalf("workers %d: %v", workers, err)
		}
	}
	called := false
	jobs := []Job{JobFunc(func(context.Context) error { called = true; return nil }), nil}
	if _, err := Run(context.Background(), 1, jobs); !errors.Is(err, ErrNilJob) {
		t.Fatalf("nil job: %v", err)
	}
	if called {
		t.Fatal("validation executed a job")
	}
}

func TestRunEmpty(t *testing.T) {
	results, err := Run(context.Background(), 2, nil)
	if err != nil || len(results) != 0 {
		t.Fatalf("empty: %v, %v", results, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, 2, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled empty: %v", err)
	}
}

func TestJobFunc(t *testing.T) {
	sentinel := errors.New("job failure")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job := JobFunc(func(got context.Context) error {
		if got != ctx {
			t.Error("context was not propagated")
		}
		return sentinel
	})
	if err := job.Run(ctx); !errors.Is(err, sentinel) {
		t.Fatalf("identity lost: %v", err)
	}
}

func TestRunOutcomes(t *testing.T) {
	sentinel := errors.New("middle failed")
	calls := make([]int, 3)
	jobs := make([]Job, 3)
	for i := range jobs {
		// Each closure captures its own loop index on modern Go. Each job writes
		// only its own counter, just as the runner will own separate result slots.
		jobs[i] = JobFunc(func(context.Context) error {
			calls[i]++
			if i == 1 {
				return sentinel
			}
			return nil
		})
	}
	results, err := Run(context.Background(), 2, jobs)
	if err != nil || len(results) != 3 {
		t.Fatalf("batch: %v %v", results, err)
	}
	for i, result := range results {
		if calls[i] != 1 || result.Index != i || !result.Started {
			t.Fatalf("job %d: calls=%d result=%+v", i, calls[i], result)
		}
		if i == 1 {
			if !errors.Is(result.Err, sentinel) {
				t.Fatal("job error identity lost")
			}
		} else if result.Err != nil {
			t.Fatal(result.Err)
		}
	}
}

type nilSafeJob struct{}

func (*nilSafeJob) Run(context.Context) error { return nil }

// A pointer receiver belongs to *nilSafeJob's method set. nilSafeJob itself
// would not satisfy Job; uncommenting a value assertion is a compile exercise.
var _ Job = (*nilSafeJob)(nil)

func TestTypedNilJob(t *testing.T) {
	var ptr *nilSafeJob
	var job Job = ptr
	if job == nil {
		t.Fatal("typed nil interface unexpectedly nil")
	}
	results, err := Run(context.Background(), 1, []Job{job})
	if err != nil || !results[0].Started || results[0].Err != nil {
		t.Fatalf("nil-safe receiver: %v %v", results, err)
	}
}

// A timeout is a guard against broken code, not the mechanism that schedules
// this test. Channel handshakes establish when jobs have actually started.
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for runner")
		var zero T
		return zero
	}
}

func TestRunBoundedConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var active, maximum atomic.Int32
	jobs := make([]Job, 4)
	for i := range jobs {
		jobs[i] = JobFunc(func(ctx context.Context) error {
			n := active.Add(1)
			defer active.Add(-1)
			for old := maximum.Load(); n > old; old = maximum.Load() {
				if maximum.CompareAndSwap(old, n) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
	done := make(chan error, 1)
	go func() { _, err := Run(ctx, 2, jobs); done <- err }()
	receive(t, started)
	receive(t, started) // Sequential execution cannot reach this barrier.
	unblock()
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	receive(t, started)
	receive(t, started)
	if maximum.Load() != 2 || active.Load() != 0 {
		t.Fatalf("max=%d active=%d", maximum.Load(), active.Load())
	}
}

func TestRunMoreWorkersThanJobs(t *testing.T) {
	results, err := Run(context.Background(), 20, []Job{JobFunc(func(context.Context) error { return nil })})
	if err != nil || len(results) != 1 || !results[0].Started {
		t.Fatalf("%v %v", results, err)
	}
}

func TestRunPreCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err := Run(ctx, 2, []Job{JobFunc(func(context.Context) error { t.Error("started canceled job"); return nil })})
	if !errors.Is(err, context.Canceled) || results[0].Started || !errors.Is(results[0].Err, context.Canceled) {
		t.Fatalf("%v %v", results, err)
	}
}

func TestRunCancellationDuringAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	jobs := []Job{
		JobFunc(func(ctx context.Context) error { started <- struct{}{}; <-ctx.Done(); return ctx.Err() }),
		JobFunc(func(context.Context) error { t.Error("second job started after cancellation"); return nil }),
	}
	type batch struct {
		results []Result
		err     error
	}
	done := make(chan batch, 1)
	go func() { results, err := Run(ctx, 1, jobs); done <- batch{results, err} }()
	receive(t, started)
	cancel()
	got := receive(t, done)
	if !errors.Is(got.err, context.Canceled) || !got.results[0].Started || got.results[1].Started {
		t.Fatalf("%+v", got)
	}
	for _, result := range got.results {
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("false success: %+v", result)
		}
	}
}

func TestRunWaitsForStartedJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, acknowledged, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var stopped atomic.Bool
	job := JobFunc(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(acknowledged)
		<-release // Deliberately delay cleanup: cancellation does not kill a goroutine.
		stopped.Store(true)
		return ctx.Err()
	})
	done := make(chan error, 1)
	go func() { _, err := Run(ctx, 1, []Job{job}); done <- err }()
	receive(t, started)
	cancel()
	receive(t, acknowledged)
	select {
	case <-done:
		t.Fatal("returned while job was still running")
	case <-time.After(100 * time.Millisecond):
		// Keep the job blocked while observing that Run does not return.
		// A default branch checks only one instant and can miss an early return
		// whose notification has not been scheduled yet. This bounded negative
		// observation supplements (not replaces) the channel handshakes.
	}
	unblock()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !stopped.Load() {
		t.Fatal("Run did not join the job")
	}
}
