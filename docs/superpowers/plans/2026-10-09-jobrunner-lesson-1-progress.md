# Execution ledger — jobrunner Lesson 1

Plan: `2026-10-09-jobrunner-lesson-1.md`; approved for native execution.

- Pre-flight: Tasks 1–3 share Job, Result and Run signatures; consistent.
- Ruling: use this local progress file instead of Git-dependent scratch scripts,
  commits and worktrees — workspace has no Git repository, as the approved plan
  records; cost if wrong: less automated recovery, mitigated by preserved files.
- Task 1: complete. Tests first failed with missing Run/Job symbols; sequential
  baseline passed all six tests/examples (`go test -count=1 ./internal/runner`).
- Task 2: complete. Sequential baseline failed TestRunBoundedConcurrency at the
  second-start barrier; bounded implementation passed all 11 runner tests/examples.
- Task 3: tests first failed with missing runCLI/delayJob symbols. Implementation
  added checked output, validated flags, ordered summaries and cooperative timers.
- Task 3: complete. `go test -count=1 ./...` passed 28 tests in two packages.
- Task 4: README covers ownership, memory bounds, interfaces, typed nil,
  closure capture, deadlines, joining, synchronization and broken-code exercises.
- Verification: gofmt applied; go vet ./... and go build ./... returned 0.
  Success demo emitted six successes. Timeout demo emitted two started canceled
  outcomes and four unstarted canceled outcomes, diagnostic deadline exceeded,
  exit status 1. Exact started counts are scheduling-dependent.
- Race check attempted: `go test -race -count=1 ./...` returned
  `go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`.
  No race-check pass claimed. No compiler installation in this lesson.
- Independent review: no production correctness defects. Important joining-test
  gap fixed: hold cleanup blocked for a bounded negative observation rather than
  select/default at one instant. An intentional early-return mutation failed
  TestRunWaitsForStartedJob with "returned while job was still running"; mutation
  removed, production Wait retained. Final suite rerun below.
- Ruling: allow a 100 ms negative observation alongside channel barriers — absence
  of return cannot be observed by a positive handshake alone; cost if wrong:
  unusual scheduler delay could still conceal a bad implementation. Documentation
  states this is bounded evidence, not a proof of every schedule.
- Ruling: treat the review's duration-exercise mismatch as a teaching defect and
  repair it — ExampleRun had no duration to change; cost if wrong: a small prose
  change. Exercise now specifies a concrete timer closure adaptation.
- Review limitations resolved: race freedom remains unverified without CGO;
  non-nil ctx and valid receiver contracts remain documented preconditions;
  historical RED/GREEN observations are recorded from executor tool output.
- Task 4: complete. Fresh final verification after review repairs: 28 tests
  passed in two packages; go vet and go build returned 0. No TODOs, broken
  mutations or generated executable artifacts remain. No deferred findings.
- Finish: local files retained, per the approved non-Git workspace plan. No
  branch integration or cleanup menu applies; source and ledger are the record.
