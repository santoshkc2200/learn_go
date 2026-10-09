# Lesson 5 — prove lifetimes and measure bottlenecks

Roadmap chapters 9, 12, 13 and 14. Read `internal/runner/benchmark_test.go`,
`diagnostics_test.go`, `fuzz_test.go` and [measurement notes](measurements/README.md).

## Correctness before timing

```powershell
Set-Location C:\Users\user\personal\dev\go\jobrunner
go test -count=1 ./...
go vet ./...
go build ./...
go test -race -count=1 ./...
go test ./internal/runner -run 'TestStream|TestRepeatedCancellation' -count=20
go test ./internal/runner -run '^$' -fuzz FuzzOptions -fuzztime 5s
```

On macOS (zsh or bash), adjust the path to your checkout location:

```sh
cd ~/personal/dev/go/jobrunner
go test -count=1 ./...
go vet ./...
go build ./...
go test -race -count=1 ./...
go test ./internal/runner -run 'TestStream|TestRepeatedCancellation' -count=20
go test ./internal/runner -run '^$' -fuzz FuzzOptions -fuzztime 5s
```

The race command was attempted here and failed because CGO is disabled. Run it
with a supported target, CGO enabled and a compatible C compiler. No race pass
is claimed. Passing the detector covers executed paths, not every possible input.

Repeated cancellation accounts for owned children with explicit start/stop/join
handshakes. Global runtime.NumGoroutine can change because of testing/runtime
activity and is not a reliable exact leak assertion. A goroutine profile or trace
can help investigate a suspicious count, but owned lifetimes still need contracts.

The fuzzer limits jobs, workers and durations so an input cannot intentionally
consume unbounded time or memory. Seeds run in normal go test; -fuzz searches
additional inputs. A positive tiny deadline may cancel a valid job depending on
scheduling, so the property tests identities/shape rather than universal success.

## Define the benchmark's operation

In PowerShell or macOS Terminal (zsh or bash):

```sh
go test ./internal/runner -run '^$' -bench '^BenchmarkBatch$' -benchmem -benchtime 1s -count 5
go test ./internal/runner -run '^$' -bench '^BenchmarkWaitingBatch$' -benchmem -benchtime 1s -count 5
```

One Batch operation is 64 jobs through dispatch, execution, result allocation and
join. No-op jobs isolate much of scheduling overhead. CPU jobs hash a shared
immutable 4 KiB payload into independent output slots. Fixtures and final output
checks are outside b.Loop; each iteration executes a fresh Run lifecycle. Waiting
Batch runs 16 one-millisecond timer jobs with four workers, exposing blocked time.
These workloads do not benchmark CLI rendering, disk/network I/O or process startup.

ns/op measures the entire batch. B/op and allocs/op are allocations per batch,
not retained heap or peak memory. Worker count and GOMAXPROCS are different:
waiting jobs can overlap without extra CPU parallelism. Tiny jobs can get slower
with more workers because channel/scheduling work exceeds useful parallel work.

The saved short samples are exploratory, not a statistical speedup claim. No
optimization is introduced without a demonstrated problem and repeated baseline.

## Profiles and trace

In PowerShell or macOS Terminal (zsh or bash):

```sh
go test ./internal/runner -run '^$' -bench '^BenchmarkBatch$/^cpu$/^workers_4$' -benchtime 2s -cpuprofile measurements/cpu.pprof -memprofile measurements/heap.pprof
go test ./internal/runner -run '^$' -bench '^BenchmarkWaitingBatch$' -benchtime 1s -blockprofile measurements/block.pprof -blockprofilerate 1 -mutexprofile measurements/mutex.pprof -mutexprofilefraction 1
go tool pprof -top measurements/cpu.pprof
go tool pprof -top -alloc_space measurements/heap.pprof
go tool pprof -top measurements/block.pprof
go tool pprof -top measurements/mutex.pprof
go test ./internal/runner -run TestDiagnosticWorkload -trace measurements/trace.out
go test ./internal/runner -run TestGoroutineSnapshot -args -goroutineprofile "$PWD/measurements/goroutine.txt"
go tool trace measurements/trace.out
```

Profiling changes timings; compare unprofiled samples separately. CPU samples
show active computation and scheduling work, not the wall time spent asleep.
alloc_space is cumulative sampled allocation; inuse_space is live sampled heap.
Block profiles attribute synchronization waiting. Mutex profiles measure contention
attributed to lock release, and can include runtime locks even when application
state uses channel ownership. An empty mutex profile is also informative for a
workload; it does not prove every lock is contention-free.

The trace records goroutine scheduling, blocking, timers and GC events.
TestDiagnosticWorkload adds a task and timer regions, so operations are named in
the trace. pprof labels use context for profiling metadata. Trace regions stay
inside one goroutine; a task can connect work across goroutines through context.
Opening go tool trace is an optional interactive browser workflow.

goroutine.txt is a live stack snapshot taken while two known jobs are blocked.
Look for their ctx.Done receive, dispatcher/join waits and the test's own stack.
It is a readable debug=2 snapshot, not a pprof binary profile. A stack snapshot
shows where goroutines are now, not how long they spent there.

Profiling go test can leave `runner.test.exe`; this generated binary is not source.
Saved profiles are local diagnostic artifacts and may contain machine paths.

## Runtime experiments

```powershell
$env:GOMAXPROCS='1'
go test ./internal/runner -run '^$' -bench '^BenchmarkBatch$' -benchmem -benchtime 200ms
Remove-Item Env:\GOMAXPROCS
$env:GODEBUG='schedtrace=1000,gctrace=1'
go test ./internal/runner -run '^$' -bench '^BenchmarkBatch$' -benchtime 2s
Remove-Item Env:\GODEBUG
go test ./internal/runner -gcflags='-m=1' -run '^$'
```

On macOS (zsh or bash), these assignments apply only to each command:

```sh
GOMAXPROCS=1 go test ./internal/runner -run '^$' -bench '^BenchmarkBatch$' -benchmem -benchtime 200ms
GODEBUG='schedtrace=1000,gctrace=1' go test ./internal/runner -run '^$' -bench '^BenchmarkBatch$' -benchtime 2s
go test ./internal/runner -gcflags='-m=1' -run '^$'
```

For the PowerShell examples, record any existing environment values and restore them
afterward; the example assumes the variables were initially unset. Escape analysis
explains compiler choices; it is not a rule that every pointer must allocate.
Goroutine stacks can grow, and heap objects become collectible after references
are released. Our batch holds all results, whereas the stream releases consumed
work incrementally. Avoid sync.Pool until profiles identify reusable allocation
pressure; pooling can retain memory and complicate ownership.

## Exercises

1. Compare worker counts for no-op, CPU and waiting work. Explain which bottleneck
   each exercises before choosing a faster-looking number.
2. Use the trace to find a blocked channel send and a timer wait. Which is
   backpressure, and which is simulated work?
3. Compare alloc_space and inuse_space. Why can allocated bytes grow throughout
   a run while the live heap remains bounded?
4. Run the bounded-input/output cancellation tests repeatedly. Inject a deliberate
   missing cancellation branch only in a scratch copy, use a timeout guard, and
   restore the branch afterward. No intentionally racy tests ship in the suite.

All five main runner lessons now have implementations and guides. Additional
language/specialization companion labs in the curriculum remain separate study
extensions. The database-backed HTTP service is the next project.
