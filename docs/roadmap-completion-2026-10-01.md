# Read-only timeline completion — 2026-10-01

The user explicitly authorized synthetic repository issues, sandbox project
items/fields/views, and finishing the feature. This supersedes older requirements
that all remaining comparisons use only pre-existing samples. The demo remains
private; authentication, permissions, and work projects were not changed.
Sanitized results below contain only counts and invented calendar semantics.

## Final supported contract

- Explicit host/project/view mappings supply both endpoint field IDs. DATE,
  ITERATION, and mixed roles resolve uniquely against project definitions.
- Project field pagination now reads current and completed iteration definitions,
  including non-visible fields. Iteration membership is resolved by ID. Start
  roles use the first day; target roles use start plus duration minus one.
  Missing membership stays unset; unavailable/unknown/duplicate values do not
  turn into guessed dates. Invalid definitions block entry/refresh.
- Saved filters use the existing board/table compatibility grammar and are
  forwarded unchanged to the server on every page. Local search only narrows
  loaded rows. Unsupported grammar remains blocked.
- Up to two distinct supported ASC/DESC sort fields reuse stable board/table
  sorting: unset last, original project-position ties. Sort order does not change
  endpoint roles. Unsupported/inconsistent metadata remains blocked.
- One single-select group uses API option order, unset last, all expanded, with
  empty-section loaded counts and a visible fallback for unavailable memberships.
  Multiple/vertical/iteration/multi-valued grouping remains outside this feature.
- Calendar range, row/detail/browser navigation, search, progressive rendering,
  refresh identity, resize fallbacks and all read-only guards remain active.

## Authorized sandbox comparisons

Twelve synthetic issues exercise normal/same-day/partial/unset dates, equal start
values, cross-month spans, outside-range dates, populated Status options and an
unset group. Three iteration definitions cover completed/current/year-boundary
ranges, with one item deliberately lacking iteration membership.

The web displays a seven-day current iteration from Oct 1 through Oct 7, a
completed iteration from Sep 24 through Sep 30, and a Dec 28 iteration through
Jan 3 of the next year. Mixed iteration-start/DATE-target and DATE-start/iteration-
target roles follow the same role-specific days. A missing iteration with a
populated DATE endpoint remains a single-day point; missing membership at both
iteration roles has no bar. These comparisons establish calendar conversion,
not saved zoom or pixel geometry.

A populated label/Status conjunction returns three matching rows. Start DATE
DESC puts the two populated rows in descending order and the unset start last.
The grouped twelve-row DATE view matches all web rows and counts in option
order: five Todo, three In Progress, three Done, and one unset. Within-group
start ASC order, stable ties, and unset starts last match the client policy.
Custom group order/collapse remains unavailable.

The twelve-row label-filtered view with start DATE DESC and target DATE ASC
matches all twelve web rows through the actual client; target dates break the
three-way start tie without changing endpoint roles.

A read-only GraphQL probe forced three-row pages. The label subset and a
repo-qualified equivalent each return twelve distinct rows over four pages;
negative Status returns nine over three pages; Status and closed-issue subsets
return three each on one page. The exact query string is retained on each cursor.
This is API pagination evidence, not a claim that the production client's
100-row page size paginated the twelve-row fixture.

The optional actual-client UI probe reports only counts and compares supplied
web order against complete saved-filter membership and local ordering. It can
run on an explicitly supplied accessible sandbox, including a private sandbox;
it submits no writes and retains no metadata. See `internal/ui/timeline_live_test.go`.

## Validation and limits

Regression coverage includes two owner adapters, non-visible iteration metadata
across project pages, inclusive/leap/year boundaries, explicit mixed roles,
malformed/unknown/unavailable membership, broader saved filters across item pages,
stable null/tie/secondary sorting, grouping window counts, selection, refresh and
read-only controls. Full race tests, vet, live-tag vet and a binary build pass. Actual-client probes
compare web order and endpoint resolution; live terminal navigation/detail/
refresh checks exercise the sandbox views.

This closes the scoped read-only implementation tracked by #26/#29/#30. Existing
large-fixture performance evidence remains in `roadmap-performance-2026-10-01.md`.
It does not claim every GitHub filter/sort/type, an independent work-organization
web comparison, or unavailable saved display state. Automatic saved endpoint
recovery remains unavailable (#25); explicit local mappings supply endpoint roles.
Saved zoom/range, markers, slicing, field sums, collapsed/custom group order and
rescheduling/writes are not part of the implementation.
