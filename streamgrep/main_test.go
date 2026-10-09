package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// A table-driven test gives the same contract several deliberately different
// inputs. Changing the copier to normalize newlines or treat bytes as runes
// would break these cases: the contract here is exact byte preservation.
func TestCopyStreamPreservesBytes(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "empty"},
		{name: "text", input: []byte("hello Go\n")},
		{name: "UTF-8", input: []byte("こんにちは Go 🌱\n")},
		{name: "CRLF", input: []byte("first\r\nsecond\r\n")},
		{name: "zero bytes", input: []byte{'a', 0, 'b', 0}},
		{name: "no final newline", input: []byte("last line")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// bytes.Buffer's zero value is ready to use. Its Write method means
			// *bytes.Buffer satisfies io.Writer without an implements declaration.
			var dst bytes.Buffer
			n, err := copyStream(&dst, bytes.NewReader(tt.input))
			if err != nil {
				t.Fatalf("copyStream: %v", err)
			}
			if n != int64(len(tt.input)) {
				t.Fatalf("copied %d bytes, want %d", n, len(tt.input))
			}
			// These are independent input/output buffers, so equality checks
			// what the copier actually wrote, not a shared object's identity.
			if !bytes.Equal(dst.Bytes(), tt.input) {
				t.Fatalf("output %q, want %q", dst.Bytes(), tt.input)
			}
		})
	}
}

// Inject failures at the I/O boundary, while keeping the real copier running.
// This catches returning success on failure and replacing an error with text
// that loses its identity. errors.Is follows a chain of wrapped errors.
func TestCopyStreamWrapsFailures(t *testing.T) {
	injected := errors.New("device unavailable")
	tests := []struct {
		name string
		src  io.Reader
		dst  io.Writer
	}{
		{name: "read", src: failingReader{err: injected}, dst: io.Discard},
		{name: "write", src: bytes.NewReader([]byte("data")), dst: failingWriter{err: injected}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := copyStream(tt.dst, tt.src)
			if n != 0 {
				t.Fatalf("copied %d bytes after immediate failure, want 0", n)
			}
			if !errors.Is(err, injected) {
				t.Fatalf("error %v does not preserve %v", err, injected)
			}
			if err == injected {
				t.Fatal("returned raw error, want added operation context")
			}
		})
	}
}

// Value receivers suffice here: neither helper changes its own state.
// These types need only one method each to satisfy their respective interfaces.
type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

// These contract tests document edge cases delegated to io.Copy. They protect
// us if a later lesson replaces it with a manual loop that mishandles (n, err).
func TestCopyStreamDataWithEOF(t *testing.T) {
	var dst bytes.Buffer
	src := iotest.DataErrReader(strings.NewReader("final bytes"))
	n, err := copyStream(&dst, src)
	if err != nil || n != 11 || dst.String() != "final bytes" {
		t.Fatalf("got (%d, %v, %q), want (11, nil, final bytes)", n, err, dst.String())
	}
}

func TestCopyStreamPartialReadFailure(t *testing.T) {
	injected := errors.New("read interrupted")
	var dst bytes.Buffer
	n, err := copyStream(&dst, &dataErrorReader{err: injected})
	// The Reader contract permits useful data AND an error in the same call.
	// Ignoring n whenever err != nil would silently drop these three bytes.
	if n != 3 || dst.String() != "abc" || !errors.Is(err, injected) {
		t.Fatalf("got (%d, %v, %q), want (3, wrapped read error, abc)", n, err, dst.String())
	}
}

type dataErrorReader struct {
	err  error
	done bool
}

func (r *dataErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	// Pointer receiver: changing done must change the original helper, not a
	// copy. The copier supplies enough space for this short test payload.
	r.done = true
	return copy(p, "abc"), r.err
}

func TestCopyStreamShortWrite(t *testing.T) {
	n, err := copyStream(shortWriter{}, strings.NewReader("abc"))
	if n != 2 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("got (%d, %v), want (2, wrapped io.ErrShortWrite)", n, err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	// Deliberately violates the Writer contract: fewer bytes than len(p)
	// requires a non-nil error. io.Copy detects and reports the short write.
	return len(p) - 1, nil
}

func TestCopyStreamLargeGeneratedInput(t *testing.T) {
	const size = 2 * 1024 * 1024
	src := &generatedReader{remaining: size}
	dst := &validatingWriter{}
	n, err := copyStream(dst, src)
	if err != nil || n != size || dst.total != size {
		t.Fatalf("copied %d, received %d, error %v; want %d bytes", n, dst.total, err, size)
	}
	if src.remaining != 0 || !src.sawEOF || src.calls <= 1 || dst.calls <= 1 {
		t.Fatalf("stream not fully consumed in chunks: reader %+v, writer %+v", src, dst)
	}
}

// Only Read is implemented, so this exercises io.Copy's generic buffer path.
// Generate a large logical input without a correspondingly large allocation.
type generatedReader struct {
	remaining int
	calls     int
	sawEOF    bool
}

func (r *generatedReader) Read(p []byte) (int, error) {
	r.calls++
	if r.remaining == 0 {
		r.sawEOF = true
		return 0, io.EOF
	}
	n := min(len(p), r.remaining, 4096)
	for i := range p[:n] {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}

type validatingWriter struct {
	total int64
	calls int
}

func (w *validatingWriter) Write(p []byte) (int, error) {
	w.calls++
	for _, b := range p {
		if b != 'x' {
			return 0, errors.New("unexpected byte in generated stream")
		}
	}
	w.total += int64(len(p))
	// Do not retain p: a copier can reuse the same slice on its next read.
	return len(p), nil
}

func TestCopyStreamStopsAfterWriteFailure(t *testing.T) {
	injected := errors.New("output disconnected")
	src := &endlessReader{}
	n, err := copyStream(failingWriter{err: injected}, src)
	if n != 0 || !errors.Is(err, injected) || src.calls != 1 {
		t.Fatalf("got count %d, error %v, reads %d; want 0, output error, 1", n, err, src.calls)
	}
}

type endlessReader struct{ calls int }

func (r *endlessReader) Read(p []byte) (int, error) {
	r.calls++
	// The logical source never ends, but this guard lets a broken copier fail
	// the test instead of hanging forever. No timing-dependent assertions.
	if r.calls > 4 {
		return 0, errors.New("copier kept reading after output failure")
	}
	p[0] = 'x'
	return 1, nil
}

// run is tested without starting a process: substituting streams is ordinary
// interface use. These tests catch mixing diagnostics into stdout, losing
// partial output, and reporting success after a transfer failure.
func TestRun(t *testing.T) {
	injected := errors.New("device unavailable")
	tests := []struct {
		name           string
		src            io.Reader
		failOutput     bool
		failDiagnostic bool
		wantOutput     string
		wantStatus     int
	}{
		{name: "normalize CRLF", src: strings.NewReader("hello\r\n"), wantOutput: "hello\n"},
		{name: "final unterminated line", src: strings.NewReader("hello"), wantOutput: "hello\n"},
		{name: "empty", src: strings.NewReader("")},
		{name: "read failure", src: failingReader{err: injected}, wantStatus: 1},
		{name: "write failure", src: strings.NewReader("data"), failOutput: true, wantStatus: 1},
		{name: "discard interrupted line", src: &dataErrorReader{err: injected}, wantStatus: 1},
		{name: "keep complete lines", src: &terminalErrorReader{data: strings.NewReader("ERROR done\nERROR broken"), err: injected}, wantOutput: "ERROR done\n", wantStatus: 1},
		{name: "diagnostic failure", src: failingReader{err: injected}, failDiagnostic: true, wantStatus: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output, diagnostic bytes.Buffer
			// Interface variables can hold different concrete stream types.
			// Only the failing cases replace the real in-memory destination.
			var stdout io.Writer = &output
			var stderr io.Writer = &diagnostic
			if tt.failOutput {
				stdout = failingWriter{err: injected}
			}
			if tt.failDiagnostic {
				stderr = failingWriter{err: errors.New("stderr closed")}
			}
			if status := run(nil, tt.src, stdout, stderr); status != tt.wantStatus {
				t.Fatalf("status %d, want %d", status, tt.wantStatus)
			}
			if output.String() != tt.wantOutput {
				t.Fatalf("stdout %q, want %q", output.String(), tt.wantOutput)
			}
			if tt.wantStatus == 0 || tt.failDiagnostic {
				if diagnostic.Len() != 0 {
					t.Fatalf("unexpected diagnostic %q", diagnostic.String())
				}
			} else if !strings.HasPrefix(diagnostic.String(), "streamgrep:") ||
				!strings.Contains(diagnostic.String(), injected.Error()) {
				t.Fatalf("diagnostic lacks program name or cause: %q", diagnostic.String())
			}
		})
	}
}
