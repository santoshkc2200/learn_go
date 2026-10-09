package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ErrInvalidChannels = errors.New("input and output channels must be non-nil")

type indexedJob struct {
	index int
	job   Job
}

// Stream consumes incrementally and sends results in completion order. The
// caller owns input/output: close input after production and close output only
// after Stream returns. With unbuffered channels the runner retains O(workers)
// jobs/results plus one prefetched job and one forwarding result, independent
// of total input size. Caller channel buffers contribute their own memory.
// Callers must drain output or cancel. On cancellation output may be incomplete;
// shutdown wins over delivering every partial result. Jobs remain cooperative.
func Stream(ctx context.Context, workers int, input <-chan Job, output chan<- Result, o Options) error {
	if workers <= 0 {
		return ErrInvalidWorkers
	}
	if err := o.validate(); err != nil {
		return err
	}
	if input == nil || output == nil {
		return ErrInvalidChannels
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	work := make(chan indexedJob)
	completed := make(chan Result)
	var wg sync.WaitGroup
	wg.Add(workers + 1) // workers plus the dispatcher, all registered before launch
	for range workers {
		go func() {
			defer wg.Done()
			for item := range work {
				result := Result{Index: item.index}
				if item.job == nil {
					result.Err = fmt.Errorf("job %d: %w", item.index, ErrNilJob)
				} else {
					result = executeJob(ctx, item.job, o)
					result.Index = item.index
				}
				select {
				case completed <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	// A separate dispatcher keeps input/rate waiting independent of draining
	// results. It can prefetch only one job; blocked output stops workers taking
	// more work, which stops dispatch, which stops the producer.
	go func() {
		defer wg.Done()
		defer close(work)
		gate := admission{interval: o.AdmissionInterval}
		for index := 0; ; index++ {
			var job Job
			select {
			case <-ctx.Done():
				return
			case value, ok := <-input:
				if !ok {
					return
				}
				job = value
			}
			if gate.wait(ctx) != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case work <- indexedJob{index, job}:
				gate.sent()
			}
		}
	}()
	joined := make(chan struct{})
	go func() { wg.Wait(); close(completed); close(joined) }()
	for result := range completed {
		select {
		case output <- result:
		case <-ctx.Done():
			<-joined                 // Workers' completed sends are cancellable, so joining cannot
			return contextError(ctx) // deadlock on a consumer that stopped reading.
		}
	}
	<-joined
	return contextError(ctx)
}
