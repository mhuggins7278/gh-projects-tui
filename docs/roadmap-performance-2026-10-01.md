# Read-only timeline integration performance — 2026-10-01

This invented fixture completes #28's large-project integration measurement.
It is not a network latency or GitHub GraphQL cost measurement. No live project
items, identifiers, titles, or dates are used.

## Progressive reads and selection

`TestLargeTimelineProgressiveReadsAndInputResponsiveness` runs 10,000 rows at
123×40 in both POSITION order and the admitted `is:issue` / start DATE ASC mode.
Rows include ties and unset endpoints. Pages contain 100 rows.

The first page renders before later pages complete, with a loaded count and
loading indicator. A subsequent page and lazy detail fetch are held behind
independent gates while row navigation, local search, and rendering run. Closing
detail discards its obsolete response. On completion, selected identity stays
stable, all 10,000 rows are loaded, and a far-end selected row remains visible.
Both modes made exactly **100 logical page-source calls and one detail-source
call**. The fixture does not assert these are total HTTP/GraphQL requests:
metadata and nested-field/detail pagination can add real network requests.

Reference run: Linux/amd64, Intel Core Ultra 9 185H, normal non-race tests.

| Mode | First page update | Input + render p95 during blocked reads | Fully loaded input + render p95 |
| --- | ---: | ---: | ---: |
| POSITION | 0.058 ms | 2.46 ms | 8.43 ms |
| Start DATE ASC | 0.070 ms | 2.57 ms | 28.91 ms |

These are local reference measurements, not universal latency guarantees. Timing
is logged rather than asserted against a machine-dependent hard deadline; request
counts, loading state, stale-response rejection, and selection are asserted.

## Model navigation benchmark

`BenchmarkTimelineModelNavigation` measures one row-navigation operation plus
full model rendering, including sorting/search, rather than only the pure
renderer. Allocs are included. Reference run used a 200 ms benchmark interval.

| Rows | Sorting | Search | Mean per operation |
| ---: | --- | --- | ---: |
| 1,000 | POSITION | Off | 2.61 ms |
| 1,000 | POSITION | On | 3.89 ms |
| 1,000 | Start DATE ASC | Off | 3.76 ms |
| 1,000 | Start DATE ASC | On | 4.29 ms |
| 10,000 | POSITION | Off | 6.89 ms |
| 10,000 | POSITION | On | 18.24 ms |
| 10,000 | Start DATE ASC | Off | 25.81 ms |
| 10,000 | Start DATE ASC | On | 18.70 ms |

The largest unsearched sorted case allocated about 5.49 MB per operation; searched
10,000-row cases allocated about 7.47 MB. Sorting and searching still scan loaded
rows, so these results do not imply constant work or an unbounded scalability
claim. The measured 10,000-row fixture remains below the existing 100 ms reference
input target, and no extra rendering-driven data reads occur.

Reproduce:

```sh
go test ./internal/ui -run '^TestLargeTimeline' -count=1 -v
go test ./internal/ui -run '^$' -bench '^BenchmarkTimelineModelNavigation$' -benchmem -benchtime=200ms
go test -race ./...
go vet ./...
go vet -tags live ./...
```

Live read-only integration was separately exercised with the confirmed public
71-row sample, including startup, selected-row detail, refresh, and 80×24 layout.
See [the filter/order and terminal evidence](roadmap-filter-order-verification-2026-10-01.md).
