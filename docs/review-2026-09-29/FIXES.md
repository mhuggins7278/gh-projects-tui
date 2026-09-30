# Review repairs

The eleven confirmed findings from the September 29 review are implemented on
`codex/review-fixes`. Their GitHub issues remain open pending review and merge.
The original report and text fixtures preserve the baseline evidence.

| Issue | Repair | Regression coverage |
| --- | --- | --- |
| [#31](https://github.com/mhuggins7278/gh-projects-tui/issues/31) | Focused editors consume printable text before shortcuts; Ctrl+C still quits. | Shortcut entry in picker search, board search, and comments. |
| [#32](https://github.com/mhuggins7278/gh-projects-tui/issues/32) | Final saves apply their plans to session-owned data. Results cannot adopt another project's items or a partial refresh. Overlapping reads finish before a fresh load. | Refresh during save, partial pages, project changes, and different view fields. |
| [#33](https://github.com/mhuggins7278/gh-projects-tui/issues/33) | Picker results bind host, owner, project, screen, and request generation. Navigation and newer selections supersede old requests. | Back during open, same-generation wrong destination, successive selections, and startup navigation. |
| [#34](https://github.com/mhuggins7278/gh-projects-tui/issues/34) | Submitted close/reopen actions retain identity and intended state. Failed, unchanged, or wrong-issue readbacks retain the write gate. `r` reads the originating issue without replaying the write. | Failed readback, unchanged/wrong state, navigation, confirmed outcome, and end-to-end submission. |
| [#35](https://github.com/mhuggins7278/gh-projects-tui/issues/35) | Archive/remove actions survive read generations and navigation. Unknown outcomes use unfiltered originating-project reads; view membership cannot unlock the gate. | Returning to a view, filtered membership, another project, definitive completion, and read-only retry. |
| [#36](https://github.com/mhuggins7278/gh-projects-tui/issues/36) | Swimlanes window around selection, measure available space and rendered heights, and use compact cards on short terminals. | Six populated rows, long headers, beginning/middle/end selection, and 24–70 row terminals. |
| [#37](https://github.com/mhuggins7278/gh-projects-tui/issues/37) | Detail rendering retains one document per model, keyed by detail identity, panel width, field visibility, and issue state. Scrolling reuses its lines. | Cache reuse, resize, fields, issue state, fresh comments, and scroll benchmarks. |
| [#38](https://github.com/mhuggins7278/gh-projects-tui/issues/38) | Shared text payload handling accepts spaces, Unicode, and bracketed paste. Backspace deletes complete runes; pasted newlines become spaces in searches. | Unicode deletion, spaces, single-line search paste, and multiline comment paste. |
| [#39](https://github.com/mhuggins7278/gh-projects-tui/issues/39) | A healthy cache follower starts/joins fresh work after its cancelled leader completes. Its own cancellation still ends its request. | Leader cancellation, follower cancellation, and invalidation overlapping an old flight. |
| [#40](https://github.com/mhuggins7278/gh-projects-tui/issues/40) | Configuration uses gh's authenticated default host when environment overrides are absent. Main passes its resolved host explicitly to GraphQL, REST, and the UI. | Single enterprise host in gh config and mocked REST/GraphQL requests with a conflicting default. |
| [#41](https://github.com/mhuggins7278/gh-projects-tui/issues/41) | Truncation and wrapping measure terminal cells and preserve grapheme boundaries. | CJK, emoji sequences, combining marks, ASCII, and narrow columns. |

Additional maintenance: issue actions now live in their own source file; CI runs
the race detector; README/API-contract descriptions match the active loader,
write reconciliation, and navigation behavior.

## Validation

Passed locally:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./...`
- `python3 docs/review-2026-09-29/reproduce.py`

Tests use fixtures and mocked HTTP. No live project data was read or mutated.
This machine runs Go 1.27; CI uses the Go version declared by the module (1.25).
The checks do not establish live API semantics or actual terminal painting.

## Detail scrolling measurements

Local Intel Core Ultra 9 185H, Linux/amd64, Go 1.27. Both benchmarks include
`Update(j)` and `View().Content`, a Markdown body, and Markdown comments. Network
latency and terminal painting are excluded.

| Comments | Baseline, 3 iterations | Repaired, 3 iterations including first render | Repaired, cached document, 100 iterations |
| --- | ---: | ---: | ---: |
| 20 | 46–50 ms/op; ~12.8 MB/op | 10–11 ms/op; ~2.66 MB/op | 2.56–2.62 ms/op; ~0.65 MB/op |
| 100 | 193–203 ms/op; ~56.3 MB/op | 35–38 ms/op; ~9.91 MB/op | 2.32–2.47 ms/op; ~0.64 MB/op |

Each range covers two local runs. Initial Markdown preparation still scales
with document length; scrolling reuses that preparation. Progressive network
loading of comments is a separate follow-up.

```sh
go test ./internal/ui -run '^$' -bench '^BenchmarkReviewDetailScroll$' -benchtime=3x -count=2
go test ./internal/ui -run '^$' -bench '^BenchmarkCachedDetailScroll$' -benchtime=100x -count=2
```

The broader architectural recommendations in the baseline report remain
follow-ups; this branch addresses the eleven confirmed issues.
