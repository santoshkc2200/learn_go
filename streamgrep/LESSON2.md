# streamgrep — Lesson 2: filter lines

These are the Lesson 2 notes. The CLI now accepts `-contains`, so the source-edit
exercise below is historical; use the flag instead. `filter.go` and its tests
remain the same. See `README.md` for Lesson 3's CLI behavior and commands.

The executable now processes **lines**, rather than copying every input byte.
The filtering function selects lines containing a case-sensitive literal
substring. It emits LF (`\n`) endings and accepts at most **1 MiB of content per
line**, excluding LF or CRLF. Total input size can be much larger.

The CLI currently passes an empty substring, which matches every line. Lesson 3
will introduce `-contains`; there are still no CLI flags in this lesson.

## Read and run

1. Start with `filter.go`: its comments explain each design choice.
2. Read `TestFilterLines` in `filter_test.go`: small examples pin down behavior.
3. Continue with the limit and failure tests.
4. Read `run` in `main.go` to see how the CLI uses the filter.

In PowerShell:

```powershell
Set-Location C:\Users\user\personal\dev\go\streamgrep
"INFO ready`r`nERROR failed`r`nINFO done" | go run .
go test -run TestFilterLines -v
go test -v ./...
```

On macOS (zsh or bash), adjust the path to your checkout location:

```sh
cd ~/personal/dev/go/streamgrep
printf '%s\n' 'INFO ready' 'ERROR failed' 'INFO done' | go run .
go test -run TestFilterLines -v
go test -v ./...
```

The executable prints all three lines because its substring is empty. The
matching tests call `filterLines` with `"ERROR"` and check that only matching
lines are written. PowerShell supplies encoded text and line endings; the filter
then normalizes the received delimiters to LF.

## Concepts to learn from the comments

| Concept | Where it matters |
| --- | --- |
| Buffered I/O | `bufio.Reader` reads chunks while `ReadSlice` finds LF. |
| Slice ownership | A fragment references the reader's buffer; `append` copies it before the next read. |
| Buffer-full versus failed input | `bufio.ErrBufferFull` means the line continues; a real read error stops processing. |
| EOF with data | A final unterminated line is processed before successful completion. |
| Byte limits | `1 << 20` permits 1,048,576 content bytes, not Unicode characters. |
| Literal matching | `bytes.Contains` searches for bytes; `.` is not a regex wildcard. |
| Reusing storage | `line[:0]` resets length while keeping capacity for later lines. |
| Partial writes | Output byte counts survive failures; `%w` preserves error identity. |

`ReadString` alone could allocate an arbitrarily large line before a size check.
Here `ReadSlice` provides bounded fragments, and the filter checks the assembled
size before appending. A line can span many fragments. CRLF can even cross a
buffer boundary, which has its own test.

The content limit allows two extra bytes while assembling a possible CRLF
delimiter. After removing the delimiter, the function checks the content length
again. This accepts exactly 1 MiB of content with LF, CRLF, or EOF, while rejecting
one extra content byte. Slice capacity can be larger than its length because of
allocation growth; memory still remains bounded independently of total input.

No extra output buffering is introduced. Each selected line is written before
processing the next line. The input buffer may read ahead; stopping on output
failure does not imply that the underlying source was read one byte at a time.

## Behavior changes from Lesson 1

- LF and CRLF both delimit lines; selected output always ends in LF.
- A final line without a newline receives one if it matches.
- Empty input emits nothing. A blank line matches an empty substring.
- A lone CR is content unless it is immediately followed by LF.
- Oversized lines are rejected even if they would not match the substring.
- A non-EOF read failure discards its incomplete line; previously emitted
  complete lines remain. If the failing read also produces an oversized
  fragment, the read failure takes precedence. EOF, in contrast, establishes
  a valid final line.
- An output failure can leave a partially written line; it cannot be rolled back.
- The reported count measures output bytes, including normalized LF endings.

The CLI returns status 1 and writes diagnostics to stderr for read, write, or
line-limit failures. No matches is successful and produces no output.

`copyStream` and its byte-preservation tests remain available for comparing the
contracts. See `LESSON1.md` for the earlier lesson's notes.

## Exercises

### 1. Predict the line contract

Before running, predict the output for:

```go
input := "ERROR first\r\n\r\nINFO ignored\nERROR final"
contains := "ERROR"
```

Add this as a case to `TestFilterLines`, writing the expected output yourself.
How many output bytes should the function report? Now repeat with an empty
substring: the blank line and INFO line should appear too.

### 2. Make the executable select lines

Temporarily change the final argument in `run` from `""` to `"ERROR"`. Rerun the
three-line PowerShell example: only `ERROR failed` should remain.

The `TestRun` cases currently expect the default empty filter, so changing it
intentionally changes their contract. Restore the empty string after the
experiment. Lesson 3 will make this a flag instead of a source edit.

### 3. Explore boundaries and failures

Read `TestFilterLinesLineLimit` and predict the result for exactly 1 MiB plus CRLF.
Why must the delimiter not reduce the content allowance?

Then compare `TestFilterLinesDataWithEOF` with the interrupted-fragment test in
`TestFilterLinesFailures`. Both involve data accompanying a terminal condition.
Explain why EOF's final line is written while the broken stream's fragment is
discarded. Run these tests and inspect their byte-count assertions.

These are correctness tests, not benchmarks. Measuring allocations and speed
still belongs to Lesson 5.

## Next lesson

Add `-contains`, usage text, argument validation, and exit codes for bad arguments.
