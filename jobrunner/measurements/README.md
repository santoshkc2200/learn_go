# Local runner measurements

Measurements collected on 2026-10-09 using Go 1.27.0 windows/amd64. See the saved
environment and result files for exact commands and machine details.

## What the short samples show

Three 100 ms samples per configuration; medians in ns per 64-job batch:

| Workers | No-op | CPU (SHA-256 of 4 KiB per job) |
| --- | ---: | ---: |
| 1 | 22,823 | 195,460 |
| 2 | 23,009 | 119,139 |
| 4 | 26,298 | 84,584 |

In these samples extra workers added overhead for tiny no-op jobs while CPU work
benefited from parallelism. Allocation counts rose from 4 to 5 to 7 per batch
with worker counts 1, 2 and 4. These are observations of this workload, not a
general rule that four workers is best. See benchmarks.txt for every sample.

CPU profiling attributed about 55% of sampled CPU time to SHA-256's block routine;
the rest included runtime scheduling/synchronization. Total sampled CPU (1.14s)
exceeded wall duration (about 572ms), which is possible with parallel CPU work.

The heap alloc_space report attributed about 78% of sampled allocation to
RunWithOptions and another 5% to its worker closures. Fixed per-batch result/
channel/worker setup is part of what this benchmark measures. The profile also
includes test/runtime/profiler setup: do not equate its total bytes with B/op.

The waiting block profile showed about 66% cumulative blocking through the timer
wait helper; select and channel waits also include dispatcher/test coordination.
Aggregate blocked time can exceed wall time because multiple goroutines wait.
The mutex profile showed about 65 microseconds attributed to runtime.unlock;
this workload has no application-owned contended mutex to optimize.

cpu-heap-top.txt and block-mutex-top.txt preserve inspected reports. trace.out
contains task/region scheduling events; goroutine.txt preserves live stacks
while known jobs wait on cancellation. The test deliberately snapshots before
joining, when the relevant goroutines still exist.

The short repeated benchmark samples are exploratory. CPU/heap and waiting
block/mutex profiles were collected in separate instrumented runs. A diagnostic
trace covers labeled timer jobs. Profile overhead and system load affect timing;
do not compare instrumented and uninstrumented ns/op as an optimization result.

No code optimization or reliable speedup is claimed. Use the longer commands in
LESSON5.md before making performance decisions. Race instrumentation is unavailable
in the current CGO-disabled environment.
