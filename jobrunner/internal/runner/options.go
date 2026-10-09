package runner

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidOptions = errors.New("invalid runner options")
	ErrJobTimeout     = errors.New("job budget exhausted")
)

// Options separates execution policy from Job's business logic. Its zero value
// preserves Lesson 1. Never store a context here: callers pass it per operation.
type Options struct {
	JobTimeout        time.Duration
	Retry             RetryPolicy
	AdmissionInterval time.Duration
}

func (o Options) validate() error {
	if o.JobTimeout < 0 {
		return fmt.Errorf("job timeout must be nonnegative: %w", ErrInvalidOptions)
	}
	if o.AdmissionInterval < 0 {
		return fmt.Errorf("admission interval must be nonnegative: %w", ErrInvalidOptions)
	}
	if o.Retry.MaxAttempts < 0 || o.Retry.InitialBackoff < 0 || o.Retry.MaxBackoff < 0 {
		return fmt.Errorf("retry counts and durations must be nonnegative: %w", ErrInvalidOptions)
	}
	if o.Retry.MaxBackoff > 0 && o.Retry.MaxBackoff < o.Retry.InitialBackoff {
		return fmt.Errorf("max backoff must cover initial backoff: %w", ErrInvalidOptions)
	}
	return nil
}

// contextError keeps BOTH the standard cancellation identity and a custom cause.
// Context.Err alone answers how it ended; context.Cause can explain why.
func contextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errors.Join(err, context.Cause(ctx))
	}
	return nil
}

func executeJob(ctx context.Context, job Job, o Options) Result {
	if o.JobTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, o.JobTimeout, ErrJobTimeout)
		defer cancel()
	}
	result := Result{}
	limit := max(1, o.Retry.MaxAttempts)
	for result.Attempts < limit {
		if ctx.Err() != nil {
			result.Err = errors.Join(result.Err, contextError(ctx))
			return result
		}
		result.Started = true
		result.Attempts++
		result.Err = job.Run(ctx)
		if ctx.Err() != nil {
			result.Err = errors.Join(result.Err, contextError(ctx))
			return result
		}
		// Errors from a job's own context must not be retried even if a caller's
		// classifier is too broad. Retrying cannot resurrect an expired budget.
		if result.Err == nil || result.Attempts == limit || errors.Is(result.Err, context.Canceled) || errors.Is(result.Err, context.DeadlineExceeded) || o.Retry.Retryable == nil || !o.Retry.Retryable(result.Err) {
			return result
		}
		if err := wait(ctx, retryDelay(result.Attempts, o.Retry)); err != nil {
			result.Err = errors.Join(result.Err, err)
			return result
		}
	}
	return result
}
