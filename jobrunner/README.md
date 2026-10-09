# Jobrunner — five Go lessons

All five main lessons are implemented. Read the guides in order; the current
source contains the complete runner, while Lesson 1 preserves the introductory
walkthrough. No external services or third-party dependencies are required.

| Lesson | Guide | Main source |
| --- | --- | --- |
| 1. Own worker lifetimes | [LESSON1.md](LESSON1.md) | internal/runner/runner.go, runner_test.go |
| 2. Cancellation and deadlines | [LESSON2.md](LESSON2.md) | options.go, options_test.go, main.go |
| 3. Retries and backoff | [LESSON3.md](LESSON3.md) | retry.go, retry_test.go, demo.go |
| 4. Streaming and backpressure | [LESSON4.md](LESSON4.md) | stream.go, stream_test.go, admission.go, cli_stream.go |
| 5. Correctness and diagnostics | [LESSON5.md](LESSON5.md) | benchmark_test.go, diagnostics_test.go, fuzz_test.go |

Each guide has runnable commands, design explanations, test reading suggestions,
prediction exercises and limits of the guarantees. Comments teach concepts beside
the relevant code. Names in the source column are under internal/runner unless
they refer to the command's main.go, demo.go or cli_stream.go.

## Try each feature

```powershell
Set-Location C:\Users\user\personal\dev\go\jobrunner
go run . -workers 2 -jobs 6 -duration 100ms -timeout 2s
go run . -jobs 3 -duration 100ms -job-timeout 20ms
go run . -jobs 3 -duration 0 -fail-first 2 -attempts 3 -backoff 10ms
go run . -stream -workers 2 -jobs 8 -duration 20ms -interval 30ms
go run . -h
go test -count=1 ./...
go vet ./...
go build ./...
```

The deadline demo intentionally exits with failure. Ctrl+C cancels a running
command through the same context used by jobs. Batch output is input-ordered;
streaming output is completion-ordered. Ordinary job errors permit unrelated
work to continue. Output errors cancel streaming children and join them.

`-interval` limits new-job admission, not retry attempts. Jobs and callbacks must
cooperate with context; the runner cannot forcibly terminate arbitrary Go code.
Streaming cancellation may discard pending output to unblock shutdown. Default
batch mode retains one result for every input.

## Checks and measurements

Run the commands in Lesson 5 for fuzzing, race detection, benchmarks, CPU/heap/
block/mutex profiles, a live goroutine snapshot and execution trace. Saved local
artifacts are in [measurements](measurements/README.md). Short samples illustrate
measurement techniques; they do not establish universal speedups.

The race command was attempted and cannot run in the current CGO-disabled Windows
environment. A supported C compiler and CGO-enabled toolchain are required; no
race pass is claimed. Cancellation/race tests exercise paths, not every schedule.

The [three-project curriculum](../docs/superpowers/specs/2026-10-09-jobrunner-design.md)
maps all 20 roadmap chapters. Additional language/specialist companion labs remain
separate study extensions; these five runner lessons do not implement every
roadmap topic. The database-backed HTTP service is project three.
