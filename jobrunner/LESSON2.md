# Lesson 2 — cancellation, deadlines and causes

Roadmap chapters 3, 4, 10 and 12. Read `internal/runner/options.go`,
`options_test.go`, `main.go` and `TestCLIJobDeadline` in `cli_test.go`.
Lesson 1 is preserved in LESSON1.md; the source now contains all five lessons.

## Run it

```powershell
Set-Location C:\Users\user\personal\dev\go\jobrunner
go run . -workers 1 -jobs 3 -duration 100ms -job-timeout 20ms -timeout 2s
go run . -workers 2 -jobs 6 -duration 200ms -timeout 50ms
go test ./internal/runner -run 'TestJobDeadline|TestCancellationCause|TestRunWaits' -v
```

The first command cancels each started job's child context, but continues the
batch. All three outcomes are `canceled (started)` and the exit status is 1.
The second command cancels the parent: unstarted jobs are canceled too. Stdout
and stderr can interleave in your terminal; the streams serve different purposes.

## Context is a tree of lifetimes

The CLI's overall timeout covers admission, execution, backoff and shutdown.
Each worker derives a child context for a job's budget. The child budget starts
when execution begins; time waiting in the input queue is not included. One job
budget covers every retry and retry delay. Canceling the parent cancels children;
canceling a child does not cancel siblings or the parent.

`WithTimeoutCause` records ErrJobTimeout as the child's explanation. `ctx.Err()`
still reports DeadlineExceeded; `context.Cause(ctx)` reports why the deadline was
configured. `contextError` joins them so callers can use errors.Is for either.
Joined errors may contain newlines in their Error text: inspect identity in tests,
not exact diagnostic sentences. Custom causes also survive WithCancelCause.

The defer in executeJob belongs to a single job call. Putting a defer inside a
worker's long loop would retain every timer until the worker exits. A separate
function gives cleanup a narrower lifetime. Its child context is canceled when
that job finishes, even on success before the deadline.

## Signals at the process boundary

`main` uses signal.NotifyContext with os.Interrupt. Run a long batch and press
Ctrl+C:

```powershell
go run . -jobs 20 -duration 1s -timeout 1m
```

For a direct executable experiment, `go build -o jobrunner.exe .` then run
`.\jobrunner.exe` with those flags. `go run` introduces an extra wrapper process,
so signal/exit presentation can vary with the shell. The program unregisters its
signal notification before os.Exit. Unit tests inject canceled contexts instead
of sending Ctrl+C to the user's terminal.

Go cannot kill a job that ignores cancellation. Run joins started jobs, so an
uncooperative job can delay return beyond both deadlines. A callback that blocks
forever has the same limitation. Context is a cooperative contract, not a watchdog
that forcibly terminates code.

## Predict, then change

1. Compare an overall 50 ms budget with a per-job 50 ms budget and workers=1.
   Which jobs start? Which cancellation should appear as the batch error?
2. In TestCancellationCause, replace the custom cause with a different sentinel.
   Why can errors.Is match both that sentinel and context.Canceled?
3. Read TestRunWaitsForStartedJob. Why does acknowledging cancellation not mean
   cleanup has finished? The bounded negative observation gives early returns
   time to become visible; the release channel still controls cleanup ordering.
4. Call executeJob with a job that captures mutable state. Context propagation
   does not protect that state; explain which goroutine owns it.

Appropriate context metadata appears in the diagnostics lesson's profiling
labels. Configuration, job data and optional function parameters belong in
ordinary arguments rather than context values.
