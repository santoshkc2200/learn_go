# Three Go projects and the next chapter: jobrunner

## Learning goal

The learner knows basic Go syntax. The completed `streamgrep` project teaches
through working code, explanatory comments, failure tests, runnable PowerShell
examples, and prediction exercises. Continue that format. Build project two in
small lessons rather than delivering every concurrency feature in one chapter.

Interpret “next chapter” as project two, Lesson 1. Roadmap chapter numbers and
project lesson numbers are different: each project combines several roadmap
chapters. Concepts already encountered in streamgrep can be revisited explicitly.

## Which application teaches which concepts?

“Primary” identifies the application with the main explanation and exercises.
Follow-up applications apply the concept in a different setting.

| Roadmap chapter | Primary application and exercise | Follow-up application |
| --- | --- | --- |
| 1. Value semantics and memory | Streaming CLI: byte slices, buffer reuse, aliasing, strings/bytes/runes; add a focused companion lab for maps, zero values, shallow/deep copies and ownership. | Runner: transfer immutable job values to workers. |
| 2. Methods, interfaces, composition | Runner: a small consumer-owned Job interface, value/pointer method sets, embedding, assertions, type switches and typed nil exercises. | Service: storage interfaces and composed handlers. |
| 3. Functions and control flow | Runner: job adapters, closures, captured variables, defer ordering and cleanup; add options only when configuration grows. Demonstrate panic/recover in an isolated test lab. | Service: middleware closures and deliberate recovery at the HTTP boundary. |
| 4. Errors and API contracts | Streaming CLI: wrapping, partial output and error identity; runner adds custom job errors and joined failures. | Service: translate domain failures to HTTP responses. |
| 5. Generics and iterators | Runner companion lab: typed result collection, comparable IDs, underlying-type constraints and range-over-function cancellation. Compare concrete, interface and generic designs. | Service: pagination iteration without forcing generics into business logic. |
| 6. Packages and modules | Runner: separate command from an internal runner package, explain dependency direction and module boundaries. | Service: dependency choice, workspaces and API/version compatibility exercises. |
| 7. Standard library | Streaming CLI: Reader/Writer, buffering and cleanup; runner adds time/timers and JSON result output. | Service: JSON validation/custom marshaling and io/fs fixtures; focused regex/sort/collection exercises. |
| 8. Concurrency fundamentals | Runner: goroutines, channels, select, channel closing/ownership, nil-channel exercises, deadlocks and leak prevention. | Service: request/server lifetimes. |
| 9. Synchronization and memory model | Runner: WaitGroup and ownership first; compare channels with Mutex/RWMutex, demonstrate Once/Cond/atomic visibility in companion labs and run race detection. | Service: synchronized metrics and shared state. |
| 10. Context and structured concurrency | Runner: parent/child cancellation, deadlines, cancellation causes and joining every worker. | Service: request context propagation and appropriate context values. |
| 11. Advanced concurrency patterns | Runner: bounded worker pool, backpressure, fan-out/fan-in, interruptible backoff/jitter, rate limiting and partial failure. | Service: graceful shutdown and safe background work. |
| 12. Testing and correctness | All three: streamgrep failure fakes; runner coordination tests, cleanup and race checks; service HTTP/database integration tests. | Focused fuzzing, examples, parallel tests and coverage limitations. |
| 13. Benchmarking/performance | Streaming CLI has measurement-before-optimization; runner adds concurrency benchmarks, goroutine/block/mutex profiles and tracing. | Service: CPU/heap profiles under a representative load and repeated comparisons. |
| 14. Runtime and GC | Runner: scheduling/GOMAXPROCS and stack/heap experiments. | Service: escape analysis, GC/memory limits, retention and measured sync.Pool experiments. |
| 15. Networking and HTTP | Database-backed service: reusable client, timeouts, TLS/TCP concepts, handlers, middleware, streaming, limits and shutdown. | Optional runner HTTP jobs after the local concurrency lessons. |
| 16. Database programming | Service: database/sql pools, scanning, prepared statements, cancellation, transactions/isolation, migrations, pagination and query plans. | No database dependency in the runner. |
| 17. Application architecture | Runner: small package boundaries and constructor injection. | Service: business logic separated from transport/storage, configuration and lifecycle. |
| 18. Reliability and security | Service: structured logs, metrics/traces, health, validation, authentication/authorization, crypto, secrets, dependency scanning and idempotency. | Runner: classify retryable failures and explain duplicate side effects. |
| 19. Build and delivery | Service: CI, containers, operational configuration and deployment. | All projects: formatting/vet/build; labs cover tags, cross-compilation, metadata and reproducibility. |
| 20. Specialist internals | Optional service companion labs: reflection versus code generation, AST inspection, unsafe/alignment/pointer constraints, cgo ownership/cost and PGO. | These are explicit isolated specializations, not required production dependencies. |

The existing CLI does not exhaust every item in chapters 1–7. The companion
labs above close those gaps rather than labeling them already mastered.
Each future lesson must identify its roadmap items and a test or exercise that
demonstrates them. All concepts have a home; they are not all implemented now.

## Approaches considered

1. **Local job runner (recommended).** Jobs simulate work with cancellable timers.
   Repeatable behavior and no network dependencies make ownership and shutdown
   easier to observe. Later inject deterministic transient failures for retries.
2. **Web crawler.** Real HTTP work is motivating, but URL normalization, network
   variation, rate limits and crawl scope obscure the first concurrency lesson.
   Keep HTTP as an optional runner extension and teach it fully in project three.
3. **Runner with persistence immediately.** Combines concurrency and database
   recovery but makes a chapter too large and duplicates the service's role.

## Project two lesson sequence

1. **Own worker lifetimes.** Implement a working bounded runner with cancellation,
   explicit outcomes, explanatory tests and a local command demonstration.
2. **Study cancellation.** Add signal cancellation, overall/per-job deadlines,
   causes, blocked-job experiments and documented cooperative cancellation.
3. **Retry deliberately.** Add explicit retry classification, maximum attempts,
   interruptible exponential backoff, injectable jitter and idempotency exercises.
4. **Control admission and results.** Extend from a fixed batch to streaming
   admission, backpressure, fan-in/fan-out and rate limiting. Establish a separate
   ownership contract for input/output channels before implementing it.
5. **Prove and measure.** Expand race/leak/failure exercises; benchmark worker
   counts and inspect block/mutex/goroutine profiles and execution traces.

Methods/interfaces, function semantics, generics/iterators and synchronization
companion labs fit between these lessons. Avoid adding abstractions solely to
mention a language feature; teach alternatives in contained examples.

## Lesson 1 design

Create an independent standard-library-only module under `jobrunner/`, matching
the installed Go toolchain after checking its version. Keep `streamgrep` intact.

Files:

- `go.mod`: independent module `example.com/jobrunner`.
- `main.go`: thin process entry point; command setup delegates to testable code.
- `cli.go`, `cli_test.go`: flags, context deadline, demo jobs and output handling.
- `internal/runner/runner.go`: reusable bounded batch runner.
- `internal/runner/runner_test.go`, `example_test.go`: deterministic behavioral
  tests and executable API example.
- `README.md`: learning path, annotated source reading order, commands,
  expected behavior, design explanation and prediction exercises.

### API and ownership

Use a small `Job` interface with `Run(context.Context) error`, and a `JobFunc`
adapter. Provide `Run(ctx context.Context, workers int, jobs []Job)
([]Result, error)`. Each Result contains the original input index and job error.

Results preserve input order although execution order is unspecified. The input
slice is borrowed for the call: callers must not mutate it while Run is active.
A job must honor ctx and must synchronize shared mutable data itself. Passing
the same stateful job more than once does not make it safe to run concurrently.

Validate worker count before starting any goroutine. A nil interface job is an
argument error; demonstrate that an interface containing a nil pointer is a
different value in a teaching test. Do not add reflection to detect typed nil.
Require job implementations to satisfy their receiver contract.

The calling goroutine dispatches indexed jobs over an unbuffered channel. Start
at most min(workers, len(jobs)) worker goroutines. Only the dispatcher closes the
work channel. Workers check cancellation before invoking each admitted job and
write to their job's exclusive result slot. A WaitGroup joins all workers before
results are returned, providing visibility of their completed writes.

Each result also records whether its job started, so unstarted cancellation is
distinguishable from a completed nil-error job. Mark every unstarted outcome with
the cancellation error when dispatch stops. This permits explaining partial work
without pretending canceled jobs succeeded.

Ordinary job failures are recorded and other jobs continue. The returned error
represents invalid arguments or batch cancellation; individual failures live in
results. Explain this distinction beside the API and at the CLI call site.
Return the context error when cancellation is observed during the batch. A
completed batch may finish immediately before a concurrent cancel; there is no
promise that a job never starts at exactly the same instant cancellation occurs.

Always wait for already started jobs. Context cancellation is cooperative: a job
that ignores ctx can delay shutdown indefinitely. Do not hide this by abandoning
a goroutine or claiming Go can forcibly terminate one.

### CLI behavior

`go run . -workers 2 -jobs 6 -duration 100ms -timeout 2s` runs numbered local demo
jobs. Flags validate positive workers, nonnegative job count/duration and positive
timeout. No positional arguments. `-h` succeeds and invalid arguments return 2.
Jobs use a timer and select on ctx.Done; stop the timer on return.

Print stable summaries in input order after Run finishes. Include started,
succeeded, failed and canceled outcomes. Diagnostics go to stderr. Return 0 for
success/help, 1 for job failure/cancellation/output failure, and 2 for invalid
arguments. Check output-write errors. Do not assert exact elapsed timings.

An early-timeout command demonstrates partial work:
`go run . -workers 2 -jobs 6 -duration 500ms -timeout 100ms`.
Explain that `go run` reports the child's failing exit status through its wrapper.
Signal handling, retries, rate limiting and network jobs belong to later lessons.

### Tests and learning exercises

Use channels as test barriers instead of sleeps to prove worker bounds and
cancellation. Release barriers and cancel contexts with t.Cleanup; use timeout
guards so a broken runner fails rather than hanging the suite.

Tests must cover empty input, invalid workers/nil job, each job exactly once,
stable result association, maximum concurrency, independent failure continuation,
pre-cancellation, in-flight cancellation, and returning only after started jobs
have stopped. Use errors.Is to preserve failure identity. Exercise command
validation, help, success, cancellation and a failing output writer.

Observe a relevant test fail before implementing the behavior. Run gofmt,
`go test -count=1 ./...`, `go vet ./...`, and `go build ./...`. Run
`go test -race -count=1 ./...` if the installed Windows toolchain has the required
compiler; if unsupported, document the exact limitation and a runnable command
for a supported environment without claiming the race check passed.

Exercises: predict execution versus output order; vary worker count; explain why
each result slot has one writer; remove the WaitGroup in an isolated experiment;
make a job ignore cancellation and observe delayed shutdown with a bounded local
experiment; compare nil interfaces and typed nil. Explain which experiments are
intentionally broken and restore them before normal use.

## Project three direction

Build a database-backed task HTTP service with create/list/read/update operations
and transactional state changes. Revisit runner concepts through lifecycle and
request cancellation. Choose the database and dependencies when designing that
project, so deployment requirements inform those choices. Add authentication,
observability, profiling and deployment as separate lessons. Do not provision
services or expose endpoints during project two.

## Acceptance and scope

This design is ready for learner review. Implementation covers runner Lesson 1
only, including comments, tests and exercises. The roadmap table is a curriculum
plan, not a claim that all 20 chapters are implemented or already learned.
Later lessons need their own focused acceptance criteria.

The workspace is not a Git repository, so retain artifacts locally without
attempting a design commit or introducing Git as part of the lesson.
