# Lesson 3 — deliberate retries

Roadmap chapters 2, 3, 4, 10, 11 and 12. Read `demo.go`,
`internal/runner/retry.go`, the attempt loop in `options.go` and `retry_test.go`.

```powershell
Set-Location C:\Users\user\personal\dev\go\jobrunner
go run . -jobs 3 -duration 0 -fail-first 2 -attempts 3 -backoff 10ms -max-backoff 40ms
go run . -jobs 1 -duration 0 -fail-first 5 -attempts 3 -backoff 10ms
go test ./internal/runner -run 'TestRetries|TestCancellationDuringBackoff|TestRetryBudget|TestBackoff' -v
```

The first command succeeds after three attempts per job. The second exhausts
three attempts and returns 1 with a failed result retaining the last error.
MaxAttempts counts the initial call, not just retries; zero in the Go API means
one attempt. The CLI requires a positive -attempts. Defaults retain Lesson 1's
one-call behavior.

## A classifier is a policy decision

TemporaryError is a custom pointer error carrying an attempt number. The CLI
uses errors.As to recognize it; the runner only invokes the supplied Retryable
function. With no classifier, nothing retries. Errors.Is handles sentinel identity;
errors.As finds a type and its data through wrapping/joining.

The runner never retries context.Canceled or context.DeadlineExceeded, even
with an overly broad classifier. JobTimeout spans the complete sequence: creating
a fresh full deadline for each attempt would allow total time to multiply.

demoJob has a pointer receiver because its counter must survive across attempts.
Each input receives a separate pointer. One worker calls that job sequentially,
so its counter needs no lock. Sharing the same pointer between inputs would break
that ownership rule. The runner does not implicitly deep-copy a job.

## Backoff and jitter

After failures 1, 2, 3, ... the base delay is initial, 2×initial, 4×initial, ...,
capped at MaxBackoff. A zero MaxBackoff defaults to max(initial, 1 second).
Zero initial backoff means immediate retries. Negative options and a configured
cap smaller than initial are rejected before launching goroutines.

The calculation saturates before doubling, so time.Duration's signed integer
cannot overflow into a negative delay. The loop stops once capped; a large
attempt number does not imply an equally large backoff-calculation loop.

`-jitter` draws a delay from [0, base) using concurrency-safe top-level rand/v2.
Randomness spreads retry waves among workers; it is not cryptography. The Go API
accepts a Jitter callback so tests can choose exact behavior. Its result is clamped
to [0, base], even if it returns a negative or enormous duration. Callbacks can
run concurrently and must be prompt and thread-safe.

Every delay uses a timer and selects on ctx.Done. A worker occupies its slot
while backing off; this simple policy reduces throughput under heavy failures.
Scheduling delayed retries in a separate queue is a later architecture choice,
not a hidden feature of this runner.

## Retrying side effects

A temporary transport failure can happen after an operation committed. Retrying
a payment, email or database insert can duplicate effects even when the error is
transient. This local demo has no external effects. In the HTTP/database project,
use idempotency keys and transactional deduplication where replay is allowed.
Classification says an error may recover; it does not establish safe replay.

## Exercises

1. Predict Attempts for temporary failures, permanent failures and exhaustion
   before reading TestRetries. Explain why batch error can remain nil while one
   result failed.
2. Run `-fail-first 10 -attempts 10 -backoff 1s -job-timeout 50ms`. Why should only
   one attempt start despite the larger attempt limit?
3. Compare jitter enabled/disabled with many jobs. Timing varies; do not assert
   a particular random schedule. Tests inject a deterministic callback.
4. Add a wrapped TemporaryError and check errors.As still classifies it. Add a
   context error wrapped with %w and check it is never retried.
5. In TestBackoffAndJitter, explain why checking before multiplication matters
   near the largest representable duration. Restore any experimental broken code.
