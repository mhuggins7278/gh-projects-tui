# Read-only terminal roadmap scope

The read-only timeline implementation is complete for the supported contract.
The user approved general enablement and synthetic sandbox validation on
2026-10-01. This supersedes the initial DATE-only/filter/order gates. Evidence,
regressions, and remaining product boundaries are in the
[completion note](roadmap-completion-2026-10-01.md).

## Supported boundary

- Explicit local host/project/view mappings supply start and target field IDs.
  Both must resolve uniquely to DATE or valid ITERATION project definitions.
  Names, visible columns, and populated values never substitute for endpoint IDs.
- DATE and current/completed ITERATION endpoints can be mixed. Iteration roles
  resolve by membership ID: first day for start, inclusive final day for target.
  Unknown/duplicate/unavailable memberships remain unavailable; unset stays unset.
- Saved filters use the board/table supported grammar, forwarded unchanged to
  GitHub on every page. Local search only narrows loaded rows.
- Up to two distinct supported ASC/DESC sort fields preserve unset-last and stable
  project-position ties. Sorting does not redefine endpoint roles.
- One single-select grouping field uses a disclosed local policy: API option
  order, unset last, all expanded. Empty groups retain loaded counts; unsupported
  memberships remain visible in an unavailable section.
- Inclusive bars, same-day/partial points, clipped/outside rows, and exact selected
  endpoint days use a local three-calendar-month viewport. `j/k` selects rows,
  `h/l` moves months, `g` returns to the current month, `/` searches loaded rows,
  `enter` opens lazy detail, `o` opens the item/project, and `r` refreshes metadata
  and items. Selection follows item ID across pages/search/refresh.
- Refresh rejects newly unsupported metadata. Cancellation, generations and
  detail request identities reject stale results. Counts are loaded counts.
- Timelines stay read-only, including project and issue-detail mutation controls,
  even with write permission. No rescheduling is implemented.

Configuration and controls: [mapping guide](local-roadmap-mappings.md).

## Completed tickets

| Issue | Result |
| --- | --- |
| #22 | Investigation complete |
| #25 | Automatic saved endpoint recovery unavailable; explicit local mappings supply roles |
| #26 | Shared saved filters and up to two supported ASC/DESC sorts enabled; populated sandbox membership/order and small-page API comparisons recorded |
| #27 | Bounded calendar/date-list renderer complete |
| #28 | Read-only lifecycle, live smoke, and 10,000-row performance checks complete |
| #29 | Grouped live rendering and populated/unset/order comparison complete within the disclosed local policy |
| #30 | DATE/ITERATION/mixed endpoints enabled with inclusive current/completed/boundary/missing conversion checks |

## Deliberate limits

Saved zoom/range, markers, slicing, field sums, collapsed/custom group ordering
and automatic saved endpoint selection are unavailable through the documented
API. Multiple/vertical/iteration/multi-valued grouping and unsupported filter or
sort types remain blocked. Invalid/reversed DATE values are labeled invalid
without swapping endpoints. These conservative policies do not claim exact web
pixel geometry or independent work-organization web parity.

Historical evidence is preserved in the DATE, filter/order, grouping, and
[large-fixture performance](roadmap-performance-2026-10-01.md) notes. Their earlier
no-mutation/sample gates are superseded only for the explicitly authorized
synthetic sandbox work described in the completion note.
