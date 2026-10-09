package runner

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestRetries(t *testing.T) {
	transient := errors.New("transient")
	permanent := errors.New("permanent")
	cases := []struct {
		name        string
		failures    int
		failure     error
		limit, want int
		wantErr     error
	}{
		{"recovers", 2, transient, 3, 3, nil},
		{"exhausted", 5, transient, 3, 3, transient},
		{"permanent", 5, permanent, 3, 1, permanent},
		{"default once", 5, transient, 0, 1, transient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			job := JobFunc(func(context.Context) error {
				calls++
				if calls <= tc.failures {
					return tc.failure
				}
				return nil
			})
			results, err := RunWithOptions(context.Background(), 1, []Job{job}, Options{Retry: RetryPolicy{MaxAttempts: tc.limit, Retryable: func(err error) bool { return errors.Is(err, transient) }}})
			if err != nil || calls != tc.want || results[0].Attempts != tc.want || !errors.Is(results[0].Err, tc.wantErr) {
				t.Fatalf("calls=%d results=%+v err=%v", calls, results, err)
			}
		})
	}
}

func TestCancellationDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := make(chan struct{})
	calls := 0
	job := JobFunc(func(context.Context) error { calls++; return errors.New("temporary") })
	o := Options{Retry: RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Hour, MaxBackoff: time.Hour, Retryable: func(error) bool { return true }, Jitter: func(d time.Duration) time.Duration { close(waiting); return d }}}
	done := make(chan error, 1)
	go func() { _, err := RunWithOptions(ctx, 1, []Job{job}, o); done <- err }()
	receive(t, waiting)
	cancel()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("retried after cancellation: %d", calls)
	}
}

func TestRetryBudget(t *testing.T) {
	job := JobFunc(func(context.Context) error { return errors.New("transient") })
	o := Options{JobTimeout: 10 * time.Millisecond, Retry: RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Hour, MaxBackoff: time.Hour, Retryable: func(error) bool { return true }}}
	results, err := RunWithOptions(context.Background(), 1, []Job{job}, o)
	if err != nil || results[0].Attempts != 1 || !errors.Is(results[0].Err, ErrJobTimeout) {
		t.Fatalf("%+v %v", results, err)
	}
}

func TestContextErrorsNeverRetry(t *testing.T) {
	calls := 0
	job := JobFunc(func(context.Context) error { calls++; return context.DeadlineExceeded })
	results, err := RunWithOptions(context.Background(), 1, []Job{job}, Options{Retry: RetryPolicy{MaxAttempts: 3, Retryable: func(error) bool { return true }}})
	if err != nil || calls != 1 || !errors.Is(results[0].Err, context.DeadlineExceeded) {
		t.Fatalf("%+v %v calls=%d", results, err, calls)
	}
}

func TestBackoffAndJitter(t *testing.T) {
	p := RetryPolicy{InitialBackoff: 10 * time.Millisecond, MaxBackoff: 25 * time.Millisecond}
	for i, want := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 25 * time.Millisecond, 25 * time.Millisecond} {
		if got := retryDelay(i+1, p); got != want {
			t.Fatalf("failure=%d got=%v want=%v", i+1, got, want)
		}
	}
	p.InitialBackoff = time.Duration(math.MaxInt64/2 + 1)
	p.MaxBackoff = time.Duration(math.MaxInt64)
	if got := retryDelay(2, p); got != time.Duration(math.MaxInt64) {
		t.Fatalf("overflow: %v", got)
	}
	p = RetryPolicy{InitialBackoff: 10 * time.Millisecond, MaxBackoff: time.Second, Jitter: func(time.Duration) time.Duration { return -time.Second }}
	if retryDelay(1, p) != 0 {
		t.Fatal("negative jitter not clamped")
	}
	p.Jitter = func(time.Duration) time.Duration { return time.Hour }
	if retryDelay(1, p) != 10*time.Millisecond {
		t.Fatal("jitter exceeds exponential bound")
	}
}

func TestInvalidRetry(t *testing.T) {
	for _, p := range []RetryPolicy{{MaxAttempts: -1}, {InitialBackoff: -1}, {MaxBackoff: -1}, {InitialBackoff: time.Second, MaxBackoff: time.Millisecond}} {
		if _, err := RunWithOptions(context.Background(), 1, nil, Options{Retry: p}); !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("accepted %+v: %v", p, err)
		}
	}
}
