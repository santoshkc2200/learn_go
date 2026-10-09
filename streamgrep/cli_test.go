package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// These tests exercise the real parser and filter together. A new FlagSet for
// each run keeps one invocation's flags from leaking into the next invocation.
func TestCLIArguments(t *testing.T) {
	const input = "INFO ready\r\nERROR failed\r\nERROR retry\nWARN -h\n"
	const all = "INFO ready\nERROR failed\nERROR retry\nWARN -h\n"
	tests := []struct {
		name       string
		args       []string
		wantOutput string
		wantStatus int
		wantError  string
		wantUsage  bool
		noRead     bool
	}{
		{name: "default matches all", wantOutput: all},
		{name: "separate value", args: []string{"-contains", "ERROR"}, wantOutput: "ERROR failed\nERROR retry\n"},
		{name: "equals value", args: []string{"-contains=ERROR"}, wantOutput: "ERROR failed\nERROR retry\n"},
		{name: "double dash flag", args: []string{"--contains", "ERROR"}, wantOutput: "ERROR failed\nERROR retry\n"},
		{name: "value with spaces", args: []string{"-contains", "ERROR failed"}, wantOutput: "ERROR failed\n"},
		{name: "explicit empty", args: []string{"-contains="}, wantOutput: all},
		{name: "no matches succeeds", args: []string{"-contains", "missing"}},
		{name: "hyphen value is not help", args: []string{"-contains", "-h"}, wantOutput: "WARN -h\n"},
		{name: "last repeated value wins", args: []string{"-contains", "WARN", "-contains", "ERROR"}, wantOutput: "ERROR failed\nERROR retry\n"},
		{name: "flag terminator", args: []string{"--"}, wantOutput: all},
		{name: "short help", args: []string{"-h"}, wantUsage: true, noRead: true},
		{name: "long help", args: []string{"--help"}, wantUsage: true, noRead: true},
		{name: "unknown flag", args: []string{"-bogus"}, wantStatus: 2, wantError: "flag provided but not defined", wantUsage: true, noRead: true},
		{name: "missing value", args: []string{"-contains"}, wantStatus: 2, wantError: "flag needs an argument", wantUsage: true, noRead: true},
		{name: "positional before flag", args: []string{"application.log", "-contains", "ERROR"}, wantStatus: 2, wantError: "positional arguments", wantUsage: true, noRead: true},
		{name: "positional after flag", args: []string{"-contains", "ERROR", "application.log"}, wantStatus: 2, wantError: "positional arguments", wantUsage: true, noRead: true},
		{name: "positional after terminator", args: []string{"--", "-h"}, wantStatus: 2, wantError: "positional arguments", wantUsage: true, noRead: true},
		{name: "empty positional", args: []string{""}, wantStatus: 2, wantError: "positional arguments", wantUsage: true, noRead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output, diagnostic bytes.Buffer
			src := &observedReader{Reader: strings.NewReader(input)}
			status := run(tt.args, src, &output, &diagnostic)
			if status != tt.wantStatus || output.String() != tt.wantOutput {
				t.Fatalf("got status %d, stdout %q; want %d, %q", status, output.String(), tt.wantStatus, tt.wantOutput)
			}
			if tt.wantError != "" && !strings.Contains(diagnostic.String(), tt.wantError) {
				t.Fatalf("diagnostic %q does not contain %q", diagnostic.String(), tt.wantError)
			}
			if tt.wantUsage {
				if !strings.Contains(diagnostic.String(), "Usage:") || !strings.Contains(diagnostic.String(), "-contains") {
					t.Fatalf("usage missing from stderr: %q", diagnostic.String())
				}
			} else if diagnostic.Len() != 0 {
				t.Fatalf("unexpected stderr: %q", diagnostic.String())
			}
			// A help request must not wait for a terminal's EOF. Invalid syntax
			// must not consume a piped stream that the program cannot process.
			if tt.noRead && src.calls != 0 {
				t.Fatalf("read stdin %d times before help/validation completed", src.calls)
			}
		})
	}
}

// Embedding io.Reader supplies its method set; overriding Read counts calls.
// It does not expose the underlying reader's optional WriterTo fast path.
type observedReader struct {
	io.Reader
	calls int
}

func (r *observedReader) Read(p []byte) (int, error) {
	r.calls++
	return r.Reader.Read(p)
}

func TestCLIInvalidArgumentsWithFailedStderr(t *testing.T) {
	src := &observedReader{Reader: strings.NewReader("data")}
	status := run([]string{"-bogus"}, src, io.Discard, failingWriter{err: errors.New("stderr closed")})
	if status != 2 || src.calls != 0 {
		t.Fatalf("status %d, reads %d; want status 2 without reading", status, src.calls)
	}
}
