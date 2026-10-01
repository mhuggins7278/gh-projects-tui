# Read-only terminal roadmap scope

**2026-10-01: the user approved the initial unfiltered, unsorted, ungrouped DATE
slice.** This supersedes the broader enablement gate recorded on 2026-09-30.
A subsequent populated comparison verified `is:issue` and one mapped start DATE
ASC sort. Other filters/sorts remain blocked while #26's comparisons proceed.

## Supported boundary

- Explicit local host/project/view mappings select start and target IDs. Both
  must resolve uniquely to complete project DATE definitions. No names, visible
  field order, or populated values are used to guess endpoint roles.
- Only saved roadmaps with an empty or `is:issue` filter and no grouping open.
  Sorting is absent or one ASC sort on the mapped start DATE field. Both picker and direct startup enforce the boundary.
- Complete item field connections and nested pagination preserve incoming project-position
  order and date strings. A verified explicit start DATE ASC sort keeps unset
  values last and POSITION ties; no sort is invented for unsorted views. No
  unfiltered substitute is applied.
- Inclusive bars, same-day/partial points, and undated nonplacement use the bounded
  [populated web/API evidence](roadmap-date-verification-2026-09-30.md).
- Reversed/invalid dates are labeled invalid without swapping endpoints; missing
  content, unavailable values, and duplicate mapped value IDs are unavailable.
  These are conservative client policies for sample gaps, not verified web geometry.
- A local three-calendar-month window starts at the current month. `h/l` shifts
  one month and `g` returns to the current month. Saved zoom/range, markers,
  slicing, and field sums are unavailable and disclosed.
- Clip bars with directional indicators; keep wholly out-of-range rows selectable
  and counted. Widths below 60 columns use a date-list fallback. Exact selected
  endpoint values remain visible within available terminal dimensions.
- `j/k` selects rows, `/` searches loaded rows, `enter` opens lazy detail, `o`
  opens the selected item/project, and `r` refreshes saved metadata and items.
  Selection follows item ID across pages/search/refresh. Counts remain loaded
  counts until pagination completes; search does not alter the saved filter.
- Refresh rejects newly unsupported filters/sorts/groups/mappings; cancellation,
  read scopes, generations, and detail request IDs reject obsolete results.
- Timelines remain read-only even with write permission. Project moves/reorders,
  archive/remove, rescheduling, and issue comment/close/reopen controls are gated.

The [2026-10-01 filter/order evidence](roadmap-filter-order-verification-2026-10-01.md)
records the 71-row membership, populated/null/tie order comparison, and remaining
verification limits.

Configuration and controls: [local mapping guide](local-roadmap-mappings.md).
The pure renderer is in `internal/ui/date_timeline.go`; its live adapter is in
`internal/ui/timeline.go`. Navigation reuses the row-selection lifecycle used by
tables, without table mutation controls.

## Remaining work

| Issue | Status |
| --- | --- |
| [#22](https://github.com/mhuggins7278/gh-projects-tui/issues/22) | Investigation complete |
| [#25](https://github.com/mhuggins7278/gh-projects-tui/issues/25) | Automatic saved endpoint recovery unavailable; explicit local mappings approved |
| [#26](https://github.com/mhuggins7278/gh-projects-tui/issues/26) | Basic DATE placement plus populated is:issue / mapped start DATE ASC (null/ties) verified; other filters/sorts and live multi-page filtered comparisons remain gated |
| [#27](https://github.com/mhuggins7278/gh-projects-tui/issues/27) | Bounded renderer complete |
| [#28](https://github.com/mhuggins7278/gh-projects-tui/issues/28) | Read-only integration complete for the initial slice and verified is:issue / start DATE ASC extension; live smoke and large-fixture measurements recorded |
| [#29](https://github.com/mhuggins7278/gh-projects-tui/issues/29) | Initial integration prerequisite satisfied; sample search found no grouped DATE roadmap; populated/unset/order verification remains gated |
| [#30](https://github.com/mhuggins7278/gh-projects-tui/issues/30) | Iteration endpoints deferred pending active/completed/missing/mixed conversion verification |

The [grouping sample search](roadmap-grouping-investigation-2026-10-01.md) records
the remaining #29 verification prerequisite.

No existing project data is created or edited to manufacture verification cases.
Do not use undocumented web endpoints, browser credentials, or authentication
changes to obtain missing saved settings. Local mapping does not imply saved-view
parity. Historical API/sample results remain in the linked evidence notes.

## Verification

The [10,000-row integration measurement](roadmap-performance-2026-10-01.md)
records progressive rendering, input latency, stable selection, and logical
page/detail call counts in position and verified DATE-sort modes.

Model fixtures cover both owner kinds at startup, metadata/mapping compatibility,
progressive pagination, full endpoint reads, project-position order, selected
identity, local search, range movement, resizing, refresh rejection, obsolete
responses, and read-only guards. Renderer fixtures cover inclusive date rules,
leap days, month/year boundaries, partial/unset/invalid/unavailable cases,
clipping, narrow/short terminals, Unicode, and 10,000 items.

```sh
go test -race ./...
go vet ./...
GH_PROJECTS_TUI_TIMELINE_PREVIEW=1 go test ./internal/ui -run '^TestTimelineSyntheticPreview$' -count=1 -v
go test ./internal/ui -run '^$' -bench '^BenchmarkDateTimelineLargeFixture$' -benchmem
```
