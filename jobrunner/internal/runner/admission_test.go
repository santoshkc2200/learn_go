package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdmissionSpacing(t *testing.T) {
	starts := make(chan time.Time, 3)
	job := JobFunc(func(context.Context) error { starts <- time.Now(); return nil })
	_, err := RunWithOptions(context.Background(), 1, []Job{job, job, job}, Options{AdmissionInterval: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := <-starts, <-starts, <-starts
	if b.Sub(a) < 15*time.Millisecond || c.Sub(b) < 15*time.Millisecond {
		t.Fatalf("admission was not spaced: %v %v", b.Sub(a), c.Sub(b))
	}
	// Lower bounds include a small scheduling allowance; this is a real-timer
	// integration check, not a promise of millisecond precision under any load.
}

func TestCancelAdmissionWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan struct{})
	job := JobFunc(func(context.Context) error { close(first); return nil })
	done := make(chan error, 1)
	go func() {
		_, err := RunWithOptions(ctx, 1, []Job{job, JobFunc(func(context.Context) error { t.Error("second job started"); return nil })}, Options{AdmissionInterval: time.Hour})
		done <- err
	}()
	receive(t, first)
	cancel()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
