# streamgrep — Lesson 1 notes: copy a stream

These are the original Lesson 1 notes. `copyStream` and its tests remain in the
project, but the executable now uses Lesson 2's line filter. The file-copy
commands below describe the Lesson 1 executable; the current executable
normalizes line endings. See `README.md` for the current behavior.

Build Go knowledge by changing a working program. This lesson copies standard
input to standard output **byte for byte**. Filtering and `-contains` arrive in
later lessons; there are no flags yet.

## Run it

From PowerShell:

```powershell
Set-Location C:\Users\user\personal\dev\go\streamgrep
'hello streaming Go' | go run .
```

On macOS (zsh or bash), adjust the path to your checkout location:

```sh
cd ~/personal/dev/go/streamgrep
printf '%s\n' 'hello streaming Go' | go run .
```

Expected visible output:

```text
hello streaming Go
```

`go run .` compiles and runs this directory's `main` package. The local module
name `example.com/streamgrep` is an identifier; running this project does not
contact that domain. This module requires Go 1.27.0 or later and no dependencies.

PowerShell's pipeline sends encoded text and line endings to the executable.
The program preserves the bytes it receives; the shell chooses those initial
bytes. Consequently, a text-pipeline example is not a test of arbitrary binary
file preservation.

To copy file bytes using Windows file redirection, build first and use `cmd`:

```powershell
go build -o streamgrep.exe .
cmd /c ".\streamgrep.exe < input.bin > output.bin"
```

On macOS (zsh or bash):

```sh
go build -o streamgrep .
./streamgrep < input.bin > output.bin
```

Create `input.bin` first. A successful copy makes `output.bin` identical. Input
files are opened by the shell, not by the Go program. Running without a pipe or
redirect waits for terminal input; on a Windows console, Ctrl+Z followed by Enter
typically signals EOF. On macOS, Ctrl+D on an empty line signals EOF.
A pipe signals EOF when its producer finishes.

## Read the code in this order

1. `copyStream` in `main.go`: the reusable operation and I/O concepts.
2. `run`: diagnostic separation and exit status.
3. `main`: connecting the operation to process streams.
4. `main_test.go`: byte fixtures first, then failures and generated streams.

The teaching comments sit beside the relevant code. The program itself has
three small functions; the tests are longer because they illustrate different
ways streams can end or fail. You do not need to memorize the test helpers.

## Design choices to explain in your own words

| Choice | Reason |
| --- | --- |
| Accept `io.Reader` and `io.Writer` | The same operation works with files, buffers, and failing streams. |
| Use `io.Copy` | Reuse its handling of EOF, partial reads, short writes, and incremental transfer. |
| Return `(int64, error)` | A transfer can write some bytes and still fail. |
| Wrap with `%w` | Add context while retaining an identifiable original cause. |
| Put diagnostics on stderr | Keep stdout usable as a data stream. |
| Let the caller own streams | Avoid closing resources someone else still needs. |
| Process synchronously | No queue or goroutine lifecycle is needed for sequential copying. |

A stream is processed incrementally: read some bytes, write them, repeat. With
these stdin/stdout streams, the copier does not collect the entire input in
memory. A `bytes.Buffer` destination, however, retains all output. The choice of
writer still matters. `io.Copy` can also delegate to `WriterTo` or `ReaderFrom`,
so it does not always execute one identical buffering strategy.

## Run the tests

In PowerShell or macOS Terminal (zsh or bash):

```sh
go test -v ./...
go test -run TestCopyStreamPartialReadFailure -v
go test -run TestCopyStreamLargeGeneratedInput -v
go vet ./...
```

Tests check actual copied bytes, byte counts, error identity, and CLI status.
The large-input test generates 2 MiB incrementally and validates output without
retaining it. This exercises incremental transfer; it is not a benchmark or a
measurement of peak memory. Timing and allocation measurements come in Lesson 5.

The stop-on-write-failure test checks that a failed destination stops further
reads. Its source has a safety guard so a regression fails rather than hangs.
The application starts no goroutines; investigating concurrency races and leaks
belongs in our later concurrent project.

## Exercises

### 1. Predict before running

For these inputs, predict the output bytes and the returned count:

- `[]byte("a\r\nb")`
- `[]byte{'a', 0, 'b'}`
- `[]byte("こんにちは")`

Add your predictions as table cases in `TestCopyStreamPreservesBytes`, then run
the test. Count **bytes**, not characters. Output must preserve CRLF, zero bytes,
and the absence of a final newline. `len` on a string reports its byte length.

### 2. Follow a failure through the program

Read `TestCopyStreamPartialReadFailure`, then change its injected error message
and rerun that test. Predict whether `errors.Is` still succeeds. It should: the
copier wraps that exact error value, regardless of its text.

As a temporary experiment, replace `%w` with `%v` in `copyStream`. Run
`go test -run TestCopyStreamWrapsFailures -v`: it should fail because the original
cause is no longer in the error chain. Restore `%w` and rerun all tests.

### 3. Trace memory ownership

In the large-input test, raise `size` from 2 MiB to 20 MiB. Before running, find
which helper could retain the whole stream. Neither does: the reader fills the
provided slice, and the writer validates and counts without retaining it.

Now explain why using `bytes.Buffer` instead of `validatingWriter` would retain
more data as input grows. The test passing supports correctness at the larger
size; it does not prove a particular memory or speed measurement.

## Next lesson

Extend the same operation to filter lines. That introduces buffered reading,
string matching, final lines without a newline, and an explicit line-size limit.
First, be able to explain why a Reader can return both bytes and an error,
and why EOF is a successful end of this copy operation.
