package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

// One operation is a complete 64-job batch, including dispatch/join and results.
// Fixtures are built outside b.Loop. This measures scheduling overhead for no-op
// jobs and a modest CPU workload, not process startup, CLI output or real I/O.
func BenchmarkBatch(b *testing.B) {
	for _, kind := range []string{"noop", "cpu"} {
		for _, workers := range []int{1, 2, 4} {
			b.Run(fmt.Sprintf("%s/workers_%d", kind, workers), func(b *testing.B) {
				payload := make([]byte, 4096)
				for i := range payload {
					payload[i] = byte(i)
				}
				sums := make([][32]byte, 64)
				jobs := make([]Job, 64)
				for i := range jobs {
					jobs[i] = JobFunc(func(context.Context) error {
						if kind == "cpu" {
							sums[i] = sha256.Sum256(payload)
						}
						return nil
					})
				}
				var results []Result
				var err error
				b.ReportAllocs()
				for b.Loop() {
					results, err = Run(context.Background(), workers, jobs)
					if err != nil {
						b.Fatal(err)
					}
				}
				if len(results) != 64 {
					b.Fatal("incomplete batch")
				}
				for i, result := range results {
					if !result.Started || result.Attempts != 1 || result.Err != nil {
						b.Fatalf("invalid result: %+v", result)
					}
					if kind == "cpu" && sums[i] != sha256.Sum256(payload) {
						b.Fatal("CPU output mismatch")
					}
				}
			})
		}
	}
}

// Timers deliberately block, making channel/timer/scheduler waits visible in
// block profiles and traces. This is not a timer precision benchmark.
func BenchmarkWaitingBatch(b *testing.B) {
	jobs := make([]Job, 16)
	for i := range jobs {
		jobs[i] = JobFunc(func(ctx context.Context) error { return wait(ctx, time.Millisecond) })
	}
	b.ReportAllocs()
	for b.Loop() {
		results, err := Run(context.Background(), 4, jobs)
		if err != nil || len(results) != 16 {
			b.Fatalf("%v %v", results, err)
		}
	}
}
