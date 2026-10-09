// Package main builds an executable. Its main function is the process entry
// point; functions in an ordinary library package would be called by other code.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// copyStream is Lesson 1's reusable core. Start reading here.
//
// io.Reader requires Read([]byte) (int, error); io.Writer requires
// Write([]byte) (int, error). Go checks these method sets implicitly: files,
// memory buffers, and our test streams all fit without naming an interface
// in their type declarations. Accepting these small interfaces separates
// the operation from the source of the bytes and makes failures testable.
//
// The caller owns the streams. This function neither opens nor closes them.
func copyStream(dst io.Writer, src io.Reader) (int64, error) {
	// := declares n and err with inferred types. A function can return several
	// values: n is the number of bytes successfully written (int64), and err
	// says why copying stopped. Bytes can be transferred even when err != nil.
	//
	// Copy moves bytes incrementally rather than using io.ReadAll. It does not
	// decode Unicode, normalize CRLF, or wait for a whole file before writing.
	// No goroutine is needed: read a chunk, write it, repeat. A slow writer
	// naturally delays subsequent reads instead of growing a queued backlog.
	//
	// io.Copy can use a source's WriterTo or a destination's ReaderFrom method.
	// Otherwise it uses a bounded buffer. Those fast paths and the chosen
	// streams matter: bytes.Buffer as a destination still retains ALL output.
	// Streaming describes the data flow, not a guarantee about every writer.
	n, err := io.Copy(dst, src)
	if err != nil {
		// %w wraps an error instead of replacing it with a string. A caller
		// can add context at each layer and still use errors.Is to identify
		// the original cause. Preserve n: output already written is not undone.
		return n, fmt.Errorf("copy stream: %w", err)
	}

	// A Reader may return data and io.EOF together. Copy writes that data
	// before finishing and treats EOF as normal completion, returning nil.
	// nil here means "no error"; it does not mean that no bytes were copied.
	return n, nil
}

// run translates the reusable operation into CLI behavior. Supplying arguments
// and streams avoids changing process globals when tests exercise the CLI.
func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	// A FlagSet owns one invocation's options. Package-level flag.String/Parse
	// would use shared global state, which is harder to test repeatedly.
	// ContinueOnError returns parse failures instead of calling os.Exit, so
	// this function can choose a status and main stays the only process exit.
	flags := flag.NewFlagSet("streamgrep", flag.ContinueOnError)
	flags.SetOutput(stderr)
	// String returns *string: Parse updates the value behind this pointer.
	// The empty default matches all lines, preserving Lesson 2's default.
	contains := flags.String("contains", "", "emit lines containing this case-sensitive literal substring")
	flags.Usage = func() {
		// This closure captures flags and stderr from this run call. Help and
		// diagnostics go to stderr; stdout remains only filtered input data.
		_, _ = fmt.Fprintln(stderr, "Usage: streamgrep [-contains text] < input")
		_, _ = fmt.Fprintln(stderr, "Read stdin; emit matching lines with LF endings (1 MiB content limit per line).")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		// The flag package prints usage for -h/-help and returns ErrHelp.
		// Help is successful and must not wait for input. Other parse failures
		// have already printed diagnostics/usage and represent invalid syntax.
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	// Go's parser stops at the first non-flag argument or at --. NArg counts
	// the remaining positional arguments; our program accepts none because
	// the shell opens files through redirection. Validate before reading stdin.
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "streamgrep: positional arguments are not supported; read input from stdin")
		flags.Usage()
		return 2
	}
	// _ is the blank identifier: intentionally discard the byte count here.
	// This CLI emits only selected lines; reporting a count on stdout would
	// corrupt a pipeline. The filterLines tests still verify that count.
	// Dereference *contains to pass the string value, rather than its pointer.
	_, err := filterLines(stdout, stdin, *contains)
	if err != nil {
		// Fprintln writes a human-readable diagnostic plus a newline. Keep it
		// on stderr so another command can safely consume stdout as data.
		// If stderr also fails, there is no reliable diagnostic destination;
		// still report failure to the shell through the exit status.
		_, _ = fmt.Fprintln(stderr, "streamgrep:", err)
		return 1
	}
	// Conventional CLI status: 0 succeeds, nonzero fails.
	return 0
}

func main() {
	// *os.File implements the I/O interfaces. These process streams belong
	// to the runtime; no file-opening logic or extra buffers are needed here.
	// os.Exit skips deferred functions. Any future owned-resource cleanup
	// should happen inside run before it returns, not in a defer in main.
	// os.Args[0] is the executable name. The slice [1:] passes only the user's
	// arguments; shell quotes have already grouped values such as "ERROR failed".
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
