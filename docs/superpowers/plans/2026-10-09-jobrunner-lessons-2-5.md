# Jobrunner Lessons 2–5 Implementation Plan

> **For agentic workers:** Use executing-plans for native execution and one final independent review. Track each task below and retain the progress record in this non-Git workspace.

**Goal:** Complete the four remaining main lessons authorized by the learner.

**Architecture:** Preserve Run as the simple batch API. Add RunWithOptions for per-job deadlines and retries, then a synchronous Stream API with caller-owned input/output channels and completion-order output. Share job execution policy between both entry points. The CLI adds demos and signal cancellation; separate lesson documents explain each change.

**Tech Stack:** Go 1.27.0 and standard library only.

**Spec:** `../specs/2026-10-09-jobrunner-design.md`, project two lesson sequence; the learner explicitly requested implementing all remaining lessons.

## Constraints and decisions

- Existing Lesson 1 behavior and tests remain valid; preserve its guide as LESSON1.md.
- Options uses plain fields: JobTimeout, Retry, AdmissionInterval. No framework.
- JobTimeout is one budget spanning all attempts/backoff, starting at worker execution.
- Retry MaxAttempts includes the initial call; zero means one. Retry only explicitly classified errors. Never retry context errors. Backoff doubles without duration overflow, capped at MaxBackoff. Inject optional jitter; clamp its output to [0, capped delay]. Callbacks must be safe for concurrent calls.
- Result adds Attempts; Started means an actual Run call happened. Preserve context error identity and cancellation cause using joined errors.
- AdmissionInterval spaces new job admissions globally; first admission is immediate. It does not rate-limit retry attempts. Streaming outputs in completion order, not input order.
- Stream does not close caller channels. Caller closes input, and closes output only after Stream returns. Cancellation unblocks input, admission and output waits. On cancellation results may be dropped; batch RunWithOptions retains one result per input.
- Stream validates options and non-nil channels up front. Nil jobs in a stream produce failed, unstarted results, allowing the stream to continue. Stream waits for all its workers; producers belong to the caller and must observe the same context.
- No provisioning, dependencies, Git operations or compiler installation. Race-check limitations are reported honestly.

## Review focus

Cancellation during retry sleep; retry side effects/idempotency; canceled blocked output;
nil input channel versus closed empty channel; backoff overflow and unsafe jitter.
Tests pin all five cases. Arbitrary callbacks/jobs still require cooperation.

## Task 1 — Lesson 2 cancellation

Files: options.go/options_test.go, runner.go, main.go, cli.go/cli_test.go, LESSON2.md.

- [x] Test-first Options{JobTimeout}, RunWithOptions, deadline/cause propagation,
  shorter job deadline isolated from the batch, invalid timeout and success.
- [x] Implement WithTimeoutCause and ErrJobTimeout, preserve errors.Is for both
  deadline/cancellation and cause; keep Run as zero-options wrapper.
- [x] Add -job-timeout flag and signal.NotifyContext(os.Interrupt) in main; test
  deadline CLI path. Explain Ctrl+C versus injected cancellation in tests.
- [x] Run tests, document overall/per-job deadline distinction and ownership.

## Task 2 — Lesson 3 retries

Files: retry.go/retry_test.go, options.go, demo.go/demo_test.go, cli.go, LESSON3.md.

- [x] Write failing tests: temporary failure then success, permanent failure,
  exhausted attempts, original identity, cancel during backoff, deadline across
  attempts, safe capped exponential growth including near max duration, jitter
  bounds and invalid configuration.
- [x] Implement RetryPolicy and shared executeJob with Attempts accounting and
  interruptible timers. Use errors.As with a custom TemporaryError in local demos.
- [x] Add -attempts, -fail-first, -backoff, -max-backoff and -jitter flags. Defaults
  remain one attempt/no simulated failures. Each demo job owns its attempt state.
- [x] Run tests; document idempotency and context errors never retried.

## Task 3 — Lesson 4 streaming and admission

Files: stream.go/stream_test.go, admission.go/admission_test.go, cli_stream.go,
cli.go/cli_test.go, LESSON4.md.

- [x] Test-first Stream(ctx,workers,input,output,options) error: completion order,
  bound/backpressure, nil job, closed input, bad channels, cancel blocked input
  and output, all workers joined, first immediate admission and later spacing.
- [x] Implement worker fan-out and result fan-in; multiplex input, dispatch and
  results with select/nil channels. Limit pending work to one job plus workers.
  Make each blocking send and wait cancellable. Stop internal channels in order.
- [x] Add -stream and -interval flags. CLI producer observes ctx; on output error
  cancel and join the producer/runner; display attempts when retries enabled.
- [x] Run tests; teach channel ownership, completion order, backpressure and
  cancellation result loss. Compare batch versus streaming memory.

## Task 4 — Lesson 5 diagnostics and measurement

Files: benchmark_test.go, diagnostics_test.go, fuzz_test.go, LESSON5.md,
measurements/README.md and saved local results; README.md becomes lesson index.

- [x] Benchmarks compare no-op/CPU jobs at workers 1/2/4; fresh batches, real output
  checks, allocations, clear operations. Add blocking workload for trace/profiles.
- [x] Fuzz option validation with capped input sizes; repeated cancellation tests
  join explicit child lifetimes rather than asserting global goroutine counts.
- [x] Run test/vet/build and race attempt; save short repeated benchmark samples,
  separate CPU/heap/block/mutex profiles and trace, inspect top reports and trace
  diagnostics. Explain results without promising an unmeasured optimization.
- [x] Provide commands for longer measurements, scheduler/GC/escape observations
  and a supported race environment. No intentionally racy code in the normal suite.
- [x] Final independent review, repair findings, rerun checks. Retain local ledger.

## Coverage and self-review

The five main lessons are complete after these tasks. Companion labs for advanced
language features and specialist internals remain curriculum extensions, not part
of the five-lesson runner. The third HTTP/database project remains separate.

Every blocking boundary has a cancellation test; no design promises forced
termination. Streaming cancellation deliberately trades complete output for
prompt shutdown. Documentation must make that difference explicit.
