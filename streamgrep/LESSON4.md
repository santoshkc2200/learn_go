# streamgrep — Lesson 4: explore failures

These are the Lesson 4 notes. The failure contracts and experiments still apply.
`README.md` now introduces Lesson 5's benchmarks and measured output-path change.

This chapter teaches how to reason about failures in a working stream processor.
The CLI still supports `-contains`, LF output, and a 1 MiB line-content limit.
The new material lives in **`failure_test.go`**, with explanations beside the
tests and a writer that accepts a chosen number of bytes before failing.

Production behavior already handled these paths. This lesson adds experiments
and regression proof without adding retries or changing the CLI contract.

## Start here

```powershell
Set-Location C:\Users\user\personal\dev\go\streamgrep
go test -run TestFailure -v
go test -v ./...
```

Read in this order:

1. `limitedWriter` and its `Write` method: how a partial write is simulated.
2. `TestFailurePartialWrite`: output bytes, byte counts, and stopping behavior.
3. `TestFailureReadErrorIdentity`: `errors.Is`, `errors.As`, and `%w`.
4. The line-limit, retry, and simultaneous output/diagnostic failure tests.

## Failure contracts

| Situation | Output and result |
| --- | --- |
| EOF with a final fragment | Process the final line and succeed. |
| A read fails within a line | Keep emitted complete lines; discard the interrupted fragment; return the read error. |
| A write accepts some bytes, then fails | Keep those bytes, include them in the output count, and stop. |
| A write returns fewer bytes without an error | Report `io.ErrShortWrite`; this violates the Writer contract. |
| A later line exceeds the limit | Keep the emitted prefix, return the line-limit error, and stop before later lines. |
| A read fails even though no lines matched | Return failure; no matches does not erase a source error. |
| Writing the diagnostic also fails | Retain partial stdout and return status 1. |

The filter returns an error for callers to inspect. `run` writes a diagnostic to
stderr and returns status 1 for processing failures; `main` turns that status into
the process exit code. Tests supply streams as arguments, so failures are
deterministic rather than depending on an actual disk filling or a pipe closing.

## Partial output is real output

`io.Writer.Write(p)` returns `(n, err)`. When `n > 0` and `err != nil`, the writer
accepted those `n` bytes before failing. Previously successful writes count too.
The stream processor cannot roll them back: `io.Writer` has no undo method.

The example input includes an ignored INFO line, CRLF, and three matching lines.
A destination that permits 14 output bytes accepts:

```text
ERROR one\nERRO
```

Here `\n` represents one LF byte. The first matching line contributes 10 bytes;
the second contributes 4 before the write fails. The filter returns count 14 and
a wrapped error. Diagnostics belong on stderr, away from these output bytes.

Stopping writes does not mean stdin was read exactly up to that byte. Buffered
input may read ahead, and the matching/output count differs from the input count.

## Error identity versus error text

`errors.Is(err, cause)` asks whether an error chain contains a particular cause.
`errors.As(err, &typedError)` finds a compatible error type and exposes its fields.

The read-error test injects an `*os.PathError` that wraps a lower-level error:

```text
filter's "read line" context
  -> *os.PathError (operation, path, underlying cause)
    -> original "device disconnected" error
```

Wrapping with `%w` retains this chain. Formatting with `%v` retains its displayed
text but loses the traversable link at that layer. A separately created
`errors.New("device disconnected")` is a different value despite identical text.
Type matching and cause identity let callers inspect errors without parsing
entire diagnostic strings.

`errors.As` receives the address of the typed variable so it can populate that
variable. For `var pathError *os.PathError`, pass `&pathError`; the comment beside
the assertion explains why this argument has type `**os.PathError`.

## Why a naive retry can corrupt output

`TestFailureRetryDuplicatesOutput` deliberately restarts the original input after
the destination accepted 14 bytes. It retains the old output and makes the sink
writable again. The combined output becomes:

```text
ERROR one\nERROERROR one\nERROR two\n
```

This test demonstrates a hazard, not an automatic retry feature. Replaying input
to a destination that already received data can duplicate complete lines and
concatenate partial fragments. A retry strategy needs a destination-specific way
to resume, replace, or otherwise account for committed output.

## Exercises: predict, change, rerun

1. **Move the failure boundary.** Add a `TestFailurePartialWrite` case with limit
   11. Predict the output bytes and number of writes before running it. Include
   the newline when counting bytes.
2. **Break counting temporarily.** In `filter.go`, change `written += int64(n)` to
   `written += int64(n) * 0`. Run `go test -run TestFailurePartialWrite -v`. Which tests
   fail even though the destination contains the right bytes? Restore the line.
3. **Break wrapping temporarily.** Change `"read line: %w"` to `"read line: %v"`.
   Run `go test -run TestFailureReadErrorIdentity -v`. The message still looks
   useful, but the original cause is no longer reachable. Restore `%w`.
4. **Distinguish EOF from failure.** Compare `TestFilterLinesDataWithEOF` with the
   typed read-failure test. Explain why an EOF fragment is valid but an
   interrupted fragment is discarded.
5. **Trace a retry.** Read `TestFailureRetryDuplicatesOutput`. Predict the
   combined output if the first attempt permitted exactly 10 bytes instead of
   14. The returned count of the second call describes only that second call.

After any experiment, restore the implementation and run `go test ./...`.
These tests verify correctness and error contracts; they do not measure speed
or allocations. The CLI starts no application goroutines, so this chapter uses
deterministic stream failures rather than race or goroutine-leak exercises.

## Keep using the CLI

```powershell
"INFO ready`nERROR failed" | go run . -contains ERROR
go run . -h
```

See `LESSON3.md` for flags, file redirection, and status codes, `LESSON2.md` for
buffered line processing, and `LESSON1.md` for the original byte copier.

## Next lesson

Measure performance with benchmarks and allocation reports before optimizing.
