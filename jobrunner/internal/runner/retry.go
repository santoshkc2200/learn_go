package runner

import (
	"context"
	"time"
)

// RetryPolicy is opt-in. MaxAttempts includes the first call; zero means one.
// Callbacks can run concurrently and must be concurrency-safe and prompt.
// A retryable error is not evidence that replaying a side effect is safe.
type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration // Zero defaults to max(InitialBackoff, 1s).
	Retryable      func(error) bool
	Jitter         func(time.Duration) time.Duration // Nil leaves the delay unchanged.
}

func retryDelay(failures int, p RetryPolicy) time.Duration {
	capDelay := p.MaxBackoff
	if capDelay == 0 {
		capDelay = max(p.InitialBackoff, time.Second)
	}
	delay := p.InitialBackoff
	// Saturate BEFORE multiplying. Duration is an int64 and can wrap negative.
	// Stop once capped, so even a huge attempt count does bounded work.
	for n := 1; n < failures && delay > 0 && delay < capDelay; n++ {
		if delay > capDelay/2 {
			delay = capDelay
		} else {
			delay *= 2
		}
	}
	delay = min(delay, capDelay)
	if p.Jitter != nil {
		delay = max(0, min(delay, p.Jitter(delay)))
	}
	return delay
}

func wait(ctx context.Context, d time.Duration) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return contextError(ctx)
	case <-timer.C:
		return contextError(ctx)
	}
}
