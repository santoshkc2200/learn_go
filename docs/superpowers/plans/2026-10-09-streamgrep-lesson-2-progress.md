# Lesson 2 execution record

- User explicitly requested implementation of the next approved lesson; retained the native execution approach.
- Scope: bounded extension of the existing project using the approved spec's line-filtering behavior; no CLI flags yet.
- Baseline: go test ./... passed (22 reported tests including subtests).
- Added filter.go and filter_test.go to keep the new lesson readable separately from Lesson 1.
- Filtering, line-boundary, and failure tests were observed failing against a stub before implementation.
- Updated CLI contract tests were observed failing against the byte copier before switching run to filterLines with an empty substring.
- Current suite: go test ./... passed (57 reported tests including subtests).
- Ruling: keep copyStream and Lesson 1 tests for educational comparison; archive the old README as LESSON1.md and explain that its executable examples describe the historical behavior.
- No Git repository exists; retain local files and this record rather than attempting branch or commit operations.
- Verification: go test -count=1 ./..., go vet ./..., go build ./... passed; PowerShell example and byte-level built executable LF-normalization check passed. Build executable and temporary verification files removed.
- Final review: one P2 finding confirmed by TestFilterLinesReadFailureAtLimit RED; raw size checking could hide a simultaneous source error. Moved real read-error handling ahead of size checking, documented precedence, and verified GREEN. Full suite passed (61 reported tests including subtests); final vet and build passed. No deferred findings or declined-to-judge behavior.
