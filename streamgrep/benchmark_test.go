package main

import (
	"bytes"
	"io"
	"testing"
)

// Pin the benchmark fixture to literal, hand-counted data. A wrong fixture
// would make throughput or matching-rate labels misleading even if it ran fast.
func TestBenchmarkInput(t *testing.T) {
	input, matchingBytes := makeBenchmarkInput(32, 8)
	const want = "ERROR x\nINFO  x\nERROR x\nINFO  x\n"
	if string(input) != want || matchingBytes != 16 {
		t.Fatalf("fixture %q, matching bytes %d; want %q and 16", input, matchingBytes, want)
	}
}

// A measured optimization target: with the same 128-byte line shape, scaling
// input 16 times should not allocate a new heap object for every matched line.
// Compare growth rather than asserting an exact compiler-dependent count.
func TestFilterLinesAllocationGrowth(t *testing.T) {
	measure := func(size int) float64 {
		input, _ := makeBenchmarkInput(size, 128)
		var reader bytes.Reader
		return testing.AllocsPerRun(10, func() {
			reader.Reset(input)
			n, err := filterLines(io.Discard, &reader, "")
			if err != nil || n != int64(len(input)) {
				t.Fatalf("count %d, error %v; want %d", n, err, len(input))
			}
		})
	}
	small := measure(4 << 10)
	large := measure(64 << 10)
	t.Logf("allocations per run: 4 KiB = %.0f, 64 KiB = %.0f", small, large)
	// Leave room for fixed overhead variation; a per-line allocation adds
	// 480 objects here, far beyond this tolerance. Fixture setup is excluded.
	if large > small+8 {
		t.Fatalf("allocations grew from %.0f to %.0f for same-shaped lines; want bounded growth", small, large)
	}
}

func makeBenchmarkInput(size, lineBytes int) ([]byte, int64) {
	// The fixtures use an even number of equally sized LF-terminated lines.
	// Half contain ERROR; the other half use a same-length INFO prefix.
	// Guard setup choices so an edited benchmark cannot silently mislabel data.
	if lineBytes < 7 || size <= 0 || size%lineBytes != 0 || (size/lineBytes)%2 != 0 {
		panic("benchmark fixture requires positive size and even complete lines of at least 7 bytes")
	}
	input := bytes.Repeat([]byte{'x'}, size)
	for start, line := 0, 0; start < size; start, line = start+lineBytes, line+1 {
		prefix := "INFO  "
		if line%2 == 0 {
			prefix = "ERROR "
		}
		copy(input[start:], prefix)
		input[start+lineBytes-1] = '\n'
	}
	return input, int64(size / 2)
}

// BenchmarkFilterLines measures the reusable filter, not process startup,
// argument parsing, fixture generation, or actual disk/pipe I/O.
func BenchmarkFilterLines(b *testing.B) {
	cases := []struct {
		name            string
		size, lineBytes int
		contains        string
	}{
		{name: "short_4KiB_all", size: 4 << 10, lineBytes: 128},
		{name: "short_4KiB_half", size: 4 << 10, lineBytes: 128, contains: "ERROR"},
		{name: "short_4KiB_none", size: 4 << 10, lineBytes: 128, contains: "ABSENT"},
		{name: "short_64KiB_half", size: 64 << 10, lineBytes: 128, contains: "ERROR"},
		{name: "short_1MiB_half", size: 1 << 20, lineBytes: 128, contains: "ERROR"},
		{name: "long_64KiB_half", size: 64 << 10, lineBytes: 8 << 10, contains: "ERROR"},
		{name: "long_1MiB_half", size: 1 << 20, lineBytes: 8 << 10, contains: "ERROR"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup happens before b.Loop's first call: it is excluded from
			// timing AND allocation counters. The benchmark owns this fixture;
			// the production filter still processes input incrementally.
			input, halfBytes := makeBenchmarkInput(tc.size, tc.lineBytes)
			want := halfBytes
			if tc.contains == "" {
				want = int64(len(input))
			} else if tc.contains == "ABSENT" {
				want = 0
			}
			var reader bytes.Reader
			reader.Reset(input)
			// Check the fixture/output count outside the measured loop. The
			// ordinary tests remain responsible for full output correctness.
			if n, err := filterLines(io.Discard, &reader, tc.contains); err != nil || n != want {
				b.Fatalf("preflight count %d, error %v; want %d", n, err, want)
			}
			b.ReportAllocs()
			// One operation processes this entire input. Throughput measures
			// INPUT bytes, even when only half (or none) are written.
			b.SetBytes(int64(len(input)))
			var n int64
			for b.Loop() {
				// Reset is part of the timed per-operation harness: otherwise
				// iterations after the first would read EOF and look very fast.
				reader.Reset(input)
				var err error
				n, err = filterLines(io.Discard, &reader, tc.contains)
				if err != nil {
					b.Fatal(err)
				}
			}
			// b.Loop stops the timer when it returns false. Final validation
			// does not inflate the reported ns/op or allocation measurements.
			if n != want {
				b.Fatalf("last count %d, want %d", n, want)
			}
		})
	}
}

// BenchmarkCopyStreamPaths isolates io.Copy's optional interface dispatch.
// These operations do different work from line filtering: do not compare their
// throughput as if copying and filtering were interchangeable implementations.
func BenchmarkCopyStreamPaths(b *testing.B) {
	for _, mode := range []string{"WriterTo", "ReaderFrom", "generic_buffer"} {
		b.Run(mode, func(b *testing.B) {
			input := bytes.Repeat([]byte{'x'}, 1<<20)
			var reader bytes.Reader
			var src io.Reader = &reader
			var dst io.Writer = io.Discard
			if mode != "WriterTo" {
				// Expose only Read, hiding bytes.Reader's optional WriteTo.
				src = readOnly{Reader: &reader}
			}
			if mode == "generic_buffer" {
				// Also hide io.Discard's ReaderFrom, forcing io.Copy's generic
				// buffer path. Wrappers are created once, outside the timer.
				dst = writeOnly{Writer: io.Discard}
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			var n int64
			for b.Loop() {
				reader.Reset(input)
				var err error
				n, err = copyStream(dst, src)
				if err != nil {
					b.Fatal(err)
				}
			}
			if n != int64(len(input)) {
				b.Fatalf("count %d, want %d", n, len(input))
			}
			// WriterTo can hand the whole slice to Discard, which accepts it
			// without examining its contents. An enormous MB/s figure there
			// is logical byte accounting, not measured memory/disk bandwidth.
		})
	}
}

// Embedding the narrow interfaces exposes only their method sets. Embedding
// the concrete streams instead would accidentally expose their fast paths too.
type readOnly struct{ io.Reader }
type writeOnly struct{ io.Writer }
