# Execution ledger — tasksvc Lesson 1

Plan: `2026-10-09-tasksvc-lesson-1.md`; design and plan approved for native execution.

- Pre-flight: Task model is shared by storage/HTTP; Create/Get/List signatures
  match the consumer interface. Executable owns DB and listener lifecycle.
- Ruling: retain this local ledger instead of Git-based task scripts/worktrees —
  approved plan accounts for unborn master and unrelated untracked learner files;
  cost if wrong: less automated history, mitigated by preserved source/evidence.
- Task 1: complete; NormalizeTitle tests RED (missing symbols) -> GREEN.
- Driver: modernc.org/sqlite v1.60.1, published 2026-09-29, requires Go 1.26.0;
  compatible with installed 1.27.0. Read pinned driver.go DSN documentation:
  repeated _pragma values run for each new connection. Only constant pragmas
  enter configuration; operator filenames are URL-encoded separately.
- Task 2: first real run exposed Windows drive-letter URI authority; added
  leading slash to the URI path. Path tests reproduce the original failure.
- Task 2: complete; 12 storage tests GREEN; go mod tidy passed after authorized
  dependency downloads. SQLite is the only direct third-party dependency.
- Task 3: input/routing/diagnostic tests GREEN; sandbox blocks loopback sockets,
  so real HTTP integration verification runs with approved local networking.
- Task 3: complete; 38 HTTP cases GREEN with local networking, including real
  SQLite HTTP integration and exact byte boundary.
- Task 4: complete; 22 executable cases GREEN with local networking, including
  run smoke, graceful/deadline shutdown, pre-canceled start and Serve errors.
- Task 5: complete; teaching guides, independent review, regression fixes and
  final verification finished. Files remain local; no commit, push or merge.
- Verification: go test -count=1 ./... -> 78 passing cases in 4 packages;
  go vet ./... -> exit 0; go build ./... -> exit 0. gofmt applied.
- Race attempt: go test -race -count=1 ./... -> cannot execute: "-race requires
  cgo". CGO_ENABLED=0, GOOS=windows, GOARCH=amd64. No compiler installed/changed.
- Preservation checks: streamgrep suite -> 95 cases GREEN; jobrunner suite ->
  58 cases GREEN. No edits to either project; no real learner database created.
- Final review dispatched read-only to independent reviewer Mencius.
- Final review: independent suite 78 GREEN. Important: cleanup active connections
  after unexpected Serve failure. Re-graded duplicate-title-then-null as Important
  because accepting invalid input contradicts the advertised validation contract.
  Regression tests written before fixes.
- Final: minor (deferred): original lifecycle test failure cleanups should join
  all goroutines; incompatible-schema setup should immediately register DB Close.
- Final: Ruling: retain projection-only schema compatibility check — documented
  initialization scope, full migrations are Lesson 2; cost if wrong: incompatible
  constraints/types can escape startup detection.
- Final: Ruling: promise pool-wait cancellation, not instantaneous external SQLite
  lock cancellation — engine/driver lock waits differ; cost if wrong: callers may
  underestimate time spent waiting on another process (busy timeout bounds it).
- Final: Ruling: leave authentication/pagination/observability/deployment for later
  lessons — approved Lesson 1 scope; cost if wrong: not suitable for public service.
- Final: fixed unexpected Serve failure cleanup —
  TestServeUnexpectedErrorWithActiveRequest/graceful and /deadline RED -> GREEN;
  original accept cause retained, active requests drain or sockets force-close.
- Final: fixed duplicate-title-then-null — TestCreateInput/duplicate_then_null
  RED -> GREEN; null-then-string remains valid, final-value semantics preserved.
- Final verification after fixes: go test -count=1 -timeout=45s ./... -> 83 cases
  GREEN in 4 packages; go vet ./... and go build ./... -> exit 0. gofmt applied.
- Final acceptance: Lesson 1 implemented; five later lessons remain planned.
  No essential TODO, no learner DB/runtime process left by tests. Minor test
  failure-path cleanup finding above remains deferred, not a production defect.
