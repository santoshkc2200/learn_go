package runner

import (
	"context"
	"time"
)

// Only the dispatcher accesses this state: ownership avoids a mutex. The
// first admission is immediate; later admissions wait relative to the last
// actual send, so a slow receiver cannot accumulate a burst of saved permits.
// This spaces NEW JOBS, not each retry's request to a downstream system.
type admission struct {
	interval time.Duration
	next     time.Time
}

func (a *admission) wait(ctx context.Context) error {
	if a.next.IsZero() {
		return contextError(ctx)
	}
	return wait(ctx, time.Until(a.next))
}
func (a *admission) sent() {
	if a.interval > 0 {
		a.next = time.Now().Add(a.interval)
	}
}
