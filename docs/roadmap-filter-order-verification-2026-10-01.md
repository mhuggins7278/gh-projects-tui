# Populated saved-filter and DATE ASC roadmap comparison — 2026-10-01

**Result: populated `is:issue` membership and one mapped-start DATE ASC sort
match an existing public user roadmap.** This extends the initial unfiltered,
unsorted subset; it does not verify arbitrary filters or sorts, or complete #26.

## Source and read boundary

A published project link identified the sample. Public visibility was checked
before retrieving project metadata/items. A bounded discovery checked eight
published project candidates, all public: three exposed roadmap layouts, one had
both a nonempty saved filter and a DATE ASC sort, and five had no roadmap layout.
No project settings, items, dates, authentication, or permissions were changed.

The first batch discovery attempt was rejected by automatic approval review
because it proposed retaining metadata before checking visibility. It did not
execute. The revised script checked only the public boolean first, and read
configuration only after public visibility was confirmed.

Only aggregate counts, contract names, and comparison boundaries are retained
here. Project/field names, node IDs, issue content, and exact sample dates are
omitted. Temporary sample data and local mappings are outside the repository.

## Saved metadata and explicit endpoints

The existing saved roadmap had:

- The exact saved filter `is:issue` in both API metadata and the visible filter.
- No horizontal or vertical grouping; the web View menu showed no grouping.
- One ascending DATE sort. The web showed the start DATE field sorted ascending;
  its definition ID matched the mapped start role.
- Start and target DATE roles visibly selected in the web View menu. Each name
  resolved uniquely to a DATE definition ID, supplying an explicit local mapping.
  The API did not recover the saved role choices.
- Quarter zoom and milestone markers. These are omitted local display settings,
  visibly disclosed rather than claimed as restored by the terminal.

## Complete reads and membership

| Read | Unique items | Pages | Content |
| --- | ---: | ---: | --- |
| Unfiltered `POSITION ASC` baseline | 103 | 2 | 71 issues, 32 PRs |
| Exact saved `is:issue` filter, `POSITION ASC` | 71 | 1 | 71 issues |
| Web saved roadmap | 71 | All row windows inspected | Same 71 issues |

All queried nested field-value connections were complete. Item IDs were unique,
and the baseline continuation cursor advanced. Filtered membership equaled
exactly the baseline issue subset; all 32 PRs were excluded. The filter was never
replaced with a local issue-kind predicate in the application loading path.

`TestLiveRoadmapSavedIssueFilter` repeated the reads through the application's
actual GitHub client and confirmed these counts and ID-set membership. It logs
aggregates only, checks public visibility before metadata reads, and remains
opt-in under the `live` build tag.

## Row order, unset values, and ties

The filtered POSITION result contained 65 populated start dates, six unset start
values, and 16 groups of tied populated dates. A stable ascending date comparison
with unset last and incoming project-position ties matched all 71 web rows.
The resulting order differed from unsorted POSITION order, so this was not a
sample where ignoring the saved sort happened to give the same result.

The web's rendered row-header title/number labels were normalized only for
whitespace; all 71 were unique. Every virtualized row window was compared against
the corresponding complete expected label sequence. Overlapping windows covered
all 71 labels with no missing or extra rows; all window sequences matched. The
six unset rows were last and retained relative POSITION order, as did populated
DATE ties. No hidden application state or undocumented endpoints were read.

Model regressions use invented IDs/dates for both owner kinds to verify exact
filter forwarding on each page, date/null/tie order, selection across appended
pages and refresh, detail targeting, and the disclosed saved filter/order.
They do not establish live multi-page filtered parity.

## Terminal integration smoke test

The locally built application opened the same saved view through direct startup
with the web-observed local mappings. It loaded 71 rows and displayed the exact
saved filter, ascending start-date order, and local range/display limitations.
The current local viewport classified 65 dated rows as outside the range and six
as undated, without dropping them. Row navigation changed the selected endpoint
readout, issue detail loaded for that row, and metadata/item refresh retained the
selected identity. No project mutation controls were used.

The first 80×24 run exposed an overly small height budget when filter/sort
notices were present. The adapter now budgets from the body canvas; a regression
and a second terminal run confirm the loaded count, range, rows, and exact
selected endpoint values remain visible at 80×24.

## Enabled subset and remaining gates

Allowed roadmap saved filters are empty or exactly `is:issue` (surrounding
whitespace is accepted and forwarded unchanged). Sorting is absent or exactly
one ASC sort on the explicitly mapped start DATE field. Other predicates,
conjunctions, DESC, target/other DATE sorting, and secondary sorts remain blocked.
Grouping and iteration endpoints remain deferred.

This sample's filtered result fits one page. The two-page **unfiltered** baseline
is not live multi-page filtered parity. The fixture checks forwarding/selection
across filtered pages, while a representative populated multi-page saved roadmap
remains a #26 verification gap. This single user sample also does not independently
verify organization-owned filtered roadmap web behavior. Broader filters/sorts
require their own populated web/API comparisons before compatibility expands.
