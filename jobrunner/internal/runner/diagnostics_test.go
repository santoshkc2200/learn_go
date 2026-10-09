package runner

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"testing"
	"time"
)

var goroutineProfile = flag.String("goroutineprofile", "", "optional live goroutine snapshot path for TestGoroutineSnapshot")

func TestRepeatedCancellationJoins(t *testing.T) {
	// Global NumGoroutine counts include test/runtime activity. Instead account
	// for every child this test starts and require a positive join handshake.
	for range 20 {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		started, stopped := make(chan struct{}), make(chan struct{})
		job := JobFunc(func(ctx context.Context) error { close(started); <-ctx.Done(); close(stopped); return ctx.Err() })
		done := make(chan error, 1)
		go func() { _, err := Run(ctx, 2, []Job{job}); done <- err }()
		receive(t, started)
		cancel()
		if err := receive(t, done); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		select {
		case <-stopped:
		default:
			t.Fatal("child outlived runner")
		}
	}
}

func TestDiagnosticWorkload(t *testing.T) {
	ctx, task := trace.NewTask(context.Background(), "diagnostic-batch")
	defer task.End()
	jobs := make([]Job, 8)
	for i := range jobs {
		jobs[i] = JobFunc(func(ctx context.Context) error {
			var err error
			trace.WithRegion(ctx, "timer-job", func() { err = wait(ctx, time.Millisecond) })
			return err
		})
	}
	// Labels use context for metadata at a tooling boundary; business inputs do
	// not belong in context values. Each child receives labels explicitly here.
	pprof.Do(ctx, pprof.Labels("workload", "waiting"), func(ctx context.Context) {
		results, err := Run(ctx, 2, jobs)
		if err != nil || len(results) != 8 {
			t.Fatalf("diagnostic batch: %v %v", results, err)
		}
	})
}

func TestGoroutineSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 2)
	job := JobFunc(func(ctx context.Context) error { started <- struct{}{}; <-ctx.Done(); return ctx.Err() })
	done := make(chan error, 1)
	go func() { _, err := Run(ctx, 2, []Job{job, job}); done <- err }()
	receive(t, started)
	receive(t, started)
	// Snapshot while children are known to be alive; a post-join snapshot would
	// miss the job stacks we want to inspect. debug=2 emits readable full stacks.
	var snapshot bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&snapshot, 2); err != nil {
		t.Fatal(err)
	}
	if snapshot.Len() == 0 {
		t.Fatal("empty goroutine snapshot")
	}
	if *goroutineProfile != "" {
		if err := os.WriteFile(*goroutineProfile, snapshot.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
