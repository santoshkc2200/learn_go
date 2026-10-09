package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// Lesson 2 changes the contract from copying bytes to selecting logical lines.
// Literal expected output catches newline normalization and matching mistakes.
func TestFilterLines(t *testing.T) {
	tests := []struct {
		name, input, contains, want string
	}{
		{name: "empty input"},
		{name: "matching lines", input: "INFO ready\nERROR failed\nINFO done\nERROR retry\n", contains: "ERROR", want: "ERROR failed\nERROR retry\n"},
		{name: "case sensitive", input: "error\nERROR\n", contains: "ERROR", want: "ERROR\n"},
		{name: "literal substring", input: "a.b\naxb\n", contains: ".", want: "a.b\n"},
		{name: "no matches", input: "INFO ready\n", contains: "ERROR"},
		{name: "empty filter", input: "first\n\nlast", want: "first\n\nlast\n"},
		{name: "CRLF", input: "INFO ready\r\nERROR failed\r\n", contains: "ERROR", want: "ERROR failed\n"},
		{name: "final matching line", input: "INFO ready\nERROR final", contains: "ERROR", want: "ERROR final\n"},
		{name: "final nonmatching line", input: "ERROR first\nINFO final", contains: "ERROR", want: "ERROR first\n"},
		{name: "blank nonmatching", input: "\n\r\n", contains: "ERROR"},
		{name: "Unicode and zero byte", input: "こんにちは\x00Go\nbye\n", contains: "Go", want: "こんにちは\x00Go\n"},
		{name: "lone CR is content", input: "a\rb\nlast\r", want: "a\rb\nlast\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			n, err := filterLines(&output, strings.NewReader(tt.input), tt.contains)
			if err != nil || output.String() != tt.want || n != int64(len(tt.want)) {
				t.Fatalf("got (%d, %v, %q), want (%d, nil, %q)", n, err, output.String(), len(tt.want), tt.want)
			}
		})
	}
}

func TestFilterLinesDataWithEOF(t *testing.T) {
	var output bytes.Buffer
	n, err := filterLines(&output, iotest.DataErrReader(strings.NewReader("ERROR final")), "ERROR")
	if err != nil || n != 12 || output.String() != "ERROR final\n" {
		t.Fatalf("got (%d, %v, %q), want (12, nil, ERROR final\\n)", n, err, output.String())
	}
}

func TestFilterLinesLineLimit(t *testing.T) {
	// A byte limit is different from a rune limit. CRLF is two delimiter bytes
	// and must not subtract from the permitted 1 MiB of content.
	for _, ending := range []string{"\n", "\r\n", ""} {
		for _, size := range []int{(1 << 20) - 1, 1 << 20, (1 << 20) + 1} {
			name := fmtLineCase(size, ending)
			t.Run(name, func(t *testing.T) {
				content := strings.Repeat("x", size)
				var output bytes.Buffer
				n, err := filterLines(&output, strings.NewReader(content+ending), "")
				if size > 1<<20 {
					if !errors.Is(err, errLineTooLong) || n != 0 || output.Len() != 0 {
						t.Fatalf("oversized line: count %d, error %v, output size %d", n, err, output.Len())
					}
				} else if err != nil || n != int64(size+1) || output.String() != content+"\n" {
					t.Fatalf("permitted line: count %d, error %v, output size %d; want %d", n, err, output.Len(), size+1)
				}
			})
		}
	}
}

func fmtLineCase(size int, ending string) string {
	// Keep the table's names readable without affecting the assertions.
	label := "EOF"
	if ending == "\n" {
		label = "LF"
	}
	if ending == "\r\n" {
		label = "CRLF"
	}
	return fmt.Sprintf("%d/%s", size, label)
}

func TestFilterLinesRejectsOversizedNonmatchingLine(t *testing.T) {
	n, err := filterLines(io.Discard, strings.NewReader(strings.Repeat("x", (1<<20)+1)), "ERROR")
	if n != 0 || !errors.Is(err, errLineTooLong) {
		t.Fatalf("got (%d, %v), want (0, line limit error)", n, err)
	}
}

func TestFilterLinesFailures(t *testing.T) {
	injected := errors.New("stream interrupted")
	t.Run("read before data", func(t *testing.T) {
		n, err := filterLines(io.Discard, failingReader{err: injected}, "")
		if n != 0 || !errors.Is(err, injected) {
			t.Fatalf("got (%d, %v), want read failure", n, err)
		}
	})
	t.Run("keep complete lines discard interrupted fragment", func(t *testing.T) {
		var output bytes.Buffer
		src := &terminalErrorReader{data: strings.NewReader("ERROR done\nERROR broken"), err: injected}
		n, err := filterLines(&output, src, "ERROR")
		if n != 11 || output.String() != "ERROR done\n" || !errors.Is(err, injected) {
			t.Fatalf("got (%d, %v, %q), want complete line and read failure", n, err, output.String())
		}
	})
	t.Run("write stops further processing", func(t *testing.T) {
		tail := &endlessReader{}
		src := io.MultiReader(strings.NewReader("ERROR first\n"), tail)
		n, err := filterLines(failingWriter{err: injected}, src, "ERROR")
		// Buffered reading may read ahead in general. This source deliberately
		// returns the first line on its own, letting us detect continued reads.
		if n != 0 || !errors.Is(err, injected) || tail.calls != 0 {
			t.Fatalf("got (%d, %v), tail reads %d; want immediate write failure", n, err, tail.calls)
		}
	})
	t.Run("short write", func(t *testing.T) {
		n, err := filterLines(shortWriter{}, strings.NewReader("abc\n"), "")
		if n != 3 || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("got (%d, %v), want (3, short write error)", n, err)
		}
	})
	t.Run("oversized stream stops early", func(t *testing.T) {
		src := &generatedReader{remaining: 4 << 20}
		n, err := filterLines(io.Discard, src, "")
		if n != 0 || !errors.Is(err, errLineTooLong) || src.remaining == 0 {
			t.Fatalf("got (%d, %v), remaining %d; want early line limit failure", n, err, src.remaining)
		}
	})
}

type terminalErrorReader struct {
	data *strings.Reader
	err  error
}

func (r *terminalErrorReader) Read(p []byte) (int, error) {
	n, err := r.data.Read(p)
	if r.data.Len() == 0 {
		// Simulate a stream breaking with data in the same read. A newline
		// already received is reliable; a fragment cut off by failure is not.
		return n, r.err
	}
	return n, err
}

func TestRunRejectsOversizedLine(t *testing.T) {
	var output, diagnostic bytes.Buffer
	status := run(nil, strings.NewReader(strings.Repeat("x", (1<<20)+1)), &output, &diagnostic)
	if status != 1 || output.Len() != 0 || !strings.Contains(diagnostic.String(), "1 MiB") {
		t.Fatalf("status %d, output size %d, diagnostic %q; want line limit failure", status, output.Len(), diagnostic.String())
	}
}

func TestFilterLinesBufferBoundary(t *testing.T) {
	// Place CR at the last byte of the read buffer and LF in the next read.
	// The delimiter belongs to one logical line even across read boundaries.
	content := strings.Repeat("x", 4095)
	var output bytes.Buffer
	n, err := filterLines(&output, strings.NewReader(content+"\r\nnext\n"), "")
	if err != nil || n != 4101 || output.String() != content+"\nnext\n" {
		t.Fatalf("count %d, error %v, output size %d; want 4101", n, err, output.Len())
	}
}

func TestFilterLinesReadFailureAtLimit(t *testing.T) {
	// A real read failure means this fragment is incomplete, even when it
	// also exceeds the line limit. Preserve its cause instead of replacing
	// it with a size error based on an interrupted line.
	injected := errors.New("connection lost at line boundary")
	for _, size := range []int{(1 << 20) - 1, (1 << 20) + 1, (1 << 20) + 3} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var output bytes.Buffer
			src := &terminalErrorReader{data: strings.NewReader(strings.Repeat("x", size)), err: injected}
			n, err := filterLines(&output, src, "")
			if n != 0 || output.Len() != 0 || !errors.Is(err, injected) {
				t.Fatalf("count %d, output size %d, error %v; want original read failure", n, output.Len(), err)
			}
		})
	}
}
