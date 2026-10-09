# Lesson 5 execution record

- User requested the final approved streaming CLI chapter: benchmark/allocation measurement before optimization.
- Baseline suite passed (93 reported tests). Read installed testing.B.Loop, B.SetBytes and io.Copy documentation before implementing the harness.
- Fixture test was observed failing on stub output before implementing alternating complete lines and an independently known matching-byte count.
- Added seven filter benchmark cases (input sizes, line sizes, matching rates) and three io.Copy optional-interface paths. b.Loop excludes setup; reader resets are included in measured operations; output counts verified.
- Saved three 100 ms unprofiled baseline samples per case and environment metadata. Short-line 1 MiB half-match case: 200832 B/op, 4098 allocs/op.
- Separate allocation profile attributed about 98% sampled alloc_space to bytes.NewReader in matched-line output.
- Allocation-growth test observed RED: 4 KiB vs 64 KiB all-match short-line input allocated 34 vs 514 objects. Target is bounded growth, with tolerance for fixed overhead.
- Narrow production change: write existing line slice directly, explicitly preserve io.ErrShortWrite, byte counts and wrapping. Existing failure tests passed; suite GREEN (95 reported tests).
- Saved repeated after-change samples: same 1 MiB case 4224 B/op, 2 allocs/op; timing medians 237607 vs 146622 ns/op documented as exploratory, not statistically reliable speedup.
- Archived Lesson 4 notes as LESSON4.md; current README teaches benchmarks, profile interpretation, optional interface paths and exercises; measurement README records raw data and limitations.
- No Git repository exists; retain local files/records. No remaining performance changes are proposed for this chapter.
- Final checks: go test -count=1 ./... passed (95 reported tests); go vet ./... and go build ./... passed; CLI example still selected ERROR failed. Generated executable/test binaries removed; raw profiles and comparison results retained.
- Independent review: no correctness or benchmark-methodology issues. Corrected old Lesson 4 counting exercise to match written += int64(n), preserving its intended assertion-failure experiment. Reviewer independently confirmed 95 tests, vet and allocation-growth/fixture checks.
- Review scope ruling: impossible Write counts are outside the io.Writer contract and were not expanded into new behavior; invalid custom writers would not have meaningful byte-count guarantees. No remaining actionable findings. All five lessons complete.
