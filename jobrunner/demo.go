package main

import (
	"context"
	"fmt"
	"time"
)

// TemporaryError carries data, unlike a sentinel. errors.As lets consumers
// find it through wrapping without parsing its human-readable message.
type TemporaryError struct{ Attempt int }

func (e *TemporaryError) Error() string {
	return fmt.Sprintf("simulated temporary failure on attempt %d", e.Attempt)
}

// A demo job is a pointer because its attempts counter changes across calls.
// Each input gets a distinct job, and only its worker runs its attempts in
// sequence. Reusing the SAME pointer concurrently would require synchronization.
type demoJob struct {
	duration  time.Duration
	failFirst int
	attempts  int
}

func (job *demoJob) Run(ctx context.Context) error {
	if err := (delayJob{duration: job.duration}).Run(ctx); err != nil {
		return err
	}
	job.attempts++
	if job.attempts <= job.failFirst {
		return &TemporaryError{Attempt: job.attempts}
	}
	return nil
}
