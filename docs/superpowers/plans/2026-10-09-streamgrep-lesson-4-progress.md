# Lesson 4 execution record

- User requested the next approved chapter: deepen failure testing and teaching comments.
- Baseline: go test ./... passed (81 reported tests including subtests).
- Added failure_test.go: partial writes inside/between lines, CLI stderr separation and status, typed read errors, Is/As through wrapping, prior output on a line-limit error, naive retry duplication, and simultaneous stdout/stderr failure.
- Added a contract-respecting test-only limitedWriter with explicit storage and no embedded fast-path methods.
- Existing production code already satisfies these contracts; new tests initially passed without production changes.
- Temporarily seeded lost-byte-count and lost-error-identity regressions; focused tests failed on the intended assertions. Restored the original filter.go in finally blocks; full suite then passed (92 reported tests before the final simultaneous-stream test).
- Archived CLI teaching notes in LESSON3.md and wrote the Lesson 4 README with runnable experiments.
- Production interfaces and behavior retained; no Git repository exists, so files and this local record are the artifacts.
- Final verification: go test -count=1 ./... passed (93 reported tests including subtests); go vet ./... and go build ./... passed. Generated executable removed.
- Final independent review: approved; no actionable findings and no declined-to-judge behaviors. Lesson 4 complete.
