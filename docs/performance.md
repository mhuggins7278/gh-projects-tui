# Board Responsiveness Measurement

## Reference setup

- Date: 2026-09-24
- OS: Linux x86_64
- CPU: Intel Core Ultra 9 185H (22 online logical CPUs)
- Go: go1.27.0-X:nodwarf5
- Display: 2880×1800 at 1.6× scale
- Terminal pane: 123×63 columns/rows

## Fixture and method

Run:

```sh
go test ./internal/ui -run 'TestLargeBoard' -count=3 -v
go test ./internal/ui -run '^$' -bench '^BenchmarkLargeBoardNavigation$' -benchtime=20x -count=2
```

The sanitized fixture contains 1,000 synthetic issue cards in ten 100-card
pages. The first page has a simulated 3 ms read delay; the next page and an
80 ms lazy detail request are held behind test gates while 102
key-input/model-update/render cycles (navigation and alternating matching search
terms) are measured. After all pages load, 67 additional cycles exercise the
last card in the lane and local search, followed by 30 cycles with a saved
title sort. Each interaction measurement includes `Model.Update` and generating
`Model.View().Content`; the first-page timing also includes the rendered view.
It does not include terminal emulator painting or a live GitHub network
request. The test checks that selection identity survives progressive page
completion.

## Results

Three runs produced:

| Metric | Results |
| --- | ---: |
| First-page read + rendered view | 8.98–12.33 ms |
| During page/detail reads: input + view-generation p95 | 6.70–7.30 ms |
| Fully loaded, position-sorted: input + view-generation p95 | 7.43–8.45 ms |
| Fully loaded, title-sorted: input + view-generation p95 | 10.24–10.84 ms |
| Board page requests | 10 |
| Lazy detail requests | 1 |

The full-board benchmark exposed a slow path: `boardWindowForLane` rendered all
1,000 cards to calculate their heights even though only a few fit in the
viewport. On this machine, the position-sorted 1,000-card navigation/render
benchmark was about **68 ms/op and 313k allocations/op** before the change.
Measuring only cards adjacent to the visible viewport reduced it to **5.9–6.1
ms/op and about 8.5k allocations/op**. The 100-card case also dropped from
roughly 12 ms/op to 4.8–5.5 ms/op. These are repeatable local model/render
baselines; the measured p95 is below the 100 ms target on this fixture and
reference machine, not a claim about live API latency or terminal paint time.

## Live board fetch investigation (2026-11-20)

`TestLiveBoardFetchComparison` compares read paths against an explicitly selected
existing board. It submits no mutations. Only aggregate timings, request counts,
response sizes, permission availability, and parity/difference counts are
printed; item IDs, titles, field names, and response bodies stay in memory.

```sh
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
GH_PROJECTS_TUI_LIVE_VIEW=BOARD_VIEW_NUMBER \
GH_PROJECTS_TUI_LIVE_ROUNDS=3 \
go test -tags live ./internal/github \
  -run '^TestLiveBoardFetchComparison$' -count=1 -v -timeout=15m
```

Organization ownership is the default; set `GH_PROJECTS_TUI_LIVE_OWNER_KIND=user`
for a personal project. Rounds default to one and are limited to five. Each
candidate invalidates application read caches and rereads view metadata.
Candidate order rotates and alternates direction between rounds; server caches
and HTTP connections are not reset. The existing serialized request scheduler
remains in place.

`setup` includes saved-view metadata and, for selected-fields REST, the field
identity lookup. `first` and `fetch` measure item reads only; `end-to-end` also
includes setup. Item-read HTTP counts include retries and nested requests.
Response sizes are bytes consumed from HTTP response bodies, not compressed wire
sizes. GraphQL points exclude setup and are not comparable to REST quota units.
These are fetch timings, not terminal paint or input-latency measurements.

Membership and canonical page order are compared separately from card data.
Card comparisons include content identity, title, repository, state, sub-issue
summary, and the requested field values/option IDs, but exclude action
permissions and transport-specific union names. Title and progress cells use
their content projection. Lean and structure-only reads compare only their
requested fields; structure-only reads do not have card content. The final
baseline reread detects changes during sampling. Parity is reported, not asserted:
a passing benchmark does not establish an equivalent production replacement.

The REST projection is test-only and intentionally limited to the sampled field
types: title, assignees, single-select, linked pull requests, and sub-issue
progress. Unknown types or unmapped field identities fail rather than silently
dropping values. Empty user/PR lists are projected as absent cells, matching the
sampled GraphQL response, and merged PR state is normalized to `MERGED`.
REST does not supply the permission flags needed by issue actions.

### Initial comparison (before the production change)

The historical tables below use "current" for the original eager-permission
loader. The test now calls that shape `legacy-permissions GraphQL`; `current
GraphQL` calls the optimized production method directly.

The sampled board contained 656 items across seven 100-item pages. All candidate
membership sets and page orders agreed. The first and final baseline differed on
two cards, so initial card-value differences cannot all be attributed to the
candidate transport.

| Item-read path | First page | All pages | Response size | GraphQL points |
| --- | ---: | ---: | ---: | ---: |
| Current selective GraphQL | 3.968 s | 20.945 s | 0.67 MiB | 35 |
| Lean GraphQL (group/sort fields plus current content) | 2.299 s | 16.085 s | 0.39 MiB | 7 |
| Actual ID-only GraphQL | 0.639 s | 6.028 s | 0.03 MiB | 7 |
| Full field-connection GraphQL | 3.126 s | 23.527 s | 0.86 MiB | 1,407 |
| Saved-view REST, default fields decoded as IDs | 1.529 s | 13.390 s | 7.98 MiB | — |
| Saved-view REST, requested card/group/sort fields | 2.216 s | 17.702 s | 10.51 MiB | — |

The full field-connection query fetches extra fields, not an identical selection.
It is now **opt-in** through `GH_PROJECTS_TUI_LIVE_FULL_FIELDS=1`, because this
single run consumed about 40 times the current query's GraphQL points and was
slower. Do not repeatedly run it just to stabilize timing noise.

The REST membership helper discards card data, but the endpoint still sends
content. Its 13.4-second result is not the cost of a minimal ID-only request.
Requesting the actual board fields reduced its apparent advantage substantially,
with about 16 times the decoded payload of selective GraphQL.

A follow-up structure-only GraphQL read (IDs and grouping/sort values, no
content) took 7.262 seconds and 7 points, with exact membership, order, and
requested-field parity. The current query took 17.552 seconds in that run; its
final verification took 16.147 seconds with unchanged card data.

### Repeated content-cost comparison

Three rotated rounds retained the same 656 items, page order, and saved-view
metadata. The final baseline had zero card-data differences from the initial
baseline. Every candidate used seven HTTP item-read requests per round.

| Item-read path | Median first page | Median all pages (range) | Median including setup |
| --- | ---: | ---: | ---: |
| Current selective GraphQL | 3.163 s | 18.103 s (17.292–18.756) | 18.666 s |
| Selected-fields REST | 2.362 s | 17.427 s (17.284–18.141) | 18.107 s |
| Structure-only GraphQL | 0.969 s | 7.336 s (7.201–7.859) | 7.665 s |
| GraphQL without action permissions | 1.971 s | 14.286 s (14.036–14.904) | 14.639 s |
| GraphQL without sub-issue progress | 2.714 s | 17.846 s (17.620–18.680) | 18.201 s |
| GraphQL without either | 2.093 s | 13.922 s (13.740–14.505) | 14.455 s |

Reproduce these read shapes with their current names (without the other initial
comparisons). The optimized production shape is now `current GraphQL`; the
old no-progress shape is `legacy-no-progress GraphQL`, and the old minimal-core
shape is `no-progress GraphQL`:

```sh
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
GH_PROJECTS_TUI_LIVE_VIEW=BOARD_VIEW_NUMBER \
GH_PROJECTS_TUI_LIVE_ROUNDS=3 \
go test -tags live ./internal/github \
  -run '^TestLiveBoardFetchComparison$/(round_[0-9]+|verification)/(current_GraphQL|legacy-permissions_GraphQL|selected-fields_REST|structure-only_GraphQL|legacy-no-progress_GraphQL|no-progress_GraphQL)$' \
  -count=1 -v -timeout=15m
```

Observed parity:

- Structure-only: exact requested grouping/sorting values, but no card content.
- Without permissions: exact compared card data, while removing all 1,830
  known issue-action permission values. This is not action-behavior parity.
- Without progress, with or without permissions: expected summary differences
  on 44 cards, and no other compared differences.
- REST after empty-cell and merged-state normalization: seven consistently
  different cards, comprising four linked-PR display values and three
  single-select option IDs. Their causes remain unresolved; these counts do not
  establish that either transport is wrong. No action permissions are supplied.

**Investigation recommendation (implemented below):** retain selective GraphQL and move
`viewerCanSetFields`, `viewerCanClose`, and `viewerCanReopen` off the eager
all-card path. Omitting only those selections reduced the median all-page read
by about 21% (3.8 seconds) and the first page by about 38%, without changing the
other compared card data. Existing detail reads already request these flags.
Board/table shortcuts and issue-field mutations must preserve their intended
permission checks before any production change; unknown flags must not simply
be assumed to authorize an action.

Keep sub-issue summaries: dropping them alone provided little benefit relative
to run-to-run variation, and dropping them in addition to permissions saved
only another 0.36 seconds at the medians. REST's approximately 4% item-read
improvement is not compelling given the much larger payload, unresolved value
differences, and missing permissions. Full field-connection GraphQL was worse
on both speed and cost.

Fourteen seconds is still a long complete-board load. Retaining in-memory
board snapshots during refresh/revisit is a separate candidate for improving
repeat-use latency. Structure-only paging suggests a possible seam for later
viewport-prioritized content loading, but its 7.3-second timing is **not** a
measurement of a usable, fully rendered board or a two-phase loader. Snapshots
and deferred card content remain unimplemented. The production follow-up below
implements only deferred action permissions.

### Production follow-up: deferred action permissions

The selective board/table row query no longer requests `viewerCanSetFields`,
`viewerCanClose`, or `viewerCanReopen` for every issue. Card fields, grouping,
sorting, issue state, and sub-issue summaries are unchanged. Full item details,
selected-item field editing, and existing mutation membership checks retain
their permission selections.

`SetIssueClosed` and issue-backed field set/clear writes now make a small,
uncached permission read for the specific issue immediately before submitting
the mutation. Only the permission relevant to that action is selected. False,
missing, null, inaccessible, or mismatched results prevent submission. A failed
permission read is a definitive no-write failure, even for a timeout or server
error; once the mutation is submitted, the existing ambiguous-outcome handling
is unchanged. Project-only field writes and position changes do not incur this
issue-permission read.

This trades one targeted request per confirmed issue action for avoiding up to
three permission computations per issue during every board load. Existing
optimistic UI actions roll back on preflight denial without entering unknown
write-outcome reconciliation. Mutation safety is verified with adapter and
board/table model fixtures; no live mutations are submitted to the sampled board.

Compare optimized production loading against the previous eager-permission
shape:

```sh
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
GH_PROJECTS_TUI_LIVE_VIEW=BOARD_VIEW_NUMBER \
GH_PROJECTS_TUI_LIVE_ROUNDS=3 \
go test -tags live ./internal/github \
  -run '^TestLiveBoardFetchComparison$/(round_[0-9]+|verification)/(current_GraphQL|legacy-permissions_GraphQL)$' \
  -count=1 -v -timeout=10m
```

The legacy comparison injects the original permission selections only inside
the test harness. It does not add a production configuration option or an
alternative loading state machine.

The production-path verification used the same board, now containing 658 items
across seven pages, for three rounds with alternating candidate order:

| Read path | Median first page | Median all pages (range) | Median including setup |
| --- | ---: | ---: | ---: |
| Optimized production | 2.137 s | 14.264 s (13.899–14.350) | 14.548 s |
| Previous eager-permission shape | 2.310 s | 16.851 s (16.656–17.025) | 17.155 s |

This is about a **15% item-fetch improvement** in this later sample, not a
guaranteed 21% from the earlier investigation. Both paths used seven item-read
requests and 35 GraphQL points. Response sizes were 0.63 versus 0.67 MiB, with
zero versus 1,836 eagerly computed permission values. All three comparisons
matched membership, page order, view metadata, and other compared card data.
The final production baseline also matched the initial one with zero
card-data differences. A roughly 14-second full fetch is still not an instant
cold board load.

### Page size and count-based parallel loading

The 100-item size is GitHub's connection limit, not an arbitrary application
default. [GitHub's pagination documentation](https://docs.github.com/en/graphql/guides/using-pagination-in-the-graphql-api)
allows `first`/`last` only from 1 to 100. Read-only probes against the sampled
project confirmed both relevant limits:

- `items(first: 300)` failed with `EXCESSIVE_PAGINATION`: the `first` limit is 100.
- `nodes(ids: ...)` with 300 entries failed with `ARGUMENT_LIMIT`: at most 100
  node IDs may be provided.

A filtered `totalCount` is available, but does not supply offset-based access or
the cursors needed to jump to arbitrary pages. Forward and backward cursor
pagination are both supported. That permits two independent chains starting
from the known ends, without inventing or decoding cursors.

The original experiment enabled three additional candidates with
`GH_PROJECTS_TUI_LIVE_LOADING=1`. The smaller-page candidates remain test-only;
the bidirectional case now delegates to the production reader described below:

- **50-per-page:** fetch full cards in sequential 50-item pages.
- **25-first:** fetch 25 cards first, then use 100-item pages for the remainder.
- **Bidirectional:** include `totalCount` with the first 100-card page, divide the
  remaining count between front and back, and read the two partitions with
  `first/after` and `last/before` concurrently. The initial prototype temporarily
  allowed two in-flight reads through the existing shared cooldown/quota logic.
  It delivers the ascending front prefix while prefetching the tail, then
  reverses the tail's page order (not each page's item order). Partial final
  requests stop each chain at its assigned partition.

The bidirectional probe does not make a separate count request. Partitioning can
require one more item request than ordinary 100-item paging, depending on the
count. Unit fixtures cover ordered delivery with a blocked tail, cancellation
and scheduler restoration, empty/small/odd/even partitions, short counted pages,
backward cursor selection, and preservation of nested field pagination.

```sh
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
GH_PROJECTS_TUI_LIVE_VIEW=BOARD_VIEW_NUMBER \
GH_PROJECTS_TUI_LIVE_ROUNDS=3 \
GH_PROJECTS_TUI_LIVE_LOADING=1 \
go test -tags live ./internal/github \
  -run '^TestLiveBoardFetchComparison$/(round_[0-9]+|verification)/(sequential_GraphQL|50-per-page_GraphQL|25-first_GraphQL|current_GraphQL)$' \
  -count=1 -v -timeout=10m
```

At the time of this experiment, production still used sequential 100-item reads.
Counts are not snapshot tokens:
external edits during a parallel read can invalidate a partition. A production
strategy would need explicit refresh/fallback behavior for membership changes,
continued stale-generation cancellation, and bounded read parallelism without
relaxing write serialization. The experiment's idle-client permit swap is not
that production policy.

Three rotated rounds on the same board, now at 660 items, produced:

| Strategy | Median first page | Median full fetch (range) | Item requests | GraphQL points |
| --- | ---: | ---: | ---: | ---: |
| Current 100-item pages | 2.074 s | 16.316 s (15.834–16.702) | 7 | 35 |
| 50-item pages throughout | 1.397 s | 21.699 s (20.398–22.431) | 14 | 42 |
| 25 first, then 100 | 1.024 s | 15.786 s (14.849–16.182) | 8 | 36 |
| Count plus front/back parallel reads | 2.553 s | 9.063 s (9.006–10.448) | 7 | 33 |

All strategies transferred approximately 0.63 MiB of decoded item responses.
Membership, canonical page order, saved-view metadata, and compared card data
matched in every round. The final baseline matched the initial one as well.
Median times including metadata setup were 16.993, 22.183, 16.119, and 9.419
seconds, respectively.

Smaller pages throughout move the latency around: the first response improves,
but doubling the request count makes completion about 33% slower. A small first
page halves time to first cards without establishing a substantial full-fetch
improvement. The bidirectional strategy reduces median completion time by about
**44%**, while fetching the same data with the same item-request count on this
sample. Its first page is not faster, so this is a completion improvement rather
than an initial-display improvement.

This supports pursuing bounded bidirectional reads for large boards, subject to
the production requirements above. Combining the small-first-page and
bidirectional strategies has not been measured; do not claim the 1.0-second
first page and 9.1-second completion together. No production loading policy was
changed by this experiment.

### Production follow-up: bounded bidirectional reads (2026-11-21)

`Client.OpenBoardItems` is now used for initial board/table loading and
post-action view refreshes. It returns lazy, ordered batches; the UI does not
manage either cursor chain. The first 100-card read includes a filtered count.
Views below 300 items continue forward sequentially. Larger views read the
remaining front and tail concurrently.

The production reader adds safeguards beyond the prototype:

- Each bounded page checks its count and requested length.
- The tail deliberately includes one overlapping join item, which must match
  the last forward item. Only that validated overlap is removed.
- Duplicate/missing identities, repeated cursors, count/length changes, failed
  optimized reads, or an invalid join cancel and join the tail worker, then
  discard the stale read-cache generation and restart once with uncached forward
  paging using the unchanged saved filter.
- The first fallback batch explicitly replaces the prior prefix. Board/table
  focus is restored by item identity; post-action refreshes also discard the
  abandoned partial result.
- A failed authoritative fallback is surfaced rather than retried indefinitely.
  Navigation and cancellation do not start a fallback. Closing a reader cancels
  without waiting on the UI thread.

The shared scheduler uses a two-slot weighted gate: a read reserves one slot,
and a write reserves both. The gate remains held through response consumption.
This caps all client reads at two while keeping writes exclusive, paced, and
never automatically replayed. Existing cooldowns and quota/cost instrumentation
remain shared. There is no runtime permit swap.

These checks detect important forms of concurrent change, but GitHub does not
offer an atomic project snapshot: equal counts and a valid join cannot prove
that no external edits occurred elsewhere during the read.

The production reader replaced the prototype implementation in the benchmark.
`current GraphQL` now uses the exact reader used by the app; `sequential GraphQL`
retains the prior stateless forward path for comparison:

```sh
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
GH_PROJECTS_TUI_LIVE_VIEW=BOARD_VIEW_NUMBER \
GH_PROJECTS_TUI_LIVE_ROUNDS=3 \
go test -tags live ./internal/github \
  -run '^TestLiveBoardFetchComparison$/(round_[0-9]+|verification)/(current_GraphQL|sequential_GraphQL)$' \
  -count=1 -v -timeout=10m
```

Three alternating-order rounds on the 660-item board measured:

| Production path | Median first batch | Median complete fetch (range) | Median including setup |
| --- | ---: | ---: | ---: |
| Bidirectional reader with safeguards | 1.933 s | 8.304 s (7.152–10.134) | 8.568 s |
| Previous sequential reader | 1.853 s | 14.004 s (13.394–14.310) | 14.437 s |

The production implementation reduced median fetch time by about **41%** in
this sample. Both paths made seven item HTTP requests and transferred about
0.63 MiB; measured GraphQL costs were 33 versus 35 points. Every comparison
matched membership, canonical order, view metadata, and other card data, with
zero sequential fallback restarts. The final baseline also matched the initial
one. Item batches in the UI are ordered deliveries, not necessarily one-to-one
with HTTP requests (a validation-only overlap can require an extra request).

Fixtures cover count/join/identity failures, uncached restart without mixing
prefixes, failed fallback termination, cancellation, UI focus and stale-result
handling, and exclusive writes during concurrent reads. Live validation remains
read-only; no mutations were submitted to the work board.
