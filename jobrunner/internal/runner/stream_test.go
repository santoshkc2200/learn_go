package runner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStreamCompletionOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	slowRelease := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(slowRelease) }) }
	defer release()
	input := make(chan Job, 2)
	output := make(chan Result)
	input <- JobFunc(func(ctx context.Context) error {
		select {
		case <-slowRelease:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	input <- JobFunc(func(context.Context) error { return nil })
	close(input)
	done := make(chan error, 1)
	go func() { done <- Stream(ctx, 2, input, output, Options{}) }()
	if result := receive(t, output); result.Index != 1 || result.Err != nil {
		t.Fatalf("want fast job first: %+v", result)
	}
	release()
	if result := receive(t, output); result.Index != 0 || result.Err != nil {
		t.Fatalf("want slow job last: %+v", result)
	}
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	// Stream borrows output; it must not close it. The owner closes after join.
	close(output)
}

func TestStreamValidationAndNilJob(t *testing.T) {
	input := make(chan Job, 2)
	output := make(chan Result, 2)
	for _, tc := range []struct {
		workers int
		in      <-chan Job
		out     chan<- Result
		options Options
		want    error
	}{
		{0, input, output, Options{}, ErrInvalidWorkers},
		{1, nil, output, Options{}, ErrInvalidChannels},
		{1, input, nil, Options{}, ErrInvalidChannels},
		{1, input, output, Options{AdmissionInterval: -1}, ErrInvalidOptions},
	} {
		if err := Stream(context.Background(), tc.workers, tc.in, tc.out, tc.options); !errors.Is(err, tc.want) {
			t.Fatalf("validation: %v", err)
		}
	}
	input <- nil
	input <- JobFunc(func(context.Context) error { return nil })
	close(input)
	if err := Stream(context.Background(), 1, input, output, Options{}); err != nil {
		t.Fatal(err)
	}
	first, second := <-output, <-output
	if first.Started || first.Attempts != 0 || !errors.Is(first.Err, ErrNilJob) || second.Err != nil || second.Index != 1 {
		t.Fatalf("%+v %+v", first, second)
	}
	empty := make(chan Job)
	close(empty)
	if err := Stream(context.Background(), 2, empty, output, Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestStreamCancelBlockedInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	input := make(chan Job, 1)
	output := make(chan Result)
	defer close(input) // Also releases a deliberately broken dispatcher on failure.
	input <- JobFunc(func(context.Context) error { return nil })
	go func() { done <- Stream(ctx, 2, input, output, Options{}) }()
	// A completed result proves Stream passed its initial ctx check and dispatched
	// work. Leave input open and empty, so cancellation must unblock input waiting.
	if result := receive(t, output); result.Err != nil {
		t.Fatal(result.Err)
	}
	cancel()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestStreamBackpressureAndCancelBlockedOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan Job)
	output := make(chan Result) // deliberately never consumed
	sent := make(chan struct{}, 4)
	producerDone := make(chan struct{})
	var ran atomic.Int32
	go func() {
		defer close(producerDone)
		defer close(input)
		for range 4 {
			select {
			case input <- JobFunc(func(context.Context) error { ran.Add(1); return nil }):
				sent <- struct{}{}
			case <-ctx.Done():
				return
			}
		}
	}()
	done := make(chan error, 1)
	go func() { done <- Stream(ctx, 1, input, output, Options{}) }()
	receive(t, sent)
	receive(t, sent)
	receive(t, sent) // one result in the forwarder + one worker + one prefetched job
	select {
	case <-sent:
		t.Fatal("unbounded input consumption with blocked output")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	receive(t, producerDone)
	if ran.Load() != 2 {
		t.Fatalf("jobs ran despite downstream backpressure: %d", ran.Load())
	}
}

func TestStreamJoinsWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan Job, 1)
	output := make(chan Result)
	started, cleanup, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	input <- JobFunc(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(cleanup)
		<-release
		return ctx.Err()
	})
	close(input)
	done := make(chan error, 1)
	go func() { done <- Stream(ctx, 1, input, output, Options{}) }()
	receive(t, started)
	cancel()
	receive(t, cleanup)
	select {
	case <-done:
		t.Fatal("stream abandoned job cleanup")
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
