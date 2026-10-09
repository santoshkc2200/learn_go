# Tasksvc Lesson 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (preserved native execution) to implement task-by-task, followed by one independent review. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a commented local HTTP API that creates, reads and lists tasks persisted in SQLite, with real integration tests and a first-lesson walkthrough.

**Architecture:** A domain model is shared by a concrete database/sql store and a standard-library HTTP consumer. HTTP depends on a small store interface, while the executable owns the database, listener and server lifecycle. No separate business layer or scheduler is needed for this first slice.

**Tech Stack:** Go 1.27.0, net/http, database/sql, encoding/json, log/slog, modernc.org/sqlite and testing/httptest.

**Spec:** `docs/superpowers/specs/2026-10-09-tasksvc-design.md` (approved).

## Global Constraints

- Independent module `example.com/tasksvc` under `tasksvc/` with `go 1.27.0`.
- The SQLite driver is the only direct third-party dependency in Lesson 1; resolve a compatible released tag and pin it in go.mod/go.sum. Do not use an ORM or web framework.
- Bind to `127.0.0.1:8080` by default; reject non-loopback hosts. Default database path is `tasks.db`.
- Implement only POST /tasks, GET /tasks/{id}, GET /tasks. Lists return up to 100 rows in ascending ID order; empty lists serialize as [].
- Request bodies are limited to 16 KiB. Titles normalize with strings.TrimSpace and contain 1–200 Unicode code points.
- Set maximum open/idle connections to one; configure foreign keys and a bounded busy timeout on every new connection. Use parameterized context-aware queries.
- Initialization runs embedded schema.sql inside a transaction. Do not reset existing databases. Initialization is not a versioned migration system.
- Server defaults: ReadHeaderTimeout=5s, ReadTimeout=10s, WriteTimeout=10s, IdleTimeout=60s, MaxHeaderBytes=1 MiB. Graceful shutdown has a separate five-second context; DB closes last.
- Preserve streamgrep, jobrunner and unrelated untracked files. The repository is on an unborn master branch: do not create a worktree from a nonexistent commit or commit unrelated projects.
- No public hosting, authentication, containers or implementation of subsequent lessons in this session.

## Review Focus

1. Windows database paths containing spaces, # or ?: construct the SQLite DSN without treating path characters as query configuration.
2. A pooled connection is occupied: request cancellation must unblock a second operation waiting for that connection.
3. Exactly 16 KiB versus 16 KiB + 1 bytes: enforce the body limit even when a valid first JSON value has oversized trailing whitespace.
4. HTTP client disconnect/write failure: finish handler cleanup, report output failures diagnostically and never append a second error response after headers are committed.
5. Cancellation before startup and shutdown deadline expiry: release the listener, join the serve loop and preserve errors rather than leaking resources.

Each condition has a named test in its owning task below.

## Task 1 — Domain contracts

**Files:** Create `tasksvc/go.mod`, `tasksvc/internal/tasks/tasks.go`, `tasksvc/internal/tasks/tasks_test.go`.

**Interfaces:**

- `type Task struct { ID int64; Title string; Status string; CreatedAt time.Time }`, JSON tags `id`, `title`, `status`, `created_at`.
- `var ErrNotFound = errors.New("task not found")`.
- `var ErrInvalidTitle = errors.New("invalid task title")`.
- `func NormalizeTitle(title string) (string, error)` trims, rejects invalid UTF-8 and enforces 1–200 code points.

- [x] Write `TestNormalizeTitle` with independently specified cases, including:

  ```go
  // Each row's error expectation is checked with errors.Is.
  {input: "  learn SQL\t", want: "learn SQL", wantErr: nil}
  {input: " \n\t", want: "", wantErr: ErrInvalidTitle}
  {input: strings.Repeat("界", 200), want: strings.Repeat("界", 200), wantErr: nil}
  {input: strings.Repeat("界", 201), want: "", wantErr: ErrInvalidTitle}
  {input: string([]byte{0xff}), want: "", wantErr: ErrInvalidTitle}
  ```

- [x] Create the module and run `go test ./internal/tasks -v`; expect missing NormalizeTitle/ErrInvalidTitle symbols before implementation.
- [x] Implement the model, sentinels and NormalizeTitle in tasks.go. Count runes after verifying UTF-8; no Unicode normalization library.
- [x] Run `go test -count=1 ./internal/tasks -v`; expect PASS. Preserve local execution evidence instead of a Git commit in this unborn repository.

## Task 2 — SQLite persistence and initialization

**Files:** Create `tasksvc/internal/store/sqlite.go`, `tasksvc/internal/store/schema.sql`, `tasksvc/internal/store/sqlite_test.go`; update `tasksvc/go.mod`, create `tasksvc/go.sum` through Go tooling.

**Consumes:** Task and domain sentinels from Task 1.

**Produces:**

- `func Open(ctx context.Context, path string) (*sql.DB, error)` configures/pings the pool, initializes the schema, and closes the pool on startup failure. Successful caller owns Close.
- `type Store struct { db *sql.DB }`; `func New(db *sql.DB) *Store` borrows the DB.
- `func (s *Store) Create(ctx context.Context, title string) (tasks.Task, error)`.
- `func (s *Store) Get(ctx context.Context, id int64) (tasks.Task, error)`.
- `func (s *Store) List(ctx context.Context) ([]tasks.Task, error)`.

- [x] Resolve driver release metadata with `go list -m -json modernc.org/sqlite@latest`; inspect its Go version and published module. Pin a compatible release using `go get modernc.org/sqlite@<resolved-tag>`, and read that release's package docs for DSN pragmas before implementing them. Record exact tag in the progress file; do not infer compatibility from the CGO-free label alone.
- [x] Write tests before storage implementation: `TestCreateGetList` checks positive ID, normalized title, pending status and nonzero UTC CreatedAt; `TestEmptyList` requires a non-nil empty slice; `TestNotFound` uses errors.Is; `TestLiteralTitle` inserts `Robert'); DROP TABLE tasks;--` and proves the table still works.
- [x] Run `go test ./internal/store -v`; expect missing Open/New/storage methods. Implement embedded schema initialization using BeginTx/tx.ExecContext/tx.QueryContext/Commit with deferred Rollback. Schema: tasks(id INTEGER PRIMARY KEY, title TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('pending','running','done')), created_at TEXT NOT NULL). Verify required query columns at startup before committing. Do not call db methods while the single connection is held by tx.
- [x] Construct a file URI from an absolute filepath with proper URL encoding, then add `_pragma=foreign_keys(1)` and `_pragma=busy_timeout(5000)` as query values after confirming the pinned driver's contract. Set MaxOpenConns(1), MaxIdleConns(1), PingContext. Use UTC RFC3339Nano internally for created_at; encoding/json encodes time.Time in RFC3339 form.
- [x] Implement Create with NormalizeTitle, ExecContext and LastInsertId; return the already known timestamp/title/status. Implement Get with QueryRowContext/Scan and wrapped ErrNotFound; List uses ORDER BY id ASC LIMIT 100, closes Rows and checks Rows.Err. Preserve underlying cancellation/storage error identities through %w.
- [x] Add `TestRestartPersistence`, `TestRepeatedInitialization`, `TestIncompatibleSchema` (precreate tasks with only id; startup must fail without deleting it), `TestListLimit` (105 rows => first 100 IDs ascending) and `TestDatabasePathCharacters` (temp directory with spaces/#; construct paths with filepath.Join).
- [x] Add `TestCanceledQuery` for a pre-canceled context and `TestCanceledPoolWait`: hold db.Conn on the single pool connection, start Get with a cancellable context, cancel and require return before releasing the held connection. Register cleanup and timeout guards. Test the DSN path encoder separately with a ? character because Windows filenames cannot contain ?.
- [x] Run `go test -count=1 ./internal/store -v`; expect PASS. Run `go mod tidy` and inspect that only the SQLite driver is a direct third-party dependency.

## Task 3 — HTTP handlers, limits and error mapping

**Files:** Create `tasksvc/internal/httpapi/api.go`, `json.go`, `api_test.go`, `integration_test.go`.

**Consumes:** Domain model/sentinels; production Store conforms implicitly to:

```go
type TaskStore interface {
    Create(context.Context, string) (tasks.Task, error)
    Get(context.Context, int64) (tasks.Task, error)
    List(context.Context) ([]tasks.Task, error)
}
```

**Produces:** `func New(store TaskStore, logger *slog.Logger) http.Handler`; nil logger uses slog.Default. Handler-owned JSON errors encode `{"error":"<public message>"}`; storage details are logged and response message is `internal server error`.

- [x] Write `TestCreateTask`, `TestGetTask`, `TestListTasks`: decode JSON instead of pinning full formatting, assert status and application/json, and require Location /tasks/{id} for 201. Empty fake-store lists must appear as [] rather than null. Use a controlled store for exact context propagation and failure injection, not a fake database driver.
- [x] Run `go test ./internal/httpapi -v`; expect missing New/interface implementation. Implement method-aware routes `POST /tasks`, `GET /tasks/{id}`, `GET /tasks`. Reject unsupported query parameters for GET /tasks; parse positive int64 IDs. Map ErrNotFound to 404, ErrInvalidTitle to 400 and storage failure to sanitized 500.
- [x] Write `TestCreateInput` with Content-Type absent/wrong/malformed =>415; application/json; charset=utf-8 =>accepted; empty/null/unknown fields/trailing values/incorrect types =>400; normalized title bounds use Task 1 contracts. Include duplicate title keys to document encoding/json's final-value semantics and an escaped malformed surrogate to document replacement behavior.
- [x] Add `TestBodyLimit`: a valid body padded with whitespace to exactly 16384 bytes is accepted, 16385 bytes =>413. Add invalid raw UTF-8 =>400. Run focused tests to observe failure before implementing bounded input decoding.
- [x] Implement request decoding with MaxBytesReader then io.ReadAll (bounded to 16 KiB), detect raw invalid UTF-8, Decode into `*createRequest` with DisallowUnknownFields, reject nil, and require a second Decode to return io.EOF. Parse media type using mime.ParseMediaType. Use errors.As for MaxBytesError to select 413.
- [x] Add `TestRouting` for 404/405 plus Allow behavior; `TestInvalidID` for 0, negative, malformed and overflow; `TestUnsupportedQuery`; `TestStorageFailureSanitized` checks the private sentinel appears in captured logs but not response JSON.
- [x] Add `TestResponseWriteFailure` using a controlled ResponseWriter returning an error: log it and do not attempt a second response. Implement checked JSON writing that encodes before committing headers, includes Content-Type and appropriate status, and logs write errors. Do not retry writes after headers are committed.
- [x] Add `TestSQLiteHTTPIntegration` using t.TempDir, Task 2 Open/New, httptest.Server and one reused server.Client; POST, GET and list must agree. Set client timeout and close every response body. Also test GET after reopen via store integration to establish persistence rather than HTTP-only fake behavior.
- [x] Run `go test -count=1 ./internal/httpapi -v`; expect PASS.

## Task 4 — Executable and owned server lifecycle

**Files:** Create `tasksvc/cmd/tasksvc/main.go`, `run.go`, `run_test.go`, `server.go`, `server_test.go`.

**Consumes:** Store Open/New and HTTP New.

**Produces:**

- `type config struct { Addr string; DBPath string }`.
- `func parseConfig(args []string, output io.Writer) (config, error)`; defaults 127.0.0.1:8080/tasks.db, recognizes flag.ErrHelp. Require a literal loopback IP and a numeric port 0–65535; port 0 permits tests to allocate safely. Reject hostnames, empty/wildcard/non-loopback hosts, invalid ports, empty DB path and positional arguments.
- `func serve(ctx context.Context, server *http.Server, listener net.Listener, shutdownTimeout time.Duration) error` owns the listener/serve goroutine; uses fresh background shutdown context and force-close on timeout. Caller closes DB afterward.
- `func run(ctx context.Context, args []string, output io.Writer, logger *slog.Logger) int` returns 0 on help/graceful stop, 2 on bad arguments, 1 on startup/serve/shutdown failures.

- [x] Write config/run tests: defaults, literal IPv4/IPv6 loopback, port 0, invalid hosts/ports, help, bad flags, positional arguments, missing DB path. A pre-canceled parent must exit without opening a DB or listener. Run `go test ./cmd/tasksvc -v`; expect missing helpers.
- [x] Implement validated flags; open DB with a bounded startup context, construct HTTP handlers, listen only after DB initialization, log actual bound address (including allocated test port). Install exact server defaults from Global Constraints. Defer DB.Close after serve returns; join before process exit. main wires signal.NotifyContext and stops signal handling before os.Exit.
- [x] Write `TestServeGracefulShutdown`: start on loopback port 0, use a request barrier to hold a handler, cancel parent and prove serve does not return while that handler is active; release it and require clean return. The independent shutdown context must permit cleanup after parent cancellation.
- [x] Write `TestServeShutdownDeadline`: hold handler cleanup past a short shutdown budget; require context.DeadlineExceeded identity and forced connection close, then release test-owned handler cleanup. Write `TestServeAlreadyCanceled` to ensure listener closes and serve goroutine is joined. Write a broken listener test to retain unexpected Serve errors.
- [x] Run lifecycle tests RED before implementing serve; implement a buffered result channel for the one serve goroutine, expected ErrServerClosed handling, Shutdown and Close failure propagation. Do not assume Server.Close forcibly joins arbitrary uncooperative handler code; document its limit and release controlled test handlers explicitly.
- [x] Run `go test -count=1 ./cmd/tasksvc -v`; expect PASS. A smoke test must start a temporary loopback instance, create/get/list using HTTP and stop/join it; use an isolated DB, never the learner's tasks.db. Exercise the executable's run wiring without sending signals to the user's terminal or leaving background processes running.

## Task 5 — Teaching guide, review and final proof

**Files:** Create `tasksvc/README.md`, `tasksvc/LESSON1.md`, `docs/superpowers/plans/2026-10-09-tasksvc-lesson-1-progress.md`.

- [x] Write six-lesson index plus first-lesson reading order, commands and expected outcomes. Include `go run ./cmd/tasksvc -db tasks.db`, PowerShell Invoke-RestMethod POST/GET/list, Ctrl+C, restart/retrieve instructions, and negative validation exercises. Specify two terminals and intentional error statuses.
- [x] Explain standard handlers, method-aware mux, interface placement, dependency direction, rune/byte limits, JSON duplicate/replacement behavior, DB pool versus connection, sql.Open versus Ping, context propagation, Rows.Close/Err, tx-owned connection, schema initialization versus migration, SQLite limits and loopback scope. Include exercise predictions and restore instructions.
- [x] Run gofmt, `go test -count=1 ./...`, `go vet ./...`, `go build ./...` in tasksvc; expect all to pass. Attempt `go test -race -count=1 ./...` and record actual result rather than claiming support. No C compiler installation is part of Lesson 1.
- [x] Request one independent final review of API contracts, SQLite cleanup/pool behavior, Unicode/body bounds and lifecycle. Fix actionable findings with regression proof, rerun affected checks and the full suite. Review JSON response bodies for accidental private error exposure.
- [x] Inspect saved guides and module files, check no essential TODO remains and remove only artifacts generated by this task. Record driver version, tests, smoke result, race limitation and decisions in progress. Do not stage unrelated learner projects or delete temporary evidence before it has a durable local record.
- [x] Deliver guide/source/test links and runnable commands. Claim only Lesson 1 complete; five later lessons remain designed increments.

## Self-review and execution handoff

The five tasks cover every first-lesson contract, including the five review cases.
Store method signatures match the HTTP consumer exactly; the executable owns
resource closure. Positive title limits count code points, body limits count bytes.
Timestamp representation, list cap, cancellation behavior and automatic mux error
formatting are explicit. Driver release resolution is a verifiable implementation
step rather than a guessed version. Existing projects are untouched.

Preserve native execution chosen earlier in the conversation: implement in this
chat, then obtain one independent review. The learner reviews this saved plan
before implementation; no new choice of execution method is necessary.
