# streamgrep Lesson 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans for native execution or superpowers:subagent-driven-development for delegated execution. Implement task-by-task and track the checkboxes.

**Goal:** Deliver a runnable, commented stdin-to-stdout copier that teaches streaming I/O and error handling through examples and tests.

**Architecture:** One Go module with one `main` package. `copyStream` owns copying and returns its byte count and error; `run` owns diagnostics and status; `main` connects the process streams. Callers retain ownership of readers and writers.

**Tech Stack:** Installed Go 1.27.0, standard library, PowerShell.

**Spec:** `docs/superpowers/specs/2026-10-09-streamgrep-design.md`

## Global Constraints

- The first implementation session covers Lesson 1 only.
- Use the Go standard library with no external dependencies.
- This stage preserves bytes exactly and has no filtering flags or line-size limit.
- Processing functions do not close streams.
- Comments explain the Go concept and reasoning at the relevant statement.
- Provide a working baseline instead of leaving required code as TODOs.
- No performance or race-detector result is claimed without running that check.
- No Git repository exists here; do not initialize one merely to commit this lesson.

## File responsibilities

- `streamgrep/go.mod`: local module `example.com/streamgrep`, `go 1.27.0`.
- `streamgrep/main.go`: commented copy function, CLI wiring, diagnostics.
- `streamgrep/main_test.go`: observable behavior tests and test-only stream types.
- `streamgrep/README.md`: Lesson 1 walkthrough, PowerShell commands, exercises.

## Review Focus

Each condition below receives a test in Task 1 or Task 2.

- Data and EOF returned together: copy the data and succeed.
- Short write without an error: report `io.ErrShortWrite`.
- Partial data followed by a read error: preserve copied bytes and error identity.
- Large generated input: process repeated bounded chunks and copy every byte.
- Output failure on an unbounded source: stop reading after the failing write.

---

### Task 1: Copy a stream and teach its contracts

**Files:** Create `streamgrep/go.mod`, `streamgrep/main.go`, `streamgrep/main_test.go`.

**Interfaces:**
- Produces `copyStream(dst io.Writer, src io.Reader) (int64, error)`.
- Uses `io.Copy`; wraps failures with `fmt.Errorf("copy stream: %w", err)`.
- Test helper types implement `Read([]byte) (int, error)` or `Write([]byte) (int, error)` only; avoid accidental `WriterTo`/`ReaderFrom` fast paths in repeated-chunk tests.

- [x] Create the module and write `TestCopyStreamPreservesBytes` as table-driven subtests. Assert byte count, nil error, and exact bytes for empty input, UTF-8, CRLF, embedded zero bytes, and a final line without a newline. Representative assertions:

```go
var dst bytes.Buffer
n, err := copyStream(&dst, bytes.NewReader(input))
if err != nil { t.Fatalf("copyStream: %v", err) }
if n != int64(len(input)) { t.Fatalf("copied %d bytes, want %d", n, len(input)) }
if !bytes.Equal(dst.Bytes(), input) { t.Fatalf("output %q, want %q", dst.Bytes(), input) }
```

- [x] Add a signature-only stub returning `(0, nil)` to allow compilation. Run `go test -run TestCopyStreamPreservesBytes -v` in `streamgrep/`; nonempty cases must FAIL with missing bytes.
- [x] Implement copying with `io.Copy` and return its results without wrapping yet. Run `go test ./...`; expect PASS.
- [x] Write `TestCopyStreamWrapsFailures` with injected read and write failures. Assert `errors.Is(err, injectedError)` and that an outer error was added (`err != injectedError`). Run the test; expect FAIL because the raw error is returned.
- [x] Wrap the error using `%w`, preserving the byte count. Run `go test ./...`; expect PASS.
- [x] Add contract tests for the five Review Focus conditions. Use `iotest.DataErrReader` for data with EOF; an error-free short writer for `io.ErrShortWrite`; and a reader returning three bytes plus a sentinel error for partial reads. Assert copied bytes/count as well as error identity.
- [x] For the large-stream test, generate `2*1024*1024` bytes of a constant value, at most 4096 per read. A validating writer checks content and counts bytes without storing the stream. Assert total bytes, EOF, and more than one read/write.
- [x] For output failure, use a reader that always supplies data and tracks calls, plus a writer that fails immediately. Assert one read, zero successfully written bytes, and the wrapped writer error. Keep tests deterministic and finite if the behavior regresses: fail the reader after a small maximum call count.
- [x] Run `go test ./...`; expect PASS. Explain that supplementary `io.Copy` contract tests can pass immediately because the standard library already implements those contracts; they document the delegated behavior.
- [x] Add focused comments explaining structural interfaces, `(n, err)`, data with errors, EOF, byte preservation, bounded copying versus reading everything, caller ownership, and error identity. Mention `io.Copy` fast paths and avoid claiming it always uses one fixed-size buffer.

### Task 2: Wire the executable and provide the lesson

**Files:** Modify `streamgrep/main.go`, `streamgrep/main_test.go`; create `streamgrep/README.md`.

**Interfaces:**
- Consumes `copyStream(dst io.Writer, src io.Reader) (int64, error)`.
- Produces `run(stdin io.Reader, stdout io.Writer, stderr io.Writer) int`.
- `run` returns 0 on success; 1 on a copy failure and writes a diagnostic prefixed `streamgrep:` to stderr. A failed diagnostic write still returns 1.
- `main()` calls `os.Exit(run(os.Stdin, os.Stdout, os.Stderr))`.

- [x] Write `TestRun`: success copies bytes with no diagnostic; read and write failures return 1 with a stderr diagnostic; already copied output survives a later read failure; a diagnostic-writer failure does not cause a panic. Assert diagnostic prefix and useful error detail, not exact complete strings.
- [x] Add a `run` stub returning 0. Run `go test -run TestRun -v`; expect FAIL on missing output and error status.
- [x] Implement `run` and the minimal `main` wiring. Explain short declarations, assignment to `_`, stderr separation, exit status, and why `os.Exit` skips deferred cleanup. Run `go test ./...`; expect PASS.
- [x] Write the README with reading order (`main.go`, then `main_test.go`), design choices, and these commands:

```powershell
Set-Location C:\Users\user\personal\dev\go\streamgrep
'hello streaming Go' | go run .
go test -v ./...
```

Explain that PowerShell encodes text and adds line endings before the program receives bytes. For byte-exact file redirection, demonstrate a built executable through `cmd /c` from PowerShell. EOF ends copying; Lesson 1 has no `-contains` flag yet.
- [x] Include three exercises: predict CRLF and unterminated-line behavior; change a test's injected error and inspect `errors.Is`; trace which functions consume memory as generated input size increases. State observable outcomes without requiring performance claims.
- [x] Run `gofmt -w main.go main_test.go`, `go test ./...`, `go vet ./...`, and `go build ./...` in `streamgrep/`. Expect exit code 0 from each.
- [x] Run the PowerShell example and verify its visible output. Use a temporary input/output pair with CRLF, zero bytes, Unicode bytes, and no final newline to verify the built executable preserves file bytes through shell redirection. Keep temporary artifacts outside the lesson source and remove the generated executable after verification.
- [x] Self-review against Lesson 1 acceptance criteria. Report actual verification outcomes, link the commented source and tests, and stop after Lesson 1 so the learner can explore it.

## Execution choice

Recommend native execution in this chat: there are only two closely related tasks in one small package. The user must review this saved plan and choose native or delegated execution before implementation.
