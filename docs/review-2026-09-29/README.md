# Code review — September 29, 2026

Reviewed commit: `b230d1d` (`record the saved roadmap endpoint-mapping blocker`). The working tree was clean at the start. This review covers the command, configuration, GitHub adapter, scheduler and caches, UI state transitions, board/table/detail rendering, mutation flows, tests, and CI. This report records the pre-repair baseline. The confirmed findings have since been repaired; see [repair tracking](FIXES.md) for issue links, behavior changes, and validation.

The main risk is ownership of asynchronous state: some completions use the screen's current data instead of the request or mutation's original data. Incremental repairs are appropriate. Preserve the API adapter, compatibility checks, and existing fixtures while strengthening the state contracts.

## Findings

P1 means fix in the next repair pass; P2 means a confirmed correctness or performance issue to address afterward. These priorities describe this pre-alpha application. The reproductions establish local behavior; they do not establish permanent loss or corruption of GitHub data.

| ID | Priority | Finding |
| --- | --- | --- |
| 1 | P1 | Typing `q` in search or a comment quits the application |
| 2 | P1 | Refresh during the final save can erase the displayed board and cancel its reload |
| 3 | P2 | Picker requests can reopen a screen after the user navigates away |
| 4 | P2 | Failed issue-action reconciliation unlocks further writes using stale state |
| 5 | P2 | Leaving a table discards its unknown mutation outcome |
| 6 | P2 | Larger swimlane boards put selected cards outside the terminal viewport |
| 7 | P2 | Detail scrolling repeatedly renders the complete Markdown thread |
| 8 | P2 | Text entry drops spaces, non-ASCII picker input, and pasted text |
| 9 | P2 | Deduplicated reads inherit another caller's cancellation |
| 10 | P2 | UI/browser host resolution differs from API host resolution |
| 11 | P2 | Text truncation counts runes instead of terminal cells |

### 1. Route text input before printable shortcuts

Location: [model.go:1060](/home/mdh/Projects/gh-projects-tui/internal/ui/model.go:1060).

`updateKey` handles `q` before the search and comment input branches. Entering a word such as “request” therefore produces `tea.QuitMsg` instead of adding its `q`. An unfinished comment is lost. `?` also toggles help while composing a comment, and `A` opens the API inspector in debug mode instead of being entered.

Reproduction: `TestReviewTextInputKeepsQ` fails for both search and comment modes.

Fix: dispatch active text input before printable navigation shortcuts. Keep the explicit interrupt shortcut separate. Add regression cases for `q`, `?`, and debug-mode `A` in each editor.

### 2. Keep save results independent of the displayed item list

Location: [mutation.go:349](/home/mdh/Projects/gh-projects-tui/internal/ui/mutation.go:349), especially the assignment at line 353.

The final successful write takes `session.canonicalItems` from `m.items`. Pressing `r` while that write is outstanding starts an item reload and sets `m.items` to nil. When the save completes, its fast path adopts that empty list and `showCanonicalWithoutOptimism` cancels the refresh. The board shows no items even though the fixture's server state still contains the saved item. Switching views also changes the data this fast path adopts.

Reproduction: `TestReviewRefreshPreservesSubmittedSave` dispatches a write, starts refresh, then delivers the successful write result. It produces an empty board with loading stopped.

Fix: apply the acknowledged plan to the session's own baseline, or perform an authoritative readback. Update the visible screen only when its project/view/load identity permits it. Never promote arbitrary current UI data to canonical mutation state. Test refresh, another view, another project, and partial page loads during final completion.

### 3. Scope picker requests to their destination and request identity

Locations: [model.go:308](/home/mdh/Projects/gh-projects-tui/internal/ui/model.go:308), [model.go:1477](/home/mdh/Projects/gh-projects-tui/internal/ui/model.go:1477).

`viewDetailMsg` and `viewsMsg` check a generation, but leaving the view picker does not advance it or cancel that request. Start opening a view, press Escape, and its late result sends the application back to the board. `viewsMsg` can likewise return to the picker and store results under the current selection. Multiple pending selections share the generation until one result starts item loading, allowing an older selection to win.

Reproduction: `TestReviewBackRejectsPendingView` returns to the project picker, delivers the old completion, and observes `screenBoard`.

Fix: give picker reads explicit owner/project/view/request identity, invalidate it on every navigation or new selection, and require the appropriate active screen when applying the result. Extend the existing item-read scoping pattern to picker reads.

### 4. Retain the issue write gate after failed reconciliation

Location: [model.go:352](/home/mdh/Projects/gh-projects-tui/internal/ui/model.go:352).

An ambiguous close/reopen result starts detail readback. Its completion clears `mutationLoading` before checking whether the readback succeeded. On failure, the old detail object remains available. Pressing `x` uses that old state to open another write confirmation, despite the status instructing the user to reconcile first. Even a successful read of unchanged state needs a deliberate outcome policy; it does not prove a timed-out write cannot finish later.

Reproduction: `TestReviewAmbiguousCloseKeepsWriteGate` simulates a timeout followed by failed readback and observes `issueActionConfirm=true`, `mutationLoading=false`.

Fix: track an issue mutation's intended outcome and blocked state explicitly. Keep subsequent issue writes disabled until reconciliation confirms the intended state or an explicit recovery path resolves the uncertainty. Use the board queue's conservative approach as a reference.

### 5. Keep unknown table actions attached to their project

Location: [model.go:1143](/home/mdh/Projects/gh-projects-tui/internal/ui/model.go:1143).

When an archive/remove outcome is blocked, Escape or `v` simply clears `tableAction`. Reopening the table also resets that field. The user can then submit the same action again even though the original outcome remains unknown. Navigation removes the write gate rather than resolving it. This matters particularly because removing a project-only draft deletes the draft itself.

Reproduction: `TestReviewUnknownTableActionSurvivesViewChange` leaves a blocked removal, returns with the item still present, and can open another removal confirmation.

Fix: separate project mutation state from the table's confirmation overlay. Permit navigation while retaining the unresolved action and its gate, and reconcile it independently of the current saved view. A filtered view's membership alone is a weaker observation than authoritative project/item state.

### 6. Add a vertical viewport for swimlanes

Location: [board.go:968](/home/mdh/Projects/gh-projects-tui/internal/ui/board.go:968).

The combined board renders every swimlane row. `swimlaneViewportHeight` has a minimum per-row height, and a card can exceed that minimum. There is no window around the active swimlane. With six populated rows in a 100×30 terminal, the view contains 67 lines and the selected last-row card is below the first 30. Navigation can move the selection to a card the terminal cannot display.

Reproduction: `TestReviewSwimlaneSelectionFitsViewport` selects the last row and checks the terminal's visible line range.

Fix: window swimlane rows around the active row, using actual rendered heights and space for headers/footer/overflow markers. Test several populated rows, long headers, small terminals, and selection at both ends.

### 7. Cache rendered detail lines instead of rebuilding them while scrolling

Locations: [detail.go:149](/home/mdh/Projects/gh-projects-tui/internal/ui/detail.go:149), [detail.go:277](/home/mdh/Projects/gh-projects-tui/internal/ui/detail.go:277), [detail.go:363](/home/mdh/Projects/gh-projects-tui/internal/ui/detail.go:363).

Each scroll calculates the maximum offset by rebuilding `detailLines`; `View` then rebuilds those lines again. Every body/comment constructs its own Glamour renderer on both passes, including content outside the viewport.

The local benchmark includes `Update(j)` plus `View().Content`, a roughly 2 KB Markdown body, and roughly 925-byte Markdown comments:

| Comments | Time per scroll/render | Allocated per operation | Allocations |
| --- | ---: | ---: | ---: |
| 20 | 46–50 ms | ~12.8 MB | ~194,000 |
| 100 | 193–203 ms | ~56.3 MB | ~891,000 |

These are two short, three-iteration runs on the local Intel Core Ultra 9 185H using Go 1.27. They measure local model/render work, excluding network and terminal painting. The large-thread case exceeds the repository's 100 ms interaction target.

Fix: prepare and cache rendered lines when detail content, width, or field visibility changes; scrolling should only update an offset and slice those lines. Reuse rendering setup where practical. Consider progressive comment loading separately, since `LoadItemDetail` also waits for the entire comment thread before returning the body.

### 8. Use text payloads and handle paste messages

Locations: [model.go:239](/home/mdh/Projects/gh-projects-tui/internal/ui/model.go:239), [model.go:1094](/home/mdh/Projects/gh-projects-tui/internal/ui/model.go:1094), [model.go:1428](/home/mdh/Projects/gh-projects-tui/internal/ui/model.go:1428).

Search entry uses `KeyPressMsg.String()` as text. Bubble Tea v2 represents a space as `"space"`, so neither board nor picker search adds it. Picker entry additionally requires a one-byte string, dropping ordinary non-ASCII characters; its backspace also removes a byte. `Update` has no `tea.PasteMsg` branch, so bracketed paste is ignored in both search and comments.

Reproductions: `TestReviewSearchAcceptsSpaces`, `TestReviewPickerAcceptsUnicode`, and `TestReviewTextInputAcceptsPaste` all fail.

Fix: use `msg.Text` for text, named keys only for controls, and process `PasteMsg.Content`. Share an editing component/helper across search and comment entry. Delete complete runes or grapheme clusters and define how pasted newlines behave in single-line search.

### 9. Do not propagate a leader's cancellation to healthy cache callers

Location: [read_cache.go:82](/home/mdh/Projects/gh-projects-tui/internal/github/read_cache.go:82).

The first caller owns the context for a deduplicated request. A later caller with a healthy context joins that flight. Cancelling the first caller sets `flight.err` to `context.Canceled`, which is returned to the later caller too. Quickly cancelling and reopening the same cached query can therefore fail the new load even though its own context is valid.

Reproduction: `TestReviewCacheFollowerSurvivesLeaderCancellation` synchronizes a follower's join before cancelling the leader. The follower receives `context canceled` with its own context still healthy.

Fix: retry a healthy follower when the shared request ended due to another caller's cancellation, or give the shared request independent lifetime management. Keep cancelled reads out of the success cache. Add cache invalidation, cancellation, and concurrent follower tests; there were no dedicated read-cache tests in the baseline suite.

### 10. Resolve one host and use it throughout the application

Location: [config.go:9](/home/mdh/Projects/gh-projects-tui/internal/config/config.go:9).

The UI resolves only environment variables, defaulting to `github.com`. The adapter uses `go-gh`, whose `auth.DefaultHost()` also reads GitHub CLI configuration and chooses its sole configured host. With only an enterprise host configured, API requests use that host while the header and generated browser links use `github.com`. `GH_ENTERPRISE_HOST` is another mismatch: this UI resolver honors it, whereas the installed `go-gh` host resolver uses `GH_HOST` and configuration.

Reproduction: `TestReviewHostMatchesAuthenticatedGHHost` uses an isolated, token-free `hosts.yml` fixture. UI host is `github.com`; API host resolution is `enterprise.example`.

Fix: resolve the host once through `go-gh` and pass it explicitly to API construction and UI/browser state. If retaining extra environment overrides, apply them to the API too. Test configuration-only enterprise access and environment precedence.

### 11. Measure terminal cells when truncating and wrapping

Locations: [styles.go:269](/home/mdh/Projects/gh-projects-tui/internal/ui/styles.go:269), [detail.go:432](/home/mdh/Projects/gh-projects-tui/internal/ui/detail.go:432).

`truncateText` and `wrapText` treat each rune as one column. CJK characters typically occupy two cells, while combining marks and emoji sequences have other widths. Twelve CJK characters survive a twelve-column truncation and occupy 24 cells. Table cells and labels can exceed their allocated widths, breaking alignment, wrapping unexpectedly, or clipping neighboring content.

Reproduction: `TestReviewTruncationFitsTerminalCells` observes 24 cells after a twelve-column truncation.

Fix: use the already-installed ANSI/display-width utilities to truncate and wrap by terminal cells without splitting grapheme clusters. Add CJK, combining-mark, and emoji fixtures to the existing table-width tests.

## Architecture and maintenance recommendations

- **Keep the adapter/UI boundary.** The adapter's domain types, bounded server-filter validation, null-node handling, cursor checks, and uncached mutation readbacks provide useful structure. The scheduler avoids automatically replaying submitted writes. These protections deserve focused integration coverage.
- **Consolidate mutation lifecycle rules.** Board moves, issue actions, and table archive/remove actions currently have different ownership, timeout, reconciliation, and navigation behavior. Share lifecycle concepts such as pending, submitted, acknowledged, unknown, blocked, and reconciled. Keep operation-specific plans separate.
- **Separate navigation from reads and writes.** `model.go` contains 2,251 lines, including discovery, pickers, text editing, issue writes, rendering, and telemetry. Extract these responsibilities gradually after adding the missing transition tests. File splitting alone will not fix the state ownership bugs.
- **Remove unused loading paths.** `parallelLaneItemsRequests` and `laneItemsCmd` have no callers in the active loading path. The model retains lane messages, pending counters, and failure maps from that alternative approach. Retaining two loading designs increases the state space reviewers must understand.
- **Correct the documented behavior before the next release.** The README claims parallel lane loading, rollback of subsequent queued moves, and navigation/refresh waiting for saves. It also describes saved-filtered views as read-only, while `TestSavedFilteredBoardAllowsSupportedMutations` explicitly verifies writes there. Decide the intended contract, then align source, tests, and documentation. The API contract also describes post-write reads more broadly than the terminal-success fast path performs.
- **Preserve comment drafts until success.** The composer clears its draft before submission, so a definitive rejection loses text the user could otherwise correct or retry. An uncertain submission should retain the text while explaining the duplicate risk.
- **Expose actionable cooldown status in normal runs.** Cooldown telemetry and its periodic updates are currently debug-only. A normal run can display a generic save/load status while the scheduler waits for quota. Show the reason and remaining delay when it affects progress.
- **Strengthen CI around real behaviors.** Add the reproductions as ordinary regression tests after repairs, run `go test -race ./...` in CI, and keep a detail-thread benchmark alongside the board benchmark. Prioritize transition and failure-path coverage over a blanket coverage target.

## Verification and limits

Baseline checks passed: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`. Tests with coverage instrumentation also passed after placing the Go build cache in writable `/tmp`.

Statement coverage: command 26.7%, config 84.6%, GitHub adapter 66.7%, UI 74.1%; overall 72.1%. Passing tests and coverage percentages did not exercise the event orders reproduced here.

The existing saved-filter fuzz target passed a ten-second run with 245,761 executions. Live-tagged adapter tests compiled with `-run '^$'`, executing no live tests. All source/evidence links and the overlay runner were checked; the overlay reproductions failed in the expected ways while the normal test suite remained green.

No live GitHub reads or writes were performed. API semantics, current third-party vulnerability advisories, and real terminal painting were not independently revalidated. The configured Go version is 1.25; this machine runs `go1.27.0-X:nodwarf5`, so these results do not establish an independent minimum-toolchain check.

## Reproducing the review

The text evidence files preserve the original baseline assertions. Repairs promote those assertions into ordinary regression tests. The runner uses the permanent tests when present and falls back to Go overlays on a pre-fix checkout:

```sh
python3 docs/review-2026-09-29/reproduce.py
python3 docs/review-2026-09-29/reproduce.py --bench
```

The first command passes on the repaired branch. It is **expected to exit with failure on the original reviewed code**, because it asserts the desired behavior for the confirmed bugs. The benchmark command passes and reports timings. The tests reuse existing fixture helpers and make no network requests.

Evidence: [UI reproductions](/home/mdh/Projects/gh-projects-tui/docs/review-2026-09-29/ui_test.go.txt), [cache reproduction](/home/mdh/Projects/gh-projects-tui/docs/review-2026-09-29/github_test.go.txt), [host reproduction](/home/mdh/Projects/gh-projects-tui/docs/review-2026-09-29/config_test.go.txt), [runner](/home/mdh/Projects/gh-projects-tui/docs/review-2026-09-29/reproduce.py).

## Suggested repair order

1. Fix text-entry dispatch and the final-save state corruption, with regression tests.
2. Scope picker requests and preserve unknown issue/table mutation state across navigation.
3. Repair input payload handling, cache cancellation, and shared host resolution.
4. Cache detail rendering, add a swimlane viewport, and use terminal-cell widths.
5. Remove obsolete loading state, align documentation, and improve CI.
