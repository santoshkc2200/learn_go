# tasksvc — project three

A local, database-backed task API for learning intermediate and advanced Go.
The streaming CLI teaches I/O and package design; jobrunner teaches concurrency;
this service connects HTTP, persistence and operational concerns.

## Six lessons

| Lesson | Increment | State |
| --- | --- | --- |
| 1 | HTTP + SQLite: create/get/list, validation, contexts, integration tests | Implemented |
| 2 | Atomic state changes, audit history, optimistic conflicts, pagination, idempotency, migrations | Planned |
| 3 | Authentication, ownership, secret configuration and security checks | Planned |
| 4 | Request logs/IDs, metrics, tracing, health/readiness and lifecycle | Planned |
| 5 | Representative load, benchmarks, query plans and CPU/heap/lock profiling | Planned |
| 6 | CI, reproducible builds, cross-compilation, containers and deployment | Planned |

Start with [Lesson 1](LESSON1.md). Later rows are learning increments, not
features that already exist. Specialist reflection/unsafe/cgo labs remain
optional; three applications do not automatically cover every Go topic.

## Run

Requires Go 1.27 or newer. The pinned SQLite driver is CGO-free, with transitive
dependencies; ordinary builds need no C compiler. First setup needs network
access to download modules.

In PowerShell terminal 1:

```powershell
cd C:\Users\user\personal\dev\go\tasksvc
go run ./cmd/tasksvc -db tasks.db
```

On macOS (zsh or bash), adjust the path to your checkout location:

```sh
cd ~/personal/dev/go/tasksvc
go run ./cmd/tasksvc -db tasks.db
```

In terminal 2:

```powershell
$base = 'http://127.0.0.1:8080'
$task = Invoke-RestMethod -Method Post -Uri "$base/tasks" `
  -ContentType 'application/json' -Body '{"title":"learn database/sql"}'
$task
Invoke-RestMethod -Uri "$base/tasks/$($task.id)"
Invoke-RestMethod -Uri "$base/tasks"
```

On macOS (zsh or bash):

```sh
base='http://127.0.0.1:8080'
task=$(curl -sS -X POST "$base/tasks" \
  -H 'Content-Type: application/json' -d '{"title":"learn database/sql"}')
printf '%s\n' "$task"
printf 'Enter the numeric id from the POST response: '
read -r task_id
curl -sS "$base/tasks/$task_id"
curl -sS "$base/tasks"
```

POST returns 201 with a generated ID, `pending` status and UTC timestamp. GET
returns 200. List returns `{"tasks":[...]}` with at most the first 100 tasks,
ordered by ID; an empty database returns `{"tasks":[]}`.

Press Ctrl+C in terminal 1, restart with the same command, and repeat GET in
terminal 2 using the saved `$task.id` (PowerShell) or `$task_id` (macOS). The row survives. The database path is
relative to the server's working directory, not the client terminal.

To use another loopback port/file:

In PowerShell or macOS Terminal (zsh or bash):

```sh
go run ./cmd/tasksvc -addr 127.0.0.1:8081 -db lesson1-experiment.db
```

Only literal loopback addresses are accepted. There is no authentication yet:
loopback limits exposure but does not establish user identity. Do not publicly
expose this introductory service. No jobs execute and no status-update/delete
endpoint exists in Lesson 1.

## Verify

In PowerShell or macOS Terminal (zsh or bash):

```sh
go test -count=1 ./...
go vet ./...
go build ./...
go test -race -count=1 ./...
```

Tests create temporary databases and stop their temporary HTTP servers. They do
not modify `tasks.db`. Race detection is separate from the driver's CGO-free
build: this machine has CGO disabled, so the last command cannot currently run.
On a supported Go environment with CGO and a suitable C compiler, run it again;
an ordinary passing test suite is not a race-check result.

Driver: `modernc.org/sqlite v1.60.1` (requires Go 1.26; this module uses Go 1.27).
Only this driver is a direct third-party dependency. `go.sum` records module
checksums; it is not a vulnerability report.
