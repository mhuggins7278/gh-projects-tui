# Implemented simplifications

All six opportunities from the review are implemented. The change removes 98 production Go lines overall and reduces the central keyboard dispatcher from roughly 390 lines to about 70. The original review is retained as a record of the baseline and prototypes; its reproduction script now uses a disposable archive of `fbdc836`.

## Changes

1. Removed the unused parallel lane loader, pending counters/maps, obsolete message handling, deduplication helper, and unreachable per-lane failure presentation. Active saved-query pagination still loads the original filter unchanged.
2. Board navigation remembers focus from the already computed lanes. Local search filters items before sorting them.
3. Sorting computes normalized text/numeric/select/iteration keys once per item and direction once per field. Stable ties, unset values last, secondary sort fields, and source slice ownership are preserved.
4. Table action readback uses the existing uncached ID-only project reader, with a compatibility fallback for sources that only expose full item pages. Issue close/reopen reconciliation uses a new uncached `ReadIssueState` query instead of loading body, fields, and comments. A confirmed state updates the existing body/comment document and invalidates its detail cache. Origin identities and stale-result guards remain intact; missing, mismatched, unchanged, or failed reads leave writes blocked.
5. Each detail document reuses one local Markdown renderer. Initialization is lazy, so empty or inaccessible sections do not pay setup cost. The renderer is never shared across goroutines, and conversion failures retain the plain-text fallback.
6. Extracted input routing into search, detail/comment/issue-confirmation, table-action, board-navigation, and general-navigation handlers. Search controls share Unicode deletion, paste normalization, and cursor clamping; navigation shares identical detail resets. Editor text still takes precedence over printable shortcuts, and Ctrl+C remains available.

## Measured results

Median of three ten-operation samples using the same fixtures as the initial review. Navigation timings include movement plus complete rendering. Search matches 5% of the deterministic shuffled title-sorted cards. These are fixture measurements, not guarantees for every board.

| Fixture | Baseline | Implemented |
|---|---:|---:|
| 1,000 cards, no search | 17.6 ms | 7.6 ms |
| 1,000 cards, search | 20.1 ms | 6.7 ms |
| 10,000 cards, no search | 161.3 ms | 32.8 ms |
| 10,000 cards, search | 183.8 ms | 20.4 ms |
| Cold Markdown detail, 20 comments | 24.2 ms | 22.9 ms |
| Cold Markdown detail, 100 comments | 111.9 ms | 100.4 ms |

The 10,000-card unfiltered operation allocates approximately 5.4 MB and 28,617 objects, down from 29.5 MB and 1,016,932. See [raw integrated benchmark output](evidence/implementation-bench.txt).

## Validation

- Full `go test -race ./...` and `go vet ./...` pass; the CLI builds locally.
- 400 deterministic randomized cases preserve the old sort comparator's output, including missing values, stable ties, multiple keys, Unicode, invalid numeric values, and NaN.
- Search ordering/input ownership, Markdown document independence, editor shortcut precedence, Unicode editing, multiline comment paste, and modal escape behavior are covered.
- Table readback tests require a complete paginated scan before accepting absence, retain the original scope after navigation, and keep the gate blocked when the target is present or a later page fails.
- Issue readback tests reject stale results, retain original issue identity after navigation, preserve body/comments, and invalidate stale detail cache entries. Adapter tests verify that issue-state reads bypass caching and select no body/comment/project data.
- The ID-only adapter fixture returns the same 100 IDs with one request where the full reader takes 101 due to per-item field pagination. This is an adversarial synthetic fixture; ordinary projects benefit mostly from smaller responses.
- The archived baseline/prototype reproduction script also passes after the application refactor.

No live GitHub mutation was used for validation. API behavior is covered with mocked GraphQL responses and existing opt-in live tests remain unchanged. Push CI continues to run tests and vet; binary release builds still run only for `v*` tags.

To repeat current performance measurements:

```sh
GOCACHE=/tmp/gh-projects-tui-review-go-cache go test ./internal/ui -run '^$' \
  -bench 'BenchmarkSortedBoardNavigation|BenchmarkColdDetailRender' \
  -benchtime=10x -count=3
```

To repeat the original baseline/prototype experiments:

```sh
python docs/review-simplification/reproduce.py --bench
```
