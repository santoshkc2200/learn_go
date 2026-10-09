# streamgrep: learn Go by building a streaming CLI

## Purpose

Build a working Go CLI in small lessons. The learner already knows basic Go
syntax and wants concepts explained in source comments, with practical examples
of design choices, failure testing, and measurement before optimization.

The first implementation session covers Lesson 1 only. Later lessons extend the
same project after the learner has explored the current lesson.

## Final behavior

`streamgrep -contains ERROR` reads standard input and writes lines containing the
case-sensitive literal substring `ERROR` to standard output. It uses bounded
buffers rather than loading the entire input into memory.

- `-contains` defaults to an empty string, which matches every line.
- No positional arguments or file-opening flags: use standard input redirection.
- The line-filtering stage accepts LF and CRLF and emits matching lines with LF.
- An unterminated final line is processed and emitted with LF if it matches.
- Empty input produces no output; a blank line matches only an empty filter.
- The filtering stage permits up to 1 MiB of line content, excluding LF or CRLF;
  longer lines return an error instead of growing memory without a bound.
- Successful processing, including no matches, returns exit code 0.
- Input or output failures return exit code 1; invalid arguments return 2.
- Diagnostics go to standard error. Already written output is not rolled back.
- `-h` prints usage and returns exit code 0.

## Lesson sequence

1. **Copy a stream.** Create a Go module and an executable that copies stdin to
   stdout byte for byte using `io.Copy`. Explain interfaces, implicit interface
   satisfaction, short variable declarations, multiple return values, EOF,
   error wrapping, and why streaming does not require goroutines. This stage
   preserves bytes exactly and has no filtering flags or line-size limit.
2. **Filter lines.** Introduce buffered reading, substring matching, newline
   handling, final lines, and the explicit line-size bound. Explain the chosen
   reading strategy and its limits beside the code.
3. **Expose CLI arguments.** Add `-contains`, usage, argument validation, and
   separate stdout/stderr. Demonstrate shell redirection and pipelines.
4. **Explore failures.** Extend tests with readers and writers that fail,
   checking error identity and partial output. Basic I/O failure tests already
   exist in Lesson 1; this lesson studies their behavior more deeply.
5. **Measure.** Add benchmarks with allocation reporting and controlled input
   sizes. Explain setup versus timed work, interface fast paths, and why a
   single measurement cannot establish an improvement. Optimize only if the
   measurements identify a useful change.

## Structure and design choices

Place the module under `streamgrep/` with a single `main` package. Start with
`main.go`, `main_test.go`, and `README.md`; split files only when it improves
readability. Use the Go standard library with no external dependencies.

Keep stream processing in functions accepting `io.Reader` and `io.Writer`.
This permits real stdin/stdout in the CLI and in-memory or failing streams in
tests without special branches in production code. The caller owns streams;
processing functions do not close them.

Keep `main` responsible for process-level wiring and exit status. Return errors
from processing functions; explain why `os.Exit` bypasses deferred cleanup and
why reusable logic should not exit the process.

Use synchronous processing in the first project version. No background workers
or application-owned goroutines are needed; race and leak investigation will
become a practical focus in the subsequent concurrent project.

## Teaching approach

Comments explain the Go concept and reasoning at the relevant statement, not
merely restate the syntax. Tests also explain assertions, failure injection,
and the behaviors they protect. Keep each lesson small enough to read in one
sitting. The README records the current lesson, runnable PowerShell commands,
expected output, design choices, and short exercises with observable outcomes.

Provide a working baseline instead of leaving required code as TODOs. Exercises
ask the learner to predict behavior or make a small change and rerun tests.
Distinguish shell text-pipeline encoding from the executable's byte-preserving
behavior when demonstrating Lesson 1 on Windows.

## Lesson 1 acceptance criteria

- `go run .` copies stdin to stdout; EOF terminates normally.
- Automated tests verify empty input and exact byte preservation, including
  Unicode, CRLF, embedded zero bytes, and an unterminated final line.
- Injected read and write errors remain identifiable through wrapping with
  `errors.Is`; tests do not depend on entire diagnostic strings.
- A generated input larger than the copy buffer exercises repeated reads and
  writes, without constructing the entire input in memory.
- A failing output stops copying without consuming an unbounded input.
- Tests are written and observed failing before the corresponding implementation.
- Source is formatted; `go test ./...`, `go vet ./...`, and `go build ./...` pass.
- No performance or race-detector result is claimed without running that check.
- Delivery links to the source and tests and includes a command to run Lesson 1.

## Out of scope

Regular expressions, recursive file search, concurrency, networking, third-party
frameworks, deployment, and a finished implementation of all five lessons in
the first session.
