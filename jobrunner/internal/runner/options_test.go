package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestJobDeadlineIsIsolated(t *testing.T) {
	// Without a child deadline this job blocks until the safety guard, so the
	// test catches a missing per-job timeout rather than trusting elapsed time.
	parent, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	jobs := []Job{
		JobFunc(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }),
		JobFunc(func(context.Context) error { return nil }),
	}
	results, err := RunWithOptions(parent, 1, jobs, Options{JobTimeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("job timeout canceled batch: %v", err)
	}
	if !errors.Is(results[0].Err, context.DeadlineExceeded) || !errors.Is(results[0].Err, ErrJobTimeout) {
		t.Fatalf("missing timeout identity/cause: %v", results[0].Err)
	}
	if results[1].Err != nil || !results[1].Started {
		t.Fatalf("next job did not succeed: %+v", results[1])
	}
}

func TestCancellationCause(t *testing.T) {
	cause := errors.New("operator stopped batch")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	results, err := RunWithOptions(ctx, 1, []Job{JobFunc(func(context.Context) error { t.Error("canceled job ran"); return nil })}, Options{})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || !errors.Is(results[0].Err, cause) {
		t.Fatalf("cause lost: %v %+v", err, results)
	}
}

func TestInvalidJobTimeout(t *testing.T) {
	if _, err := RunWithOptions(context.Background(), 1, nil, Options{JobTimeout: -time.Second}); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("invalid options: %v", err)
	}
}
