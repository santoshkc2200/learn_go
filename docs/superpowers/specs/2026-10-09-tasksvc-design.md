# Tasksvc: project three, database-backed HTTP service

## Purpose and learning format

Continue the learner's completed streaming CLI and five-lesson job runner with
a local task HTTP API. Teach standard Go HTTP, database programming and production
concerns through commented source, real failure tests, runnable PowerShell examples
and small prediction exercises. The learner already knows basic Go syntax.

“Start another project” means start project three's first lesson. The full project
has six main lessons. Only Lesson 1 is implemented in the initial session;
later lessons extend the same service with independently testable changes.

Use a task tracker rather than executing arbitrary jobs through HTTP. Tasks have
a title and a status. This reuses familiar terminology while keeping HTTP/storage
learning independent of scheduler integration and external side effects.

## Approaches and recommendation

1. **SQLite with database/sql and modernc.org/sqlite (recommended).** A local
   file persists data across restarts. The driver is CGO-free, fitting the current
   Windows Go installation. Explain connection pooling and SQLite limitations.
2. **PostgreSQL from the beginning.** Good for server database isolation and
   production concurrency, but adds server/container setup before the first API.
   Use as a later comparison or migration exercise rather than assuming SQLite
   and PostgreSQL share lock/isolation behavior.
3. **In-memory fake storage first.** Fast for HTTP-only tests, but does not satisfy
   the intended database-backed project. Use a fake at the HTTP test boundary and
   real SQLite in storage integration tests.

Use standard net/http, encoding/json, database/sql and testing. The SQLite driver
is the only direct third-party dependency in Lesson 1; it has transitive module
dependencies. Resolve a compatible released tag at implementation time and pin
it in go.mod/go.sum. Do not use an ORM or web framework.

## Six-lesson sequence

| Lesson | Working increment | Main roadmap chapters |
| --- | --- | --- |
| 1. HTTP meets persistent storage | Create/read/list API, strict JSON, parameterized SQL, schema initialization, request contexts and integration tests. | 6, 7, 12, 15, 16, 17 |
| 2. Atomic state changes | Status update plus audit history in one transaction, rollback tests, optimistic version conflicts, keyset pagination, idempotent creation and migration tracking. | 4, 10, 12, 16, 18 |
| 3. Secure access | Authentication, task ownership/authorization, secret configuration, request limits, secure token handling and dependency vulnerability checks. | 7, 15, 17, 18 |
| 4. Operate and shut down | Structured request logs, request IDs, metrics, trace propagation, health/readiness and graceful lifecycle tests. | 8–11, 15, 17, 18 |
| 5. Profile representative load | Reused HTTP client, realistic storage/HTTP benchmarks, CPU/heap/lock profiles, query plans, pool statistics, GC/escape/memory-limit experiments. | 12–16, 18 |
| 6. Build and deliver | CI, build metadata/tags, cross-compilation, container with persistent storage, configuration, TLS/deployment notes, reproducibility and local rollout verification. | 6, 15, 18, 19 |

Specialist Chapter 20 topics and advanced language companion labs remain optional
isolated studies from the shared curriculum. Do not claim that completing these
six lessons automatically exercises every reflection/unsafe/cgo topic.

## Lesson 1 behavior

Create an independent module `example.com/tasksvc` under `tasksvc/` with
`go 1.27.0`, matching the installed toolchain. Bind to `127.0.0.1:8080` by default;
use `-addr` to select a different loopback address/port. Reject non-loopback hosts
in this unauthenticated introductory lesson. Use `-db tasks.db` for persistent
storage; database paths belong to the operator and are never accepted from HTTP.

The service supports:

| Request | Success | Failure behavior |
| --- | --- | --- |
| POST /tasks with {"title":"learn transactions"} | 201, task JSON and Location header /tasks/{id}. | 400 malformed/invalid JSON or title; 413 body too large; 415 unsupported content type; 500 storage error. |
| GET /tasks/{id} | 200 task JSON. | 400 invalid ID; 404 missing task; 500 storage error. |
| GET /tasks | 200 JSON object {"tasks":[...]}, up to 100 rows in ascending ID order. | 400 unsupported query parameters; 500 storage error. |

Method-aware ServeMux routing produces 405 for unsupported methods and 404 for
unknown paths. Full JSON envelopes apply to implemented handler errors; automatic
mux 404/405 responses may retain standard net/http text formatting in Lesson 1.
Document this boundary rather than promising a global middleware contract.

List is intentionally capped at 100. Pagination arrives in Lesson 2; Lesson 1
documents this limit and does not pretend to return an unlimited collection.
An empty list serializes as [] rather than null. Do not launch a background worker
or import jobrunner to perform these synchronous database operations.

### Input and JSON contracts

POST accepts Content-Type application/json, optionally with parameters parsed by
mime.ParseMediaType. Limit request bodies to 16 KiB with http.MaxBytesReader.
Reject unknown fields, empty input, null top-level values, trailing values and
incorrect field types. Use a pointer to a concrete request struct to distinguish
null from a populated object. DisallowUnknownFields does not reject duplicate
JSON keys; Lesson 1 follows encoding/json's final-value behavior and documents it.

Normalize title with strings.TrimSpace, require valid UTF-8 and 1–200 Unicode
code points after normalization. Detect invalid raw UTF-8 before JSON decoding
because encoding/json replaces invalid byte sequences. Escaped malformed Unicode
surrogates follow encoding/json replacement behavior; explain that distinction
instead of claiming complete Unicode normalization. Preserve apostrophes and
other ordinary text through SQL parameters; titles are never SQL fragments.

Task JSON fields: id (positive int64), title (string), status ("pending"),
created_at (UTC RFC3339 timestamp). Clients cannot supply ID, status or timestamps.
Parse IDs as positive base-10 integers fitting int64. Validate query parameters;
GET /tasks has no supported query parameters in Lesson 1.

### Persistent schema and cleanup

SQLite table tasks has integer primary key, non-null title, status constrained
to pending/running/done and a non-null creation timestamp. New tasks start pending;
status modification belongs to Lesson 2. AUTOINCREMENT is unnecessary for this
lesson because no delete endpoint exists; do not teach it as universally required.

Keep the initialization SQL in an embedded schema.sql asset and apply it at startup
inside a transaction, making repeated startup safe. CREATE TABLE IF NOT EXISTS is
initialization, not a general migration strategy; versioned migrations follow in
Lesson 2. Fail startup on incompatible schema/query errors rather than deleting
or replacing an existing database. Never automatically reset a user's DB.

sql.Open creates a pool handle; PingContext verifies availability. Set maximum
open/idle connections to one for an explicit first-lesson SQLite policy, and
configure foreign keys and a bounded busy timeout on every new connection through
the driver's documented DSN pragmas. This serializes operations through this pool;
it does not remove external database lock contention. Avoid claiming SQLite
transactions demonstrate all PostgreSQL isolation levels.

Parameterized ExecContext inserts tasks and LastInsertId retrieves the generated
ID; QueryRowContext reads one task; QueryContext plus Scan reads lists. Close Rows,
check Rows.Err, and distinguish sql.ErrNoRows from failed queries. Pass request
contexts into every storage call, including waiting for a pooled connection.
The caller owns sql.DB and closes it after HTTP work is stopped.

### Package boundaries

- `cmd/tasksvc/main.go`: flags, signal context, open database, construct handlers,
  configure http.Server and process exit. Keep errors returned until this boundary.
- `internal/tasks/tasks.go`: Task model and ErrNotFound/validation contracts.
- `internal/store/sqlite.go`, `schema.sql`: sql.DB lifecycle, embedded schema and
  concrete task storage methods. Storage imports the model, not HTTP.
- `internal/httpapi/api.go`, `json.go`: consumer-owned small storage interface,
  constructor injection, handlers and JSON helpers. HTTP imports model, not a
  database driver. Place error-to-status translation at this boundary.
- Matching *_test.go files, README.md and LESSON1.md explain behavior and tests.

Use a small interface at the HTTP consumer for Create/List/Get methods taking
context.Context. A fake can inject a storage failure in HTTP tests; SQLite tests
verify actual persistence and constraints. Add a separate business-service layer
only once Lesson 2's transactional state-change rules need it.

### Server lifecycle

Set ReadHeaderTimeout=5s, ReadTimeout=10s, WriteTimeout=10s, IdleTimeout=60s and
MaxHeaderBytes=1 MiB. These are explicit educational defaults, not universal values.
Use signal.NotifyContext(os.Interrupt); graceful Shutdown has a separate five-second
context from context.Background, since the signal context is already canceled.
Join the serve loop, handle http.ErrServerClosed as expected and close the DB last.
If graceful shutdown expires, close the server and report the failure. Later
lifecycle lessons deepen request cancellation and readiness.

No authentication, public hosting or container execution is included now. This
introductory server is a loopback learning service. Do not publish or expose it
externally before implementing the appropriate deployment/security lesson.

## Lesson 1 tests and acceptance

- Observe failing behavior tests before each implementation slice; gofmt,
  go test -count=1 ./..., go vet ./... and go build ./... must pass.
- Storage tests use an isolated SQLite file under t.TempDir, not the user's DB.
  Cover create/get/list, empty list, missing IDs, literal SQL-looking titles,
  cancellation, capped lists, repeated initialization, close/reopen persistence
  and startup failure for an incompatible schema. Tests close Rows before another
  operation so the one-connection pool cannot self-deadlock.
- HTTP tests use httptest and controlled stores. Verify status, headers, decoded
  JSON fields, invalid IDs, title boundaries with Unicode, null/unknown/trailing
  JSON, malformed content type, malformed UTF-8, body limit and internal error
  sanitization. Storage error details go to diagnostics, not response JSON.
- At least one httptest.Server test uses real SQLite and a reusable http.Client
  to create, list and retrieve a task across the HTTP/storage boundary.
- Exercise signal/shutdown helpers with controlled listeners/contexts rather than
  sending Ctrl+C to the user's active terminal. Use guards to prevent hanging tests.
- Start a temporary loopback server on an available port for a real smoke check,
  then stop/join it. Do not leave a background server running after verification.
- Record exact race-check support; current CGO-disabled setup cannot run -race,
  even though the SQLite driver itself does not need CGO.
- Guides explain source reading order, PowerShell Invoke-RestMethod commands,
  restart persistence, error contracts, cleanup and experiments. Do not leave
  essential code as TODOs or claim future lessons have been implemented.

## Environment and change boundaries

The workspace is now a Git repository on an unborn master branch. Existing docs,
jobrunner and streamgrep are untracked learner files. Preserve them; do not stage
or commit unrelated projects, initialize a second repository or create a worktree
from a nonexistent commit. Retain the new design locally for review. Establish
Git integration only when it fits the actual repository state and user intent.

## Documentation used to validate choices

- [SQLite Go driver](https://pkg.go.dev/modernc.org/sqlite): CGO-free driver and
  driver-specific configuration; read the pinned release's docs before coding.
- [Go database guide](https://go.dev/doc/database/): database/sql operations,
  connection handling and contexts.
- [Go transactions guide](https://go.dev/doc/database/execute-transactions):
  use sql.Tx methods consistently and check commit/rollback behavior.
- [SQLite isolation](https://www.sqlite.org/isolation.html): single-writer behavior
  and engine-specific isolation, to be contrasted with server databases later.

## Design review result

The first lesson is one coherent local HTTP/persistence slice. Later security,
observability, transaction rules and deployment remain separate increments.
Schema initialization is distinguished from migrations; JSON replacement behavior
and bounded list behavior are explicit. No external infrastructure is required.
