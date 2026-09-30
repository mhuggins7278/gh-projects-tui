# Read-only terminal roadmap proposal (#22)

**Status: fixture-backed renderer implemented (#27); saved-roadmap integration
blocked.** Saved roadmaps still stop in the view picker. The pure renderer uses
explicit synthetic endpoints and provisional placement conventions. It does
not authorize enabling live roadmaps or creating project data to obtain samples.

## Decision and evidence

Do not infer start/target fields from their names, visible-field order, the only
two DATE fields in a project, or populated item values. GitHub lets users select
date **or iteration** fields for each endpoint, but the introspected GitHub.com
view/configuration schema exposes neither selection. It also lacks saved zoom,
viewport range, markers, slicing, collapsed groups, and custom group order.

Generic date/iteration values and project position are readable. That is enough
for a future explicitly configured timeline, not enough to mirror a saved
roadmap. Local endpoint overrides would be a separate product decision, not a
silent workaround for missing saved-view metadata.

The [#25 endpoint-source review](api-contract.md#saved-endpoint-mapping-decision-25)
also checked the documented REST Projects APIs and records a no-go decision as
of 2026-09-29. Saved-view enablement remains blocked; synthetic renderer work is
independent of that decision.

See [the investigation and live probe results](api-contract.md#roadmap-layout-investigation).
GitHub's [roadmap documentation](https://docs.github.com/en/issues/planning-and-tracking-with-projects/customizing-views-in-your-project/customizing-the-roadmap-layout)
describes endpoint selection, Month/Quarter/Year zoom, markers, grouping, sorting,
slicing, and field sums. Documentation describes the web UI, not an API contract
for their saved values or date-boundary behavior.

## Proposed first representation

- A fixed three-calendar-month viewport with month headers and daily buckets
  within each month. Bucket width adapts to terminal width; dates do not acquire
  time-of-day or local-time-zone offsets.
- A title gutter and one row per issue, PR, or draft. Selection exposes exact
  endpoint dates and any unavailable values in detail; coarse buckets must not
  be presented as exact dates.
- Two valid endpoints draw an inclusive bar. One endpoint draws a point. These
  are **proposed conventions requiring web verification**, not confirmed GitHub
  behavior. Reversed/invalid endpoints get an explicit invalid-date row; do not
  swap endpoints or invent a duration.
- Clip bars at viewport boundaries with directional indicators. Count items
  wholly outside the range separately; allow their selection without pretending
  they are undated or absent from the saved filter.
- Label rows with both endpoints unset as undated. Label unavailable fields or
  inaccessible content as unavailable. Preserve the supplied order rather than
  regrouping these rows and changing the saved sort.
- Preserve verified saved row sorting, with project position as a stable tie
  breaker. With no explicit saved sort, preserve project position; do not
  silently replace it with start-date sorting.
- Keep selection stable by item ID during paging. Counts are explicitly loaded
  counts until pagination finishes. Local search must not alter saved filtering.
- On terminals too narrow for the title gutter plus all three months, use a
  labeled date-list fallback rather than a misleading compressed chart.
- Label zoom/range as local defaults when their saved state cannot be restored.
  Do not claim exact web parity. Read-only navigation, detail, refresh, and open
  in GitHub belong to the later integration; the pure renderer has no controls.

## Implemented synthetic renderer (#27)

`internal/ui/date_timeline.go` accepts ordered item rows, explicit start/target
values with separate availability flags, a starting month, terminal dimensions,
selection by item ID, and a loading flag. It does not inspect project fields,
infer endpoint roles, read GitHub, or connect to `Model` or the view picker.
Inclusive bars and one-endpoint points remain **provisional**, subject to #26.

The renderer displays three calendar months with adaptive day buckets and a
title gutter. Selection shows the original start/target values separately,
including `unset` and `unavailable`; the coarse axis is not an exact-date readout.
Clipped bars use `<`/`>` indicators. Rows wholly before or after the viewport
remain in the supplied order, contribute to the outside count, and can be
selected. Undated, unavailable, and invalid/reversed rows have distinct labels.

Widths below 60 columns use a labeled date list. Every line fits the supplied
width; long rows are truncated, with selected endpoint values shown below them.
Short terminals prioritize the selected row, then endpoint lines, then headers.
At extreme dimensions, text and endpoint details can also be truncated. Visible
rows are bounded by height, while loaded counts describe all supplied rows.
Paging can append rows without losing selection by ID. Classification scans the
loaded rows; only the visible window is formatted, without allocating a date-span
array for the full input.

Fixtures cover leap days, month/year boundaries, year-one dates, partial values,
unset/unavailable/invalid/reversed values, both clipping directions, wholly
outside dates, mixed content kinds, input order, Unicode/control characters,
zero/narrow/wide/short terminals, and 10,000 items. All data is invented; these
tests do not establish GitHub web placement or saved sort parity.

```sh
go test ./internal/ui -run '^TestTimeline'
GH_PROJECTS_TUI_TIMELINE_PREVIEW=1 go test ./internal/ui -run '^TestTimelineSyntheticPreview$' -count=1 -v
go test ./internal/ui -run '^$' -bench '^BenchmarkDateTimelineLargeFixture$' -benchmem
```

The [recorded terminal preview](date-timeline-preview.txt) shows wide and narrow
synthetic layouts. #28 still requires #25/#26 before connecting this component
to live saved views. Iterations, grouping, markers, slicing, field sums, and
mutation controls remain outside this component.

## Compatibility gates for a future implementation

| Concern | Initial boundary | Today |
| --- | --- | --- |
| Endpoint identity | Explicit start/target field IDs from a verified supported source; both resolve uniquely to project DATE definitions | Blocked: mapping is not exposed |
| Date values | Complete nested pagination; calendar dates remain unchanged; populated, partial, unset, invalid, and unavailable cases checked against the web | Decoding fixture-backed; placement unverified |
| Iteration endpoints | Defer, including mixed DATE/ITERATION endpoints and completed iterations | Definitions/values available; conversion unverified |
| Saved filtering | Forward only supported expressions unchanged, as with boards/tables | Bounded grammar exists; roadmap membership still needs comparison |
| Row ordering | Project position or a specifically verified saved roadmap sort; no invented date-axis coordinate | Position read verified; roadmap sort/null/tie parity unverified |
| Grouping | Initially no grouping; reject grouped roadmaps rather than flattening them. Later single-select groups need populated/unset and order verification | Generic metadata available; sampled roadmaps ungrouped |
| Vertical/multi-valued grouping | Defer | No roadmap projection contract |
| Zoom/range | Explicitly disclosed local three-month viewport, subject to review of the parity limitation | Saved settings not exposed |
| Markers/slicing/field sums | Defer and disclose that their saved state is unavailable | Not exposed in view configuration |
| Writes | None, even with `viewerCanUpdate=true` | Picker blocks roadmap item loading |

## Evidence required before enabling anything

1. Identify a supported source for the saved endpoint field IDs and verify it on
   the target host. If this remains unavailable, keep all saved roadmaps blocked.
2. Compare existing populated roadmaps with the web, without creating/editing
   project data: two endpoints, same-day endpoints, each one-endpoint case,
   unset/unavailable values, reversed dates, and month/year boundaries.
3. Establish grouping and row-order semantics, including saved sorts, nulls,
   ties, and filtered membership. An unfiltered baseline is not saved-view parity.
4. Exercise multi-page item/nested-field reads, clipped/out-of-range dates, narrow
   terminals, progressive selection, and refresh with synthetic fixtures.
5. Review the local zoom/range and omitted-display-setting limitations. Approval
   of this proposal alone does not verify the missing GitHub semantics.

Do not use undocumented web endpoints, browser-session credentials, or changes
to GitHub authentication as an implicit part of this investigation. Any alternate
configuration source requires a separate, explicit review.

## Tracked follow-up work

| Issue | Scope | Dependencies |
| --- | --- | --- |
| [#25](https://github.com/mhuggins7278/gh-projects-tui/issues/25) | Resolve saved endpoint mapping or record a no-go decision | First enablement gate |
| [#26](https://github.com/mhuggins7278/gh-projects-tui/issues/26) | Verify populated date placement, saved filtering, and row order | Mapping from #25 |
| [#27](https://github.com/mhuggins7278/gh-projects-tui/issues/27) | Bounded renderer, clipping, date-list fallback, and fixtures implemented | Fixture-only; live use requires #25/#26 |
| [#28](https://github.com/mhuggins7278/gh-projects-tui/issues/28) | Safe read-only integration, navigation, and responsiveness | #25, #26, #27 |
| [#30](https://github.com/mhuggins7278/gh-projects-tui/issues/30) | Iteration endpoints (deferred) | Initial verified timeline; independent of grouping |
| [#29](https://github.com/mhuggins7278/gh-projects-tui/issues/29) | Single-select grouping (deferred) | Initial verified timeline; independent of iteration endpoints |

Investigation review/landing remains tracked by #22. Creating these follow-ups
does not enable saved roadmap rendering or relax any compatibility gate.
