package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// 1 << 20 shifts the integer 1 left 20 bits: 1,048,576 bytes, or 1 MiB.
// This bounds line content, not the total input size or number of lines.
const maxLineBytes = 1 << 20

// A sentinel error gives tests and future callers a stable errors.Is target.
var errLineTooLong = errors.New("line content exceeds 1 MiB")

// filterLines is Lesson 2's core. Unlike copyStream, it understands line
// boundaries: matching lines receive LF endings, even if the input used CRLF
// or the final line had no delimiter. Matching is literal and case sensitive.
// The returned count measures output bytes, including the normalized newlines.
func filterLines(dst io.Writer, src io.Reader, contains string) (int64, error) {
	// ReadString could accumulate an arbitrarily long line before we check it.
	// ReadSlice returns fragments from a bounded buffer instead. It reports
	// ErrBufferFull when a line continues beyond the buffer; that is a signal
	// to gather another fragment, not a failed input stream.
	reader := bufio.NewReaderSize(src, 4096)
	needle := []byte(contains)
	var line []byte
	var written int64
	for {
		fragment, readErr := reader.ReadSlice('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, bufio.ErrBufferFull) {
			// A real read failure is different from EOF. Keep already written
			// complete lines, but discard the interrupted fragment. Check this
			// before its size: even an oversized fragment must not hide the
			// source failure. Wrapping retains the original error's identity.
			return written, fmt.Errorf("read line: %w", readErr)
		}
		// Allow two extra bytes for CRLF while assembling the raw line. Check
		// BEFORE appending, so a source with no newline cannot grow line forever.
		if len(line)+len(fragment) > maxLineBytes+2 {
			return written, fmt.Errorf("filter lines: %w", errLineTooLong)
		}
		// ReadSlice's returned slice aliases its internal buffer and expires
		// at the next read. append copies those bytes into our own storage.
		line = append(line, fragment...)
		if errors.Is(readErr, bufio.ErrBufferFull) {
			continue
		}
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			return written, nil // Empty input, or EOF after a trailing newline.
		}

		// Only CR immediately before LF is part of a CRLF delimiter. A lone
		// CR elsewhere (including at EOF) is content and must be preserved.
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
		}
		if len(line) > maxLineBytes {
			return written, fmt.Errorf("filter lines: %w", errLineTooLong)
		}
		// Contains performs byte substring matching, not regular expressions.
		// An empty needle matches every line, including an empty one.
		if bytes.Contains(line, needle) {
			line = append(line, '\n')
			// Lesson 5's allocation profile found that creating a bytes.Reader
			// for every matched line allocated a heap object. We already own the
			// whole line, so write its slice directly without that extra reader.
			n, writeErr := dst.Write(line)
			written += int64(n) // Count accepted bytes even when Write also fails.
			// io.Copy used to detect this Writer contract violation for us.
			// Preserve its behavior explicitly: a short write needs an error.
			if writeErr == nil && n != len(line) {
				writeErr = io.ErrShortWrite
			}
			if writeErr != nil {
				return written, fmt.Errorf("write line: %w", writeErr)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil // The final unterminated line has been processed.
		}
		// Reset length while keeping capacity: reuse storage for the next line.
		// That capacity may exceed the content limit due to append's growth,
		// but it remains bounded independently of the whole stream's size.
		line = line[:0]
	}
}
