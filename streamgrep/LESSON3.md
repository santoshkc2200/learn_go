# streamgrep — Lesson 3: CLI arguments

These are the Lesson 3 notes. Its CLI commands still apply; `README.md` now
introduces Lesson 4's failure experiments and test helpers.

Select lines from stdin without editing the source:

```powershell
Set-Location C:\Users\user\personal\dev\go\streamgrep
"INFO ready`r`nERROR failed`r`nINFO done" | go run . -contains ERROR
```

On macOS (zsh or bash), adjust the path to your checkout location:

```sh
cd ~/personal/dev/go/streamgrep
printf '%s\n' 'INFO ready' 'ERROR failed' 'INFO done' | go run . -contains ERROR
```

Expected output:

```text
ERROR failed
```

Matching is a case-sensitive literal substring search. Selected lines receive
LF endings; the 1 MiB content limit from Lesson 2 still applies. An empty
substring (the default) selects all lines. No matches is a successful empty result.

## Read the code

1. `run` in `main.go`: the new flag parsing and validation comments.
2. `main`: why it passes `os.Args[1:]` to `run`.
3. `cli_test.go`: actual parser/filter integration and unread-stdin checks.
4. `filter.go`: the existing streaming implementation from Lesson 2.

## Flags, help, and input

```powershell
"ERROR failed`nERROR retry" | go run . -contains "ERROR failed"
"INFO ready`nERROR failed" | go run . -contains=ERROR
"INFO ready`nERROR failed" | go run . --contains ERROR
go run . -h
go run . --help
```

On macOS (zsh or bash):

```sh
printf '%s\n' 'ERROR failed' 'ERROR retry' | go run . -contains "ERROR failed"
printf '%s\n' 'INFO ready' 'ERROR failed' | go run . -contains=ERROR
printf '%s\n' 'INFO ready' 'ERROR failed' | go run . --contains ERROR
go run . -h
go run . --help
```

The first example emits only `ERROR failed`. The shell groups quoted text into
one argument. Go's standard `flag` package accepts a separate string value or
an equals value, and either one or two leading hyphens. `-h` and `-help` (also
`--help`) display usage on stderr and return successfully without reading stdin.
Keeping help off stdout preserves stdout as filtered data only.

To read a file with Windows redirection, build and invoke through `cmd`:

```powershell
go build -o streamgrep.exe .
cmd /c ".\streamgrep.exe -contains ERROR < application.log"
```

On macOS (zsh or bash):

```sh
go build -o streamgrep .
./streamgrep -contains ERROR < application.log
```

Create `application.log` first. The shell opens it; the program has no file-name
arguments. Alternatively, `Get-Content application.log | .\streamgrep.exe -contains ERROR`
uses PowerShell's text pipeline, which chooses encoding and line endings before
Go receives the input.

## Concepts explained in comments

| Concept | Purpose |
| --- | --- |
| `flag.NewFlagSet` | Keep each invocation's parsing state local and testable. |
| `flag.ContinueOnError` | Return parsing errors instead of exiting inside reusable code. |
| A `*string` flag value | Parsing updates the pointed-to value; dereferencing supplies it to the filter. |
| A usage closure | Capture this invocation's flag set and diagnostic writer. |
| `flag.ErrHelp` | Distinguish a successful help request from invalid syntax. |
| `NArg` and `--` | Detect unsupported positional arguments after parsing stops. |
| `os.Args[1:]` | Exclude the executable name from the user arguments. |

Parse and validate before reading any input. An unknown flag, missing value, or
positional argument reports an error without consuming a piped stream. Each
call to `run` creates a new flag set; one test's filter cannot leak into another.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Processing succeeded (including no matches), or help was requested. |
| 1 | Reading, writing, or a line-size check failed. |
| 2 | Invalid command-line arguments. |

Use the built executable when inspecting its actual process status:

```powershell
.\streamgrep.exe -bogus
$LASTEXITCODE
```

On macOS (zsh or bash):

```sh
./streamgrep -bogus
echo $?
```

The result is `2`. By contrast, `go run . -bogus` reports `exit status 2` but the
Go runner itself returns status `1` to the shell. This is a wrapper distinction,
not a different exit-code policy in `streamgrep`.

Help and diagnostics use stderr. If stderr itself cannot be written, the program
still returns the status selected for the request or failure.

## Test and experiment

In PowerShell or macOS Terminal (zsh or bash):

```sh
go test -run TestCLI -v
go test -v ./...
go vet ./...
```

1. **Predict argument grouping.** Compare `-contains "ERROR failed"` with
   `-contains ERROR failed`. The first supplies one value; the second leaves
   `failed` as an unsupported positional argument. Predict output and status,
   then try both with the built executable.
2. **Separate help from a string value.** Compare `-h` with `-contains -h`.
   The latter uses `-h` as the substring because a string flag consumes the next
   argument. Read the corresponding tests and predict which one reads stdin.
3. **Explore the terminator.** Compare `--` with `-- application.log`. The first
   ends option parsing and uses the default empty filter; the second leaves a
   positional argument and fails. Check the observed-reader assertions that
   prove invalid arguments do not read input.

For repeated `-contains` flags, the last value wins. Add a table case to check
that behavior, or change its existing fixture and expected selected lines.

## Previous and next lessons

`LESSON1.md` preserves the byte-copier notes; `LESSON2.md` preserves the line-filter
notes. Their historical examples are labeled. Lesson 4 will explore partial
output and read/write failures more deeply. Performance measurement stays in
Lesson 5.
