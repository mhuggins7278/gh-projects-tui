# Read-only terminal roadmap proposal (#22)

**Status: proposed, not implemented.** Saved roadmaps still stop in the view
picker. This document proposes a deliberately limited timeline; it does not
authorize enabling it or creating project data to obtain test samples.

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
- Show items with both endpoints unset in an undated section. Unavailable fields
  or inaccessible content get an unavailable section, not an undated label.
- Preserve verified saved row sorting, with project position as a stable tie
  breaker. With no explicit saved sort, preserve project position; do not
  silently replace it with start-date sorting.
- Keep selection stable by item ID during paging. Counts are explicitly loaded
  counts until pagination finishes. Local search must not alter saved filtering.
- On terminals too narrow for the title gutter plus all three months, use a
  labeled date-list fallback rather than a misleading compressed chart.
- Label zoom/range as local defaults when their saved state cannot be restored.
  Do not claim exact web parity. Read-only navigation, detail, refresh, and open
  in GitHub are available; rescheduling/reordering controls are not.

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
| [#27](https://github.com/mhuggins7278/gh-projects-tui/issues/27) | Bounded renderer, clipping, date-list fallback, and fixtures | Synthetic work can proceed; live use requires #25/#26 |
| [#28](https://github.com/mhuggins7278/gh-projects-tui/issues/28) | Safe read-only integration, navigation, and responsiveness | #25, #26, #27 |
| [#30](https://github.com/mhuggins7278/gh-projects-tui/issues/30) | Iteration endpoints (deferred) | Initial verified timeline; independent of grouping |
| [#29](https://github.com/mhuggins7278/gh-projects-tui/issues/29) | Single-select grouping (deferred) | Initial verified timeline; independent of iteration endpoints |

Investigation review/landing remains tracked by #22. Creating these follow-ups
does not enable saved roadmap rendering or relax any compatibility gate.
