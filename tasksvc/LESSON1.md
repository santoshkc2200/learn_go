# Lesson 1 — from HTTP request to persistent task

Goal: understand a complete synchronous request, not memorize a framework.
You will learn handlers, a consumer-owned interface, strict JSON, contextual SQL,
connection ownership, initialization transactions and integration testing.
Use the two-terminal commands in README before reading the implementation.

## 1. Read the domain contract first

Open `internal/tasks/tasks.go` and its test. A Task is shared data, not an HTTP
handler or database abstraction. Its JSON tags describe the wire field names;
`time.Time` supplies UTC RFC3339 output through encoding/json.

`NormalizeTitle` trims Unicode whitespace and requires 1–200 valid Unicode code
points. A Chinese character usually occupies three UTF-8 bytes but counts as
one code point. An emoji sequence or a letter plus combining mark can contain
several code points while looking like one character. We do not count grapheme
clusters or perform Unicode normalization in this lesson.

Prediction: does `strings.Repeat("界", 200)` pass? Does 201 pass? Read
`TestNormalizeTitle` and explain why byte length is the wrong title rule.
The HTTP body limit, however, measures bytes: it protects bounded memory use.

Sentinels establish meanings across packages. Use `errors.Is` rather than string
comparison, so adding context with `fmt.Errorf("...: %w", err)` does not break
the caller's classification.

## 2. Trace POST through the HTTP boundary

Read `internal/httpapi/api.go`, then `json.go`, then `api_test.go`.

`New` constructs a standard `http.Handler`. The method-aware ServeMux separates
`POST /tasks`, `GET /tasks/{id}` and `GET /tasks`. GET patterns also support HEAD
under the standard mux. Unknown paths get 404; unsupported methods get 405 and
an Allow header. Mux-generated 404/405 may be plain text; errors from our handlers
have JSON envelopes. This is not a global JSON middleware promise.

The HTTP package declares the three-method TaskStore interface it consumes.
Concrete storage satisfies it implicitly. Production HTTP code imports the
domain, not SQLite. The executable imports both and connects them. A separate
business-service layer would add little here; transactional state changes in
Lesson 2 will give it a purpose.

POST input travels through these checks:

1. Parse Content-Type with mime.ParseMediaType; require application/json.
   Parameters such as charset=utf-8 are accepted.
2. Read the entire body through MaxBytesReader, capped at 16,384 bytes.
   A valid JSON value followed by too much whitespace still produces 413.
3. Reject invalid raw UTF-8 before the decoder can replace it.
4. Decode into a pointer to a request struct, reject null/unknown fields, and
   require a second decode to return EOF. That allows whitespace, not another
   JSON value. Missing/empty/null title fails the title contract.
5. Normalize the title, call Create with `r.Context()`, return 201 and Location.

The concrete request has only title: clients cannot choose ID, status or time.
`DisallowUnknownFields` does not reject duplicate keys. For repeated string title
fields, encoding/json's later value wins. Title is a *string so a final null also
overwrites an earlier string and fails validation; plain string fields would
silently ignore null. Escaped malformed surrogates such as
`\ud800` become the replacement character U+FFFD; invalid raw UTF-8 bytes are
rejected earlier. Do not confuse this with complete Unicode validation.

Before running `TestBodyLimit`, predict exactly-16-KiB and one-byte-over outcomes.
Why would stopping after the first decoder.Decode miss the second case?

Read `writeJSON`: encode before committing headers; check the final Write result.
A disconnected client may make Write fail. Log it, but do not append another
error response to a response whose headers have already been sent.

## 3. Follow the SQL operation

Read `internal/store/sqlite.go` and `schema.sql`, then `sqlite_test.go`.

`sql.Open` creates a pool handle, not proof of a working database connection.
`PingContext` verifies availability. This lesson configures one open and one idle
connection: operations serialize through this pool. That does not eliminate
contention from another process. SQLite allows one writer at a time; its behavior
must not be generalized to all PostgreSQL isolation levels.

The driver is imported for registration via a blank import. Its pinned release
documents repeated `_pragma` URI parameters. We set foreign_keys and a 5-second
busy timeout for each newly opened connection, not just whichever connection
happened to receive one startup Exec. Pragmas are constant application settings;
the operator's filename is encoded independently. On Windows the file URI path
needs a leading slash before the drive letter. Spaces/#/? must not become query
parameters or fragments; ? is tested only in the encoder because it is not a
valid Windows filename character.

Create uses `ExecContext` with SQL placeholders. The title is a value, never a
piece of SQL syntax. LastInsertId provides the generated integer ID. The known
title, pending status and UTC timestamp form the returned Task. Timestamp text
uses RFC3339Nano; Scan parses it into time.Time. INTEGER PRIMARY KEY is enough
for this lesson: there is no delete endpoint. AUTOINCREMENT is not universally
required and has different ID-reuse semantics.

Get uses QueryRowContext; a missing row returns sql.ErrNoRows when Scan runs,
which storage maps to wrapped ErrNotFound. Other failures retain their cause.
List uses QueryContext, Scan, ORDER BY id ASC and LIMIT 100. It starts with a
non-nil empty slice so JSON becomes [] rather than null.

Rows own resources. Close them and check Rows.Err after iteration: Next returning
false can mean exhaustion or an error. With one connection, forgetting Close
can make the next operation wait for the connection you are still holding.
Request contexts propagate through database/sql, including pool acquisition.
`TestCanceledPoolWait` deliberately occupies the pool, observes WaitCount, then
proves cancellation unblocks a second query before releasing that connection.
This does not prove every external SQLite lock wait stops instantaneously.

Prediction: what happens to `Robert'); DROP TABLE tasks;--`? Read TestLiteralTitle
and explain why quoting titles yourself would be less safe than parameters.

## 4. Initialization is a transaction, not a migration system

The embedded SQL is part of the executable. Startup begins a transaction,
creates the table if absent, checks the required SELECT columns and commits.
Deferred Rollback protects failure paths and is harmless after successful Commit.

All operations during initialization use tx, not db. A transaction owns its
connection. With a one-connection pool, calling db.Query while tx owns it can
wait on yourself. A failed commit must be returned, not treated as success.

Repeated startup preserves data. An existing tasks table missing required columns
causes startup failure without a reset. This projection check is not full schema
validation: it does not compare every type, constraint, index or old row. CREATE
TABLE IF NOT EXISTS is initialization, not versioned migration management. That
comes in Lesson 2. Never automatically delete a database to hide a schema error.

Read TestRestartPersistence and TestIncompatibleSchema before experimenting with
SQL files. Use a separate experiment filename, keep your real tasks.db unchanged,
and return to the original startup command afterward.

## 5. Ownership extends to process shutdown

Read `cmd/tasksvc/run.go`, `server.go`, `main.go`, then their tests.

The executable owns the pool and listener. Store borrows the pool. Handlers borrow
the store. Startup validates flags and opens the database before listening. It
uses a bounded startup context and logs the actual listening address; port zero
lets tests request an available port safely.

Server defaults are explicit educational values: header read 5s, read/write 10s,
idle 60s, maximum headers 1 MiB. They are not universal production settings.
Shutdown gets a new 5-second context derived from Background: the signal context
is already canceled and would otherwise defeat graceful cleanup.

The helper joins its one Serve goroutine and treats ErrServerClosed as expected.
An unexpected accept error also runs bounded cleanup of already accepted requests
before returning the original cause. On shutdown deadline expiry it force-closes
connections and preserves the error.
Server.Close does not forcibly stop arbitrary handler code. An uncooperative
handler can continue after sockets close; tests explicitly release held handlers.
Later lifecycle work will deepen cooperative request cancellation/readiness.
The pool closes after serve returns, not while normal graceful requests finish.

main turns Ctrl+C into context cancellation. It calls the signal stop function
before os.Exit, because os.Exit does not execute deferred functions. Help exits
0; invalid arguments 2; startup/serve/shutdown failures 1; graceful stop 0.

## 6. Run deliberate failures

In terminal 2, each invalid request below intentionally raises a PowerShell HTTP
error. Catch displays the status rather than mistaking it for server failure:

```powershell
try {
  Invoke-RestMethod -Method Post -Uri "$base/tasks" `
    -ContentType 'application/json' -Body '{"title":"   "}'
} catch { [int]$_.Exception.Response.StatusCode } # 400

try {
  Invoke-RestMethod -Method Post -Uri "$base/tasks" `
    -ContentType 'application/json' -Body '{"title":"x","status":"done"}'
} catch { [int]$_.Exception.Response.StatusCode } # 400

try {
  Invoke-RestMethod -Method Post -Uri "$base/tasks" `
    -ContentType 'text/plain' -Body '{"title":"x"}'
} catch { [int]$_.Exception.Response.StatusCode } # 415

try { Invoke-RestMethod -Uri "$base/tasks/0" }
catch { [int]$_.Exception.Response.StatusCode } # 400

try { Invoke-RestMethod -Uri "$base/tasks/9223372036854775807" }
catch { [int]$_.Exception.Response.StatusCode } # 404 unless that ID exists

try { Invoke-RestMethod -Uri "$base/tasks?limit=10" }
catch { [int]$_.Exception.Response.StatusCode } # 400; pagination is not implemented

$body = '{"title":"x"}' + (' ' * 16372)
try {
  Invoke-RestMethod -Method Post -Uri "$base/tasks" `
    -ContentType 'application/json' -Body $body
} catch { [int]$_.Exception.Response.StatusCode } # 413: 13 + 16372 = 16385 bytes
```

Internal storage errors produce sanitized 500 JSON. The real cause goes to
server diagnostics, not the response. TestStorageFailureSanitized injects a
private error and checks both channels without damaging a real database.

## 7. What the tests prove

Run all checks from README. Focused exploration:

```powershell
go test -count=1 ./internal/tasks -v
go test -count=1 ./internal/store -v
go test -count=1 ./internal/httpapi -v
go test -count=1 ./cmd/tasksvc -v
```

HTTP unit tests use a controlled TaskStore only to inject failures and observe
context forwarding. Storage tests use actual SQLite. TestSQLiteHTTPIntegration
starts a real HTTP server and reuses one client for create/get/list; all responses
are closed. TestRunSmoke exercises executable wiring with an isolated temporary
database, a real loopback listener, cancellation and a joined shutdown. Lifecycle
tests coordinate handlers with channels instead of sending signals to your shell.

Exercise: temporarily change LIMIT 100 to LIMIT 99. Predict which test fails,
run it, then restore LIMIT 100 and rerun the suite. Do the same with the byte
limit and ErrNotFound mapping, one change at a time. Restore every change before
moving on. Do not remove tests to make a broken experiment appear successful.

## Next lesson

Lesson 2 will atomically update task state and audit history, then add conflict,
pagination, idempotency and migration contracts. None of those are implemented
here. You now have the HTTP/storage boundary on which to build them.

Primary references: [Go database guide](https://go.dev/doc/database/),
[Go transactions](https://go.dev/doc/database/execute-transactions),
[pinned SQLite driver](https://pkg.go.dev/modernc.org/sqlite@v1.60.1),
[SQLite isolation](https://www.sqlite.org/isolation.html).
