# streamgrep — Lesson 5: measure before optimizing

Read **`benchmark_test.go`** for comments explaining benchmark setup, timed work,
input bytes versus output bytes, allocation counters, and optional I/O methods.
The CLI's behavior stays the same; `filter.go` now writes an assembled line
directly after profiling identified a per-line reader allocation.

## Run benchmarks

```powershell
Set-Location C:\Users\user\personal\dev\go\streamgrep
go test ./...
go test -run '^$' -bench BenchmarkFilterLines -benchmem -benchtime=1s -count=5
go test -run '^$' -bench BenchmarkCopyStreamPaths -benchmem -benchtime=1s -count=5
```

On macOS (zsh or bash), adjust the path to your checkout location:

```sh
cd ~/personal/dev/go/streamgrep
go test ./...
go test -run '^$' -bench BenchmarkFilterLines -benchmem -benchtime=1s -count=5
go test -run '^$' -bench BenchmarkCopyStreamPaths -benchmem -benchtime=1s -count=5
```

`-run '^$'` skips ordinary tests for this measurement; it does not mean correctness
has been checked. Run tests separately first. `-count` repeats the measurements.
`-benchtime` gives each benchmark a measurement duration, not a fixed iteration
count. Go chooses how many operations to execute.

The checked-in local baseline used three 100 ms samples per case for a quick
exploration. Use longer runs and more samples when making performance decisions.
Raw results and the allocation profile are in `measurements/`.

## Define the operation before reading numbers

One filter operation processes an entire fixture from a fresh reader position:

- Short lines are 128 bytes including LF; long lines are 8 KiB including LF.
- Input sizes are 4 KiB, 64 KiB, and 1 MiB across the benchmark cases.
- ERROR and INFO lines alternate, so an ERROR filter matches half the bytes.
- Empty and absent filters exercise all-match and no-match cases at 4 KiB.
- The destination is `io.Discard`, isolating filtering from retaining output.

This measures the in-memory filter, including its per-call buffers, writes, and
the harness's reader reset. It excludes fixture construction, process startup,
flag parsing, and real disk or pipe latency. Fixture allocations belong to the
benchmark setup, not to the streaming production algorithm.

`b.Loop()` resets timing/allocation counters at its first call and stops timing
when it returns false. Setup goes before the loop and final assertions after it.
The exact `for b.Loop() { ... }` form also protects benchmark work from being
fully optimized away. Our module already requires Go 1.27.0, which supports it.

The reader must be reset every iteration. Otherwise, later operations would
measure an already exhausted stream and produce a misleadingly small time.
The fixture and output-count checks guard against labeling a broken run as fast;
ordinary tests still check full bytes and error contracts.

## Read benchmark output

| Field | Meaning |
| --- | --- |
| Iteration count | Number of complete operations used in that sample. |
| `ns/op` | Average nanoseconds for one entire input fixture. |
| `MB/s` | Logical input-byte throughput recorded by `SetBytes`; decimal MB. |
| `B/op` | Bytes allocated per operation, not peak or retained memory. |
| `allocs/op` | Number of allocations per operation. |

An absent filter still processes all input, so it can report input throughput
while writing zero bytes. Compare cases with the same input and output contract.
The numeric suffix on a benchmark name records GOMAXPROCS, not a claim that these
benchmarks run multiple filter operations concurrently.

## What the measurements changed

The baseline's 1 MiB short-line/half-match case reported **200,832 B/op and 4,098
allocs/op**. An allocation-space profile attributed about **98%** of its sampled
allocated bytes to `bytes.NewReader` in the matched-line output path.

We already had a complete line slice. Writing that slice directly eliminated
the extra reader object per match. The code explicitly retains the short-write
check that `io.Copy` previously supplied; partial counts and error identity are
still covered by the failure tests.

The same case afterward reported **4,224 B/op and 2 allocs/op**. The three-sample
median changed from **237,607 ns/op to 146,622 ns/op** on this machine. The
allocation reduction is the clearest evidence. Short, separate before/after
timing batches do not establish a statistically reliable speedup or predict
file/pipe performance. See `measurements/README.md` for the exact setup and data.

`TestFilterLinesAllocationGrowth` compares all-match inputs with the same short
line shape at 4 KiB and 64 KiB. Before the change it failed with 34 versus 514
allocations; afterward it reports 2 versus 2. The test tolerates fixed overhead
variation rather than pinning one exact compiler-dependent allocation count.

## Why optional interfaces matter

`BenchmarkCopyStreamPaths` keeps the same logical 1 MiB input but changes which
methods the streams expose:

1. `WriterTo`: `bytes.Reader.WriteTo` can hand the whole slice to `io.Discard`.
2. `ReaderFrom`: hiding `WriterTo` lets the destination's read path take over.
3. `generic_buffer`: hiding both optional methods forces `io.Copy`'s buffer path.

The first path's discard writer does not inspect the slice contents. Its enormous
reported MB/s is logical accounting, **not measured memory or disk bandwidth**.
Do not treat it as an optimization candidate for filtering: the filter must
actually find line boundaries and inspect content. The wrapper comments explain
how embedding an interface hides the concrete stream's optional methods.

## Inspect a profile

```powershell
go test -run '^$' -bench '^BenchmarkFilterLines$/^short_1MiB_half$' -benchtime=3s -memprofile .\measurements\alloc-current.pprof
go tool pprof -top -alloc_space .\measurements\alloc-current.pprof
go tool pprof -top -alloc_space .\measurements\alloc-before.pprof
```

On macOS (zsh or bash):

```sh
go test -run '^$' -bench '^BenchmarkFilterLines$/^short_1MiB_half$' -benchtime=3s -memprofile ./measurements/alloc-current.pprof
go tool pprof -top -alloc_space ./measurements/alloc-current.pprof
go tool pprof -top -alloc_space ./measurements/alloc-before.pprof
```

Profiling adds overhead; keep its timings separate from unprofiled comparison
runs. `alloc_space` reports cumulative sampled allocation during the run, not
the process's live heap or peak memory. Profile attribution is sampled, so its
percentages are approximate. A profiled test run may leave `streamgrep.test.exe`;
remove that generated binary after inspecting it if you do not need it.

## Exercises

1. **Predict allocation growth.** Run `go test -run TestFilterLinesAllocationGrowth -v`.
   Explain why increasing the number of equally sized lines need not increase
   allocation count, while increasing maximum line length can grow a buffer.
2. **Compare matching rates.** Benchmark the three 4 KiB cases. They scan the same
   amount of input but do different amounts of output work. Repeat enough times
   to see timing variation rather than trusting a single fastest sample.
3. **Change one factor.** Add an all-match 64 KiB case, or vary line size while
   keeping input size fixed. The fixture requires an even number of complete
   lines. Predict the work and allocations before measuring.
4. **Investigate a new hypothesis.** If you propose another optimization, save
   repeated baseline results first, change only that factor, rerun correctness
   and failure tests, then compare repeated measurements. Explain any changed
   buffering or failure semantics; discard changes unsupported by evidence.

Use `go doc testing.B.Loop`, `go doc testing.B.ReportAllocs`, and `go doc io.Copy`
to read the installed toolchain's documentation.

## Five lessons completed

`LESSON1.md` through `LESSON4.md` preserve earlier walkthroughs. Continue using:

```powershell
"INFO ready`nERROR failed" | go run . -contains ERROR
go run . -h
```

On macOS (zsh or bash):

```sh
printf '%s\n' 'INFO ready' 'ERROR failed' | go run . -contains ERROR
go run . -h
```

You now have a streaming CLI with comments explaining I/O, line handling, flags,
failure behavior, and measured optimization. The next roadmap project is a
concurrent crawler or job runner, where goroutine lifetimes and cancellation
become practical concerns.
