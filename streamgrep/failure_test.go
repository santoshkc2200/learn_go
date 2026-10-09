package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Lesson 4: a Writer can return n > 0 AND an error. Those n bytes have already
// escaped to the destination; returning an error cannot take them back.
// These cases place failure inside a line and exactly between complete lines.
func TestFailurePartialWrite(t *testing.T) {
	const input = "INFO skip\nERROR one\r\nERROR two\nERROR three\n"
	injected := errors.New("destination full")
	tests := []struct {
		limit int
		want  string
		calls int
	}{
		{limit: 0, want: "", calls: 1},
		{limit: 4, want: "ERRO", calls: 1},
		{limit: 10, want: "ERROR one\n", calls: 2},
		{limit: 14, want: "ERROR one\nERRO", calls: 2},
		{limit: 20, want: "ERROR one\nERROR two\n", calls: 3},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.limit), func(t *testing.T) {
			writer := &limitedWriter{remaining: tt.limit, err: injected}
			n, err := filterLines(writer, strings.NewReader(input), "ERROR")
			// Assert observable bytes, not just how often our helper was called.
			// n counts successful output bytes, including earlier complete lines.
			if writer.output.String() != tt.want || n != int64(len(tt.want)) {
				t.Fatalf("count %d, output %q; want %d, %q", n, writer.output.String(), len(tt.want), tt.want)
			}
			if !errors.Is(err, injected) || err == injected {
				t.Fatalf("error %v must wrap and retain destination failure", err)
			}
			// No retry and no write of later lines after failure. This is a
			// deterministic check, independent of scheduling or wall-clock time.
			if writer.calls != tt.calls {
				t.Fatalf("writes %d, want %d; processing continued after failure", writer.calls, tt.calls)
			}

			// The CLI should report failure without inserting its diagnostic
			// into the partially written stdout. Reset the writer for a fresh run.
			writer = &limitedWriter{remaining: tt.limit, err: injected}
			var diagnostic bytes.Buffer
			status := run([]string{"-contains", "ERROR"}, strings.NewReader(input), writer, &diagnostic)
			if status != 1 || writer.output.String() != tt.want || !strings.Contains(diagnostic.String(), injected.Error()) {
				t.Fatalf("status %d, stdout %q, stderr %q", status, writer.output.String(), diagnostic.String())
			}
		})
	}
}

// Implement just Write. The buffer is a field, rather than embedded, so optional
// methods such as ReaderFrom cannot bypass this deliberate failure boundary.
type limitedWriter struct {
	remaining int
	err       error
	output    bytes.Buffer
	calls     int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.calls++
	n := min(len(p), w.remaining)
	_, _ = w.output.Write(p[:n]) // bytes.Buffer.Write always accepts these bytes.
	w.remaining -= n
	if n < len(p) {
		// Unlike shortWriter in Lesson 1, this satisfies the Writer contract:
		// when fewer than len(p) bytes are accepted, return a non-nil error.
		return n, w.err
	}
	return n, nil
}

func TestFailureReadErrorIdentity(t *testing.T) {
	lowLevel := errors.New("device disconnected")
	// A typed error carries context plus a cause. PathError's Unwrap method
	// exposes Err, so a wrapping chain can preserve both metadata and identity.
	sourceError := &os.PathError{Op: "read", Path: "lesson-stream", Err: lowLevel}
	for _, tt := range []struct {
		name, input, want string
	}{
		{name: "complete matching line before interruption", input: "ERROR one\nINFO done\nERROR broken", want: "ERROR one\n"},
		{name: "failure even without matches", input: "INFO complete\nINFO broken", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			src := &terminalErrorReader{data: strings.NewReader(tt.input), err: sourceError}
			n, err := filterLines(&output, src, "ERROR")
			if output.String() != tt.want || n != int64(len(tt.want)) {
				t.Fatalf("count %d, output %q; want %d, %q", n, output.String(), len(tt.want), tt.want)
			}
			// errors.Is asks whether the chain contains this specific cause.
			// Matching messages is not enough: a new error with the same text
			// is still a different error value.
			if !errors.Is(err, sourceError) || !errors.Is(err, lowLevel) || errors.Is(err, errors.New(lowLevel.Error())) {
				t.Fatalf("error chain lost identity: %v", err)
			}
			// errors.As asks whether a typed error exists in that chain.
			// Pass &pathError (a **os.PathError) so As can set our pointer variable.
			var pathError *os.PathError
			if !errors.As(err, &pathError) || pathError != sourceError {
				t.Fatalf("typed source context missing: %v", err)
			}
			if err == sourceError {
				t.Fatal("filter must add operation context around the source error")
			}
			// One more application layer can add context without breaking Is/As.
			outer := fmt.Errorf("import log: %w", err)
			if !errors.Is(outer, lowLevel) || !errors.As(outer, &pathError) {
				t.Fatalf("outer context lost the cause: %v", outer)
			}

			var diagnostic bytes.Buffer
			output.Reset()
			src = &terminalErrorReader{data: strings.NewReader(tt.input), err: sourceError}
			if status := run([]string{"-contains", "ERROR"}, src, &output, &diagnostic); status != 1 || output.String() != tt.want {
				t.Fatalf("status %d, output %q; want failed run with preserved complete lines", status, output.String())
			}
			if !strings.Contains(diagnostic.String(), "read line") || !strings.Contains(diagnostic.String(), "lesson-stream") {
				t.Fatalf("diagnostic lacks operation/source context: %q", diagnostic.String())
			}
		})
	}
}

func TestFailureLineLimitPreservesOutput(t *testing.T) {
	// Even a nonmatching oversized line must stop processing. Earlier emitted
	// lines remain, and a matching line after the failure must not appear.
	input := "ERROR one\n" + strings.Repeat("x", (1<<20)+1) + "\nERROR later\n"
	var output bytes.Buffer
	n, err := filterLines(&output, strings.NewReader(input), "ERROR")
	if n != 10 || output.String() != "ERROR one\n" || !errors.Is(err, errLineTooLong) {
		t.Fatalf("count %d, error %v, output %q; want retained prefix and line limit error", n, err, output.String())
	}
}

func TestFailureRetryDuplicatesOutput(t *testing.T) {
	const input = "ERROR one\nERROR two\n"
	writer := &limitedWriter{remaining: 14, err: errors.New("destination full")}
	n, err := filterLines(writer, strings.NewReader(input), "ERROR")
	if n != 14 || err == nil || writer.output.String() != "ERROR one\nERRO" {
		t.Fatalf("unexpected first attempt: count %d, error %v, output %q", n, err, writer.output.String())
	}
	// Deliberately simulate a naive retry: restart the input, retain the old
	// output, and make the destination writable again. This is an experiment,
	// not a retry feature. io.Writer offers no general rollback operation.
	n, err = filterLines(&writer.output, strings.NewReader(input), "ERROR")
	const want = "ERROR one\nERROERROR one\nERROR two\n"
	if err != nil || n != 20 || writer.output.String() != want {
		t.Fatalf("retry count %d, error %v, output %q; want 20 new bytes and %q", n, err, writer.output.String(), want)
	}
}

func TestFailureBothOutputAndDiagnostic(t *testing.T) {
	writer := &limitedWriter{remaining: 14, err: errors.New("stdout disconnected")}
	diagnostic := failingWriter{err: errors.New("stderr disconnected")}
	status := run([]string{"-contains", "ERROR"}, strings.NewReader("ERROR one\nERROR two\n"), writer, diagnostic)
	// Diagnostics are a second I/O operation and can fail too. That failure
	// must neither turn the original transfer into success nor undo its bytes.
	if status != 1 || writer.output.String() != "ERROR one\nERRO" || writer.calls != 2 {
		t.Fatalf("status %d, output %q, writes %d; want failed transfer with retained prefix", status, writer.output.String(), writer.calls)
	}
}
