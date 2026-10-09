package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"time"

	"example.com/jobrunner/internal/runner"
)

// A value receiver is sufficient: this job has immutable configuration and
// no shared progress counter. Copying it copies the duration value.
type delayJob struct{ duration time.Duration }

var _ runner.Job = delayJob{}

func (job delayJob) Run(ctx context.Context) error {
	timer := time.NewTimer(job.duration)
	defer timer.Stop() // Cleanup belongs to the code that creates the resource.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runCLI(parent context.Context, args []string, stdout, stderr io.Writer) int {
	// ContinueOnError keeps flag parsing testable instead of exiting the process.
	// Capture flag output so both help writes and diagnostic writes are controlled.
	var usage bytes.Buffer
	flags := flag.NewFlagSet("jobrunner", flag.ContinueOnError)
	flags.SetOutput(&usage)
	workers := flags.Int("workers", 2, "maximum concurrent jobs (positive)")
	count := flags.Int("jobs", 6, "number of demo jobs (nonnegative)")
	duration := flags.Duration("duration", 100*time.Millisecond, "work per job (nonnegative)")
	timeout := flags.Duration("timeout", 2*time.Second, "batch deadline (positive)")
	jobTimeout := flags.Duration("job-timeout", 0, "budget per job; zero disables it")
	attempts := flags.Int("attempts", 1, "maximum attempts per job, including first")
	failFirst := flags.Int("fail-first", 0, "simulate this many temporary failures per job")
	backoff := flags.Duration("backoff", 50*time.Millisecond, "initial retry delay")
	maxBackoff := flags.Duration("max-backoff", time.Second, "maximum retry delay")
	jitter := flags.Bool("jitter", false, "randomize retry delay within its exponential bound")
	stream := flags.Bool("stream", false, "consume incrementally and print completion order")
	interval := flags.Duration("interval", 0, "minimum spacing between new job admissions")
	flags.Usage = func() {
		fmt.Fprintln(&usage, "Usage: jobrunner [flags]")
		flags.PrintDefaults()
	}
	// stderr is our last reporting destination. A diagnostic write failure is
	// best-effort: the failing operation still determines the nonzero exit code.
	diagnostic := func(err error) { fmt.Fprintf(stderr, "jobrunner: %v\n", err) }
	write := func(s string) error {
		n, err := io.WriteString(stdout, s)
		if err == nil && n != len(s) {
			err = io.ErrShortWrite
		}
		return err
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if err := write(usage.String()); err != nil {
				diagnostic(fmt.Errorf("write help: %w", err))
				return 1
			}
			return 0
		}
		diagnostic(err)
		return 2
	}
	if flags.NArg() != 0 {
		diagnostic(errors.New("positional arguments are not supported"))
		return 2
	}
	if *workers <= 0 || *count < 0 || *duration < 0 || *timeout <= 0 || *jobTimeout < 0 || *interval < 0 {
		diagnostic(errors.New("workers and timeout must be positive; jobs and duration must be nonnegative"))
		return 2
	}
	if *attempts < 1 || *failFirst < 0 || *backoff < 0 || *maxBackoff < 0 || (*maxBackoff > 0 && *maxBackoff < *backoff) {
		diagnostic(errors.New("invalid retry counts or backoff"))
		return 2
	}
	options := runner.Options{JobTimeout: *jobTimeout, Retry: runner.RetryPolicy{MaxAttempts: *attempts, InitialBackoff: *backoff, MaxBackoff: *maxBackoff, Retryable: func(err error) bool { var temporary *TemporaryError; return errors.As(err, &temporary) }}}
	options.AdmissionInterval = *interval
	if *jitter {
		options.Retry.Jitter = func(d time.Duration) time.Duration {
			if d <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(d)))
		}
	}
	// Context is an explicit parameter, not a field stored in a long-lived job.
	// Cancel also releases the deadline's resources on successful early completion.
	ctx, cancel := context.WithTimeout(parent, *timeout)
	defer cancel()
	code := 0
	emit := func(result runner.Result) error {
		status := "succeeded"
		switch {
		case errors.Is(result.Err, context.Canceled), errors.Is(result.Err, context.DeadlineExceeded):
			if result.Started {
				status = "canceled (started)"
			} else {
				status = "canceled (not started)"
			}
		case result.Err != nil:
			status = fmt.Sprintf("failed: %v", result.Err)
		}
		if result.Err != nil {
			code = 1
		}
		if *attempts > 1 {
			status += fmt.Sprintf(" (attempts=%d)", result.Attempts)
		}
		return write(fmt.Sprintf("job %d: %s\n", result.Index, status))
	}
	newJob := func(int) runner.Job { return &demoJob{duration: *duration, failFirst: *failFirst} }
	if *stream {
		if err := streamCLI(ctx, *workers, *count, options, newJob, emit); err != nil {
			diagnostic(err)
			return 1
		}
		return code
	}
	jobs := make([]runner.Job, *count)
	for i := range jobs {
		jobs[i] = newJob(i)
	}
	results, batchErr := runner.RunWithOptions(ctx, *workers, jobs, options)
	for _, result := range results {
		if err := emit(result); err != nil {
			diagnostic(fmt.Errorf("write summary: %w", err))
			return 1
		}
	}
	if batchErr != nil {
		diagnostic(batchErr)
		return 1
	}
	return code
}
