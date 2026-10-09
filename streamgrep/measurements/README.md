# Lesson 5 measurements — 2026-10-09 (Japan time)

Environment: Go 1.27.0, Windows/amd64, Intel Core i7-12700F. Benchmark names report
GOMAXPROCS 20. These are sequential in-memory microbenchmarks using `io.Discard`.

## Reproduction and artifacts

From the module directory, both comparison batches used:

In PowerShell or macOS Terminal (zsh or bash):

```sh
go test -run '^$' -bench 'Benchmark(FilterLines|CopyStreamPaths)' -benchmem -benchtime=100ms -count=3
```

- `environment.txt`: toolchain and platform.
- `baseline.txt`: before changing the matched-line output path.
- `after.txt`: direct-write implementation, same cases and flags.
- `profile-run.txt`: separate one-second allocation-profiling run of the baseline.
- `alloc-before.pprof`: baseline sampled allocation profile.
- `alloc-before.txt`: its `go tool pprof -top -alloc_space` summary.

The production change replaces a per-matched-line `bytes.NewReader` plus
`io.Copy` with `dst.Write(line)` and an explicit short-write check. It preserves
output bytes, partial byte counts, stopping behavior, and wrapped error identity.
The existing failure tests pass after the change.

## Selected results

Timing values below are medians of three samples; allocations were identical
within each corresponding unprofiled case. They are exploratory local results.

| Filter case | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 4 KiB, 128-byte lines, all match | 1,830 | 1,171 | 5,760 | 4,224 | 34 | 2 |
| 64 KiB, 128-byte lines, half match | 14,915 | 9,966 | 16,512 | 4,224 | 258 | 2 |
| 1 MiB, 128-byte lines, half match | 237,607 | 146,622 | 200,832 | 4,224 | 4,098 | 2 |
| 1 MiB, 8 KiB lines, half match | 47,570 | 44,925 | 20,736 | 17,664 | 67 | 3 |

The baseline allocation profile attributed approximately 98% of sampled allocated
space to `bytes.NewReader`, supporting the specific change. Its roughly 925 MB
total refers to cumulative sampled allocation across thousands of operations,
not a 925 MB live heap or an allocation needed for one input.

## Limits and interpretation

These were two separate short batches with a profiling run between them. There
was no randomized ordering, controlled system-load experiment, or statistical
significance analysis. Some unchanged copy-path cases also varied substantially,
demonstrating why timing alone is weak evidence. The allocation reduction and
the allocation-growth regression test support the narrow conclusion that the
change removes per-match reader allocations in these fixtures.

The benchmark output checks byte counts and ordinary tests check content/errors.
No actual disk, pipe, CLI-startup, peak-memory, or concurrent-workload performance
claim follows from this data. A broader workload should be measured before
making deployment or latency decisions.
