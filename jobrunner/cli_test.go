package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCLIArguments(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"help", []string{"-h"}, 0},
		{"unknown", []string{"-missing"}, 2},
		{"positional", []string{"extra"}, 2},
		{"zero workers", []string{"-workers", "0"}, 2},
		{"negative workers", []string{"-workers", "-1"}, 2},
		{"negative jobs", []string{"-jobs", "-1"}, 2},
		{"negative duration", []string{"-duration", "-1s"}, 2},
		{"zero timeout", []string{"-timeout", "0"}, 2},
		{"negative timeout", []string{"-timeout", "-1s"}, 2},
		{"malformed duration", []string{"-duration", "soon"}, 2},
		{"zero jobs", []string{"-jobs", "0"}, 0},
		{"instant jobs", []string{"-jobs", "3", "-duration", "0"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, diagnostic bytes.Buffer
			if got := runCLI(context.Background(), tc.args, &out, &diagnostic); got != tc.want {
				t.Fatalf("code=%d want=%d stderr=%q", got, tc.want, diagnostic.String())
			}
			if tc.want == 2 && diagnostic.Len() == 0 {
				t.Fatal("invalid arguments need a diagnostic")
			}
			if tc.name == "help" && !strings.Contains(out.String(), "-workers") {
				t.Fatal("missing usage")
			}
		})
	}
}

func TestCLISummary(t *testing.T) {
	var out, diagnostic bytes.Buffer
	code := runCLI(context.Background(), []string{"-jobs", "3", "-duration", "0"}, &out, &diagnostic)
	if code != 0 || out.String() != "job 0: succeeded\njob 1: succeeded\njob 2: succeeded\n" || diagnostic.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), diagnostic.String())
	}
}

func TestCLICancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, diagnostic bytes.Buffer
	code := runCLI(ctx, []string{"-jobs", "2"}, &out, &diagnostic)
	if code != 1 || out.String() != "job 0: canceled (not started)\njob 1: canceled (not started)\n" || !strings.Contains(diagnostic.String(), "context canceled") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), diagnostic.String())
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestCLIOutputFailure(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"-jobs", "1", "-duration", "0"}} {
		for _, writer := range []interface{ Write([]byte) (int, error) }{failingWriter{errors.New("disk full")}, shortWriter{}} {
			var diagnostic bytes.Buffer
			if code := runCLI(context.Background(), args, writer, &diagnostic); code != 1 || diagnostic.Len() == 0 {
				t.Fatalf("code=%d stderr=%q", code, diagnostic.String())
			}
		}
	}
}

func TestDelayJobCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- (delayJob{duration: time.Hour}).Run(ctx) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timer job ignored cancellation")
	}
}

func TestCLIJobDeadline(t *testing.T) {
	var out, diagnostic bytes.Buffer
	code := runCLI(context.Background(), []string{"-jobs", "1", "-duration", "1s", "-job-timeout", "10ms"}, &out, &diagnostic)
	if code != 1 || !strings.Contains(out.String(), "canceled (started)") || diagnostic.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), diagnostic.String())
	}
}

func TestCLIRetries(t *testing.T) {
	var out, diagnostic bytes.Buffer
	code := runCLI(context.Background(), []string{"-jobs", "2", "-duration", "0", "-fail-first", "2", "-attempts", "3", "-backoff", "0"}, &out, &diagnostic)
	if code != 0 || out.String() != "job 0: succeeded (attempts=3)\njob 1: succeeded (attempts=3)\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), diagnostic.String())
	}
}

func TestCLIStreaming(t *testing.T) {
	var out, diagnostic bytes.Buffer
	code := runCLI(context.Background(), []string{"-stream", "-jobs", "3", "-duration", "0", "-workers", "1", "-interval", "1ms"}, &out, &diagnostic)
	if code != 0 || out.String() != "job 0: succeeded\njob 1: succeeded\njob 2: succeeded\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), diagnostic.String())
	}
	diagnostic.Reset()
	if code := runCLI(context.Background(), []string{"-stream", "-jobs", "6", "-duration", "0"}, failingWriter{errors.New("broken pipe")}, &diagnostic); code != 1 {
		t.Fatalf("failed stream write code=%d", code)
	}
}
