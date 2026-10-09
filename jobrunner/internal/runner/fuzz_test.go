package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func FuzzOptions(f *testing.F) {
	f.Add(2, 3, int64(0), int64(0))
	f.Add(0, 1, int64(-1), int64(0))
	f.Fuzz(func(t *testing.T, workers, count int, timeout, interval int64) {
		// Bound work to protect the fuzzer from turning one input into hours of
		// timers or millions of goroutines. Negative values still probe validation.
		workers = workers % 5
		count = count % 9
		timeout = timeout % 1_000_000
		interval = interval % 1_000_000
		if count < 0 {
			count = -count
		}
		jobs := make([]Job, count)
		for i := range jobs {
			jobs[i] = JobFunc(func(context.Context) error { return nil })
		}
		options := Options{JobTimeout: time.Duration(timeout), AdmissionInterval: time.Duration(interval)}
		results, err := RunWithOptions(context.Background(), workers, jobs, options)
		if workers <= 0 {
			if !errors.Is(err, ErrInvalidWorkers) {
				t.Fatal(err)
			}
			return
		}
		if timeout < 0 || interval < 0 {
			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatal(err)
			}
			return
		}
		if err != nil || len(results) != count {
			t.Fatalf("%v %v", results, err)
		}
		for i, result := range results {
			if result.Index != i || result.Attempts > 1 {
				t.Fatalf("invalid result: %+v", result)
			}
		}
	})
}
