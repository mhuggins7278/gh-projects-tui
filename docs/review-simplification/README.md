# Simplification and performance review

This report records the initial review of application code at `fbdc836`, focusing on the UI, rendering, GitHub adapters, request scheduling, caching, filtering, configuration, and startup. Its six opportunities have since been implemented; see [implementation and validation](IMPLEMENTATION.md). The original experiments used Go overlays in `/tmp` without changing application sources.

The strongest performance result is faster navigation through large, sorted boards. The strongest simplification is deleting an unreachable lane-loading subsystem. These can be implemented in small steps using the existing regressions.

## 1. Delete the unused parallel lane loader

**Value: high maintenance benefit. Effort: small. Prototype validated.**

[parallelLaneItemsRequests](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/board.go#L229) and [laneItemsCmd](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/model.go#L853) have no callers. The lane-loading maps and pending counter are never initialized by active loading code; they are repeatedly cleared across navigation, refresh, and mutation transitions. Their message handler, pagination loop, deduplication helper, and rendering flags therefore describe a loading mechanism the application no longer uses.

Remove the two unused entry points, `updateLaneItems`, `laneItemsMsg`, `laneItemsRequest`, `appendUniqueItems`, the three model fields, and their resets. Lanes can take their loading flag directly from `m.itemsLoading`. The prototype removes **148 production lines** and passes all tests after replacing one obsolete test assertion against the removed private fields. That test still executes both pages of the saved-filter query. The unreachable `boardLane.Failed` presentation branches can also be removed when landing the change; that additional cleanup is outside the prototype.

Keep the active saved-query pagination path unchanged.

## 2. Avoid rebuilding lanes to remember focus, and filter before sorting

**Value: high on large boards and searches. Effort: small. Prototype validated.**

[moveBoardLane / moveBoardCard](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/board.go#L800) already have the selected lane and item. Calling [rememberBoardFocus](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/model.go#L1052) rebuilds the entire sorted/grouped board to retrieve the same item's ID. Rendering then builds it again. Assign the ID from the lanes already computed by the navigation handler.

[boardItems](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/board.go#L55) sorts all items before applying local search. Filter first and sort the surviving items. Stable sorting with the same comparator preserves the intended order of matches. Preserve the existing search matching rules and input ownership.

Together, these changes reduced the 10,000-card search fixture from **183.8 ms to 24.4 ms** per navigation-plus-render operation. With no search, the same fixture fell from **161.3 ms to 109.9 ms**. This avoids introducing a persistent derived-view cache and its invalidation rules.

## 3. Compute sort keys once per item

**Value: high on large sorted boards. Effort: moderate. Prototype validated.**

[sortedItemsForView / compareSortField](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/board.go#L101) repeatedly extracts field values inside the comparison function. Each comparison may lowercase title/text values, parse a number, or search options/iterations. Those values only depend on the item and sort specification during this sort.

Build normalized sort keys once per item and field, sort lightweight records containing the original index and keys, then produce the ordered item slice. Determine sort direction once per field. Keep stable ties and unset values last in both directions. Preserve current numeric behavior, including unparseable numbers and NaN; changing that behavior would be a separate correctness decision.

The prototype includes opportunity 2. On the 10,000-card fixture without search it reduced **161.3 ms to 33.4 ms**, allocated bytes from **29.5 MB to 5.4 MB**, and allocations from **1,016,932 to 28,618** per operation. On 1,000 cards, timing fell from **17.6 ms to 7.8 ms**. The sort helper is also used by tables; table performance was not benchmarked here.

Beyond existing regressions, **400 deterministic randomized cases** compare the prototype with the original sorter across title, text, numeric, single-select, and iteration fields; ascending/descending order; secondary keys; missing content; unavailable values; mixed case; and numeric edge cases.

## 4. Use minimal authoritative reads for reconciliation

**Value: fewer API requests and dependencies. Effort: small for table membership, moderate for issue state. Adapter evidence validated; UI integration proposed.**

### Table deletion / archive readback

[startTableActionReadback](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/table_actions.go#L266) calls `readCanonicalProjectItems`, whose `PageItems` path fetches content and all item field values. The result handler only checks whether a target ID is present.

Reuse the existing [readMutationItems](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/mutation.go#L268) helper with no fields and the client's [PageMutationItems](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/github/selective_items.go#L17). That query already returns IDs in canonical project order and bypasses the read cache. Preserve the full project scan before treating absence as confirmed, the originating project scope, and the blocked state when readback fails. Keep a fallback for sources that only implement the existing `ItemsSource` interface if needed.

In a **synthetic 100-item fixture where every item's field values require a second page**, the existing full reader made **101 requests**; the existing ID-only reader made **1**, returning the same IDs in the same order. Ordinary projects with no extra field-value pages still use one request per project page; their benefit is smaller responses and less projection work. This is a fixture result, not a measured production workload.

### Issue close / reopen readback

[startIssueReadback](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/issue_actions.go#L33) needs the originating issue ID and state, but calls [LoadItemDetail](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/github/detail.go#L160). That fetches body, project metadata, field values, and all comments before returning. A fixture with 250 comments required **4 requests**, including three comment pages, just to confirm `CLOSED`.

Add an uncached issue-state read using the already stored issue ID. Return a small typed result, update the displayed item's state, and invalidate detail data separately. A dedicated state read would avoid comment/field pagination. The proposed state query was not implemented or checked against live GitHub in this pass. Retain action/read IDs, origin scope, and the persistent write gate; inaccessible or mismatched state must remain unknown, and mutations must never be automatically replayed.

## 5. Reuse a Markdown renderer for one detail document

**Value: modest cold-render improvement. Effort: small. Prototype validated.**

[detailLines](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/detail.go#L254) calls [renderMarkdown](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/detail.go#L396) for the body and every comment. Each call creates and configures a Glamour renderer. Initialize one renderer for a detail document and reuse it for the independent body/comment conversions, with the existing plain-text fallback on failure. The installed renderer's `Render` method converts into a fresh output buffer, so this can use a renderer local to a single document render rather than sharing it across goroutines.

The cold 100-comment fixture improved from **111.9 ms to 99.3 ms** and allocated bytes from **31.0 MB to 24.9 MB**. Most remaining work is parsing/rendering the Markdown itself. This change helps initial display and width/field-toggle changes; the existing line cache already handles scrolling. Do not describe these cold timings as per-scroll costs.

The output comparison covers narrow/wide widths, independent link/reference documents, tables, code blocks, headings, quotes, and lists. A production implementation can extract a shared render helper instead of duplicating the existing formatting/fallback code as the isolated prototype does. Lazy or incremental comments would offer a different UX tradeoff and needs more state; it is not required for this cleanup.

## 6. Extract focused input handlers from the model

**Value: maintainability. Effort: moderate. Proposal; no performance claim.**

[updateKey](https://github.com/mhuggins7278/gh-projects-tui/blob/fbdc836/internal/ui/model.go#L1058) spans roughly 390 lines and mixes global shortcuts, editor routing, picker search, table confirmation/movement, issue writes, detail controls, and navigation. Search editing and cursor clamping are duplicated across this function, `appendInputText`, and `updateBoardSearchKey`. Detail/search resets also recur in view loading, refresh, and back navigation.

Extract handlers by the existing mode boundaries: picker search, board search, comment editing, detail controls, and table action controls. Share only the editing operations and reset groups that actually have identical semantics. Keep text payload routing ahead of printable global shortcuts, with Ctrl+C still available. Preserve generation/request scope checks and the separate board, table, and issue mutation lifetimes. A generic mutation framework or a wholesale state-machine rewrite would add risk without a demonstrated benefit.

Land this after the smaller deletions and measured performance changes. File splitting alone is not a useful outcome; the goal is reducing repeated transitions and the number of modes one handler must reason about.

## Measurements and validation

Median of three samples, ten operations per sample, Go `go1.27.0-X:nodwarf5`, Linux amd64, Intel Core Ultra 9 185H. Navigation fixtures use deterministic shuffled mixed-case titles, a 123×63 terminal, and alternate card movement followed by a complete `View()` render. Search matches 5% of cards. Cold detail fixtures use Markdown-rich body/comments at an 80-column document width. Timings include allocation/GC effects and are illustrative, not a latency guarantee for every project.

| Workload | Current | Small navigation changes | Navigation changes + sort keys |
|---|---:|---:|---:|
| 1,000 cards, no search | 17.6 ms | 14.2 ms | 7.8 ms |
| 1,000 cards, 5% search matches | 20.1 ms | 7.2 ms | 7.1 ms |
| 10,000 cards, no search | 161.3 ms | 109.9 ms | 33.4 ms |
| 10,000 cards, 5% search matches | 183.8 ms | 24.4 ms | 20.0 ms |

| Cold detail | Current | One renderer per document |
|---|---:|---:|
| 20 comments | 24.2 ms | 22.0 ms |
| 100 comments | 111.9 ms | 99.3 ms |

Validation completed:

- The baseline and all four individual prototypes pass `go test ./...`.
- The combined dead-code, navigation, sort-key, and Markdown prototype passes `go test -race ./...` and `go vet ./...`.
- Render/focus/sort output digests match the baseline; randomized sorter and explicit Markdown equivalence tests also pass.
- Mock adapter tests confirm the membership request counts, identical ID order, and the issue-detail comment reads.
- No live GitHub writes or reads were performed for this review. Existing opt-in live tests remain skipped.

Raw outputs are saved in [evidence](evidence/baseline.txt). Reproduce with:

```sh
python docs/review-simplification/reproduce.py --bench
```

The script archives the reviewed commit into a disposable checkout, generates prototype sources and overlays under `/tmp/gh-projects-tui-simplification-review`, uses a temporary Go build cache, validates the variants, and optionally repeats benchmarks. It does not edit application sources. Benchmark results from a later run may vary.

## Suggested implementation order

1. Delete the unused lane loader.
2. Land the small navigation/search changes.
3. Precompute sort keys, retaining equivalence checks for edge cases.
4. Reuse ID-only table readback, then add an issue-state-only read.
5. Reuse a local Markdown renderer if the measured cold-render improvement is worthwhile.
6. Extract input handlers after the behavior and loading paths are smaller.

Do not increase API concurrency, replace cache ownership/cancellation logic, or introduce a global derived-view cache in this pass. The existing request scheduler and cache address concrete correctness constraints; the measured opportunities above offer gains without disturbing them.
