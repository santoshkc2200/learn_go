# Execution ledger — jobrunner Lessons 2–5

User authorized completing the four remaining lessons in this chat.
Plan: `2026-10-09-jobrunner-lessons-2-5.md`.

- Pre-flight: all tasks share Options, Result and executeJob; preserve Run wrapper.
- Ruling: implement remaining lessons under the user's explicit instruction,
  preserving native execution from Lesson 1 rather than repeating approval gates.
  Cost if wrong: more code than a staged lesson; separate guides retain study order.
- Ruling: admission rate limit applies to new jobs, not retry attempts; cost if
  wrong: it cannot protect an external request quota, explicitly documented.
- No Git repository: keep local progress files; no worktree/commit cleanup.
- Lesson 2: tests RED (missing options API / unknown CLI flag), then 32 tests GREEN.
- Lesson 3: tests RED (missing RetryPolicy/Attempts/demo types), then 44 GREEN.
- Lesson 4: tests RED (missing Stream/interval API and flag). Initial streaming
  backpressure expectation failed consistently: traced one held forwarding result,
  one worker result and one dispatcher prefetch with a single worker.
- Ruling: use a separate dispatcher and result forwarder for readable ownership
  instead of a complex multiplexed select. Memory bound includes one forwarding
  result in addition to workers and prefetch; cost if wrong: one extra retained
  result, still independent of total input. Corrected the test's numerical bound.
- Lesson 4: complete; full suite passed 52 tests after the corrected bound.
- Lesson 5: added benchmarks, bounded fuzz properties, repeated cancellation
  accounting, labeled diagnostic trace and optional live goroutine snapshot.
  Suite passed 57 tests before the snapshot addition; final suite follows.
- Measurement commands/results saved under jobrunner/measurements. Inspected CPU,
  alloc_space, block and mutex top reports. Parsed the diagnostic execution trace
  successfully. No optimization was introduced or reliable speedup claimed.
- FuzzOptions: 2-second run passed, 32,398 executions, 21 new interesting inputs.
- Independent review dispatched for complete Lessons 2–5.
- Independent review: no production correctness findings. Found empty LESSON1.md
  caused by using a yielded read command's empty initial output without waiting.
  Restored foundational guide from the recorded original, adapted references to
  the final source and inspected the full saved file. README index is intact.
- Review test gap fixed: blocked-input cancellation now waits for a successful
  streamed result before canceling an open empty input. Removing dispatcher's
  input cancellation branch made this test fail at its timeout guard (RED).
  Restored the branch; final full-suite verification follows.
- Ruling: address the blocked-input test gap despite its Minor review label —
  it was a named Lesson 4 acceptance condition, and immediate cancel could bypass
  the boundary; cost if wrong: a small test change and extra mutation verification.
- Review limitations: CGO prevents runtime race instrumentation; real Ctrl+C is
  documented as a manual executable exercise, not claimed automated proof; short
  samples remain exploratory and README/measurement artifacts received author QA.
- Final verification after repairs: `go test -count=1 ./...` passed 58 tests;
  go vet ./... and go build ./... returned 0. Blocked-input cancellation,
  streaming joins and retry-backoff cancellation passed 10 focused repetitions
  (30 tests). All five saved guides inspected and non-empty.
- No review findings remain deferred. The intentional cancellation mutation was
  removed. Removed only generated jobrunner/runner.test.exe (recreatable by
  profiling go test); retained source, profiles, trace and readable reports.
- Tasks 1–4 complete. Local files/progress retained; no Git integration applies.
