# Jobrunner Lesson 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an annotated, runnable bounded job runner that teaches worker ownership, cooperative cancellation and deterministic concurrency testing.

**Architecture:** A thin command calls an internal batch runner through a small Job interface. The caller dispatches indexed jobs; workers own separate result slots and a WaitGroup joins them before results are returned. Local timer jobs demonstrate the behavior without network dependencies.

**Tech Stack:** Go 1.27.0, standard library, PowerShell examples, testing and go vet.

**Spec:** `docs/superpowers/specs/2026-10-09-jobrunner-design.md` (approved).

## Global Constraints

- Independent standard-library-only module under `jobrunner/`, module `example.com/jobrunner`.
- Keep `streamgrep` intact. Implement runner Lesson 1 only.
- Input order determines result order; execution order is unspecified.
- Ordinary job failures are recorded and other jobs continue.
- Always wait for already started jobs; cancellation is cooperative.
- No signal handling, retries, rate limiting, persistence or networking in this lesson.
- Explain concepts in source and tests; provide working code, not required TODOs.
- No Git repository exists: preserve local files and execution evidence, without commits or worktree creation.
- Installed environment: go1.27.0 windows/amd64, CGO_ENABLED=0, no gcc found. Attempt the race check and report actual support/failure; do not claim a pass without evidence.

## Review Focus

1. More workers than jobs: start no more workers than useful jobs, preserve results.
2. Cancellation before an empty batch: validate arguments first, then return ctx.Err; successful empty input on a live context returns an empty result slice.
3. Cancellation during admission: never leave an unstarted result with a misleading success value.
4. Help/output writer failures: surface I/O failure as exit code 1 even when rendering help.
5. Duration zero and malformed flags: zero-duration jobs succeed, invalid values return 2 without starting jobs.

## Task 1: Batch API and failure contracts

**Files:** Create `jobrunner/go.mod`, `jobrunner/internal/runner/runner.go`, `jobrunner/internal/runner/runner_test.go`, `jobrunner/internal/runner/example_test.go`.

**Interfaces:**

- `type Job interface { Run(context.Context) error }`
- `type JobFunc func(context.Context) error`, method `Run(context.Context) error` delegates to the function.
- `type Result struct { Index int; Started bool; Err error }`
- `var ErrInvalidWorkers = errors.New("workers must be positive")`
- `var ErrNilJob = errors.New("job is nil")`
- `func Run(ctx context.Context, workers int, jobs []Job) ([]Result, error)`.
- ctx must be non-nil, following normal context conventions. A nil JobFunc and a typed nil pointer are the job implementation's responsibility; do not use reflection.

- [x] Write table tests `TestRunValidation` and `TestRunEmpty`: workers 0/-1 match ErrInvalidWorkers, a nil interface matches ErrNilJob with index context, invalid jobs are rejected before execution, live empty input succeeds, pre-canceled empty input matches context.Canceled. Add `TestJobFunc` to prove error identity and context propagation.
- [x] Run `go test ./internal/runner -run 'TestRunValidation|TestRunEmpty|TestJobFunc' -v` from `jobrunner`; record the expected missing implementation failure.
- [x] Create go.mod with `go 1.27.0`, implement the API with an initially sequential baseline and argument validation. Create input-indexed results, check cancellation before job execution and mark unstarted jobs with ctx.Err. The sequential baseline is a temporary teaching implementation, not final acceptance.
- [x] Add `TestRunOutcomes`: three jobs, middle job returns a sentinel; all start once, result indices match inputs, only the middle result has that error and Run returns nil. Add a compile-time pointer receiver satisfaction assertion and `TestTypedNilJob` showing a nil-safe pointer receiver inside a non-nil interface.
- [x] Add `ExampleRun` printing ordered outcomes without asserting scheduling order. Run `go test ./internal/runner -v`; confirm contracts pass.

## Task 2: Bounded workers and cancellation

**Files:** Modify `jobrunner/internal/runner/runner.go` and `runner_test.go`.

**Interfaces:** Consume Task 1 API unchanged. Run returns context error for observed batch cancellation, preserves started outcomes and fills every unstarted outcome with that error.

- [x] Add `TestRunBoundedConcurrency`: four jobs, workers=2, buffered started notifications and a release barrier. Observe two starts before releasing either, track active/max with synchronization, and assert four starts with max=2. This also proves more than one worker is active. Register release/cancel cleanup and bounded timeout guards.
- [x] Run `go test ./internal/runner -run TestRunBoundedConcurrency -v`; record failure of the sequential baseline.
- [x] Implement an unbuffered indexed work channel; launch min(workers,len(jobs)) workers, dispatch in the calling goroutine with select on ctx.Done, close the channel only in the dispatcher and wait for every worker. Each worker checks ctx.Err before calling Job.Run and writes only its assigned result slot. Fill unstarted cancellation outcomes after Wait. Explain why Wait makes writes visible and why channel transfer does not make arbitrary job state thread-safe.
- [x] Add `TestRunMoreWorkersThanJobs`, `TestRunPreCanceled`, `TestRunCancellationDuringAdmission` and `TestRunWaitsForStartedJob`. For the last test, use a job that acknowledges cancellation then waits on a release channel; assert Run has not returned while that job is held, release it, then require return. Use channel barriers, cleanup and bounded guards instead of sleep-based scheduling assumptions.
- [x] Run `go test -count=1 ./internal/runner -v`; confirm all jobs execute at most once, cancellation identity is retained, results are ordered and no started child outlives Run.

## Task 3: Runnable local CLI

**Files:** Create `jobrunner/main.go`, `jobrunner/cli.go`, `jobrunner/cli_test.go`.

**Interfaces:**

- `func runCLI(parent context.Context, args []string, stdout, stderr io.Writer) int`.
- `type delayJob struct { duration time.Duration }` implements Job using a stopped timer and select on ctx.Done.
- main calls `os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr))`; all defers belong inside runCLI, before os.Exit.

- [x] Write `TestCLIArguments`: defaults workers=2, jobs=6, duration=100ms, timeout=2s; help returns 0; unknown flags, positional arguments, workers<=0, jobs<0, duration<0 and timeout<=0 return 2. Zero jobs and zero duration succeed. Inject a pre-canceled parent to test cancellation without wall-clock timing.
- [x] Write `TestCLIOutputFailure`: a writer returning a sentinel causes exit code 1, including help rendering. Summary tests assert ordered lines `job 0: succeeded`, etc.; canceled lines distinguish started and unstarted jobs. Diagnostic checks assert meaningful context without pinning entire flag library messages.
- [x] Run `go test . -run TestCLI -v`; record missing implementation failure.
- [x] Implement flags with flag.ContinueOnError, explicit help rendering to stdout and checked writes. Validate before creating jobs. Use context.WithTimeout(parent, timeout) with deferred cancel. Print one ordered outcome per input, distinguish failed/canceled via errors.Is, and check all summary writes. Return 1 on batch/job/output errors, 2 on arguments, otherwise 0. Keep stderr diagnostic failure best-effort because no further reliable reporting destination exists.
- [x] Add `TestDelayJobCancellation` with an already canceled context and long duration; guard against hanging. Run `go test -count=1 ./...`.

## Task 4: Teaching chapter, review and evidence

**Files:** Create `jobrunner/README.md` and `docs/superpowers/plans/2026-10-09-jobrunner-lesson-1-progress.md`; add explanatory comments to the files above as needed.

**Interfaces:** Documentation describes the final API and behavior already tested; it must not promise immediate termination of a job that ignores ctx.

- [x] Explain reading order (runner, tests, CLI, example), roadmap coverage, interface satisfaction/method sets, slice ownership, channel closing, select cancellation races, WaitGroup visibility, partial outcomes and process exit behavior. Link the full 20-chapter curriculum design. Provide exercises about scheduling/order, shared-state races, typed nil and cooperative cancellation; label broken experiments and restore instructions.
- [x] Include PowerShell success and early-timeout commands from the spec, `go test`, `go vet`, `go build`, and `go test -race`. Explain go run's wrapper status and that Lesson 2 adds richer cancellation rather than claiming those features exist.
- [x] Run gofmt over new Go files. Run `go test -count=1 ./...`, `go vet ./...`, and `go build ./...` from jobrunner; all must succeed. Run success and timeout commands and verify actual outputs.
- [x] Attempt `go test -race -count=1 ./...`; record failure if this environment cannot support it. Provide the same command for a supported Go installation with CGO/compiler support. Do not install a compiler as part of this lesson.
- [x] Request one independent review of runner ownership, cancellation and CLI contracts; repair actionable findings and rerun affected checks. Record RED/GREEN observations, final checks and any platform limitation in the progress file.
- [x] Check final files for placeholders and generated executable artifacts; remove only binaries created during this task. Deliver source/test/README links and one runnable example.

## Self-review and handoff

The four tasks cover the approved Lesson 1 scope; later roadmap coverage remains
curriculum only. API signatures and result fields are shared consistently.
The five review conditions have tests in their owning tasks. No new dependencies,
external services, Git operations or streamgrep changes are required.

Recommended execution: native implementation in this session, followed by one
independent review. The tasks share a small API and this is a local teaching
application; per-task agent handoffs would add overhead without improving the
learner's chapter.
