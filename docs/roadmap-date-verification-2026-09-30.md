# Existing populated roadmap verification — 2026-09-30

**Result: basic DATE placement verified on one populated sample; #26 is not complete.** This read-only
probe used an existing public organization project. It did not create or edit
project data, save view changes, change authentication, or use undocumented
endpoints. This note retains aggregate counts and verification boundaries;
project/field names, node IDs, item content, and exact dates are omitted.

## Endpoint selection and population

The web roadmap's **Date fields** menu showed a selected DATE field in each of
the Start date and Target date sections. A third DATE field was unselected.
The two observed selections each resolved uniquely to a DATE field ID in the
supported GraphQL project-field definitions. This is an explicit, web-observed
configuration for a local sample; the API did not recover the saved endpoint
roles. Names alone did not determine those roles.

The project returned 17 field definitions, including three DATE fields. Its
selected view was `ROADMAP_LAYOUT`, with an empty saved filter and zero grouping,
vertical-grouping, or sorting fields. The web showed quarter zoom and no sort
applied. The local renderer's three-month viewport remains a separate local
display choice.

The supported `items(first: 100, after: ..., orderBy: {field: POSITION,
direction: ASC})` connection returned 540 unique items over six pages. Every
item's queried field-values connection was complete; no item IDs were duplicated
and every continuation cursor advanced. Across all three DATE fields, there were
1,565 populated date-value nodes.

Using only the two explicitly observed endpoint selections, the existing items
fell into these cases:

| Endpoint case | Items |
| --- | ---: |
| Two distinct dates in ascending order | 507 |
| Same-day endpoints | 12 |
| Target only | 1 |
| Start only | 8 |
| Neither endpoint populated | 12 |
| Reversed endpoints | 0 |
| Unavailable item content | 0 |

These counts establish that populated and partial-date samples exist. They do
not establish how the web draws each case. No durations or missing endpoints
were inferred.

## Order and placement boundaries

The first 21 `POSITION ASC` items matched all 21 initially visible web rows in
exact order. This supports the no-sort project-position order for that observed
sample. It does not establish sorted null/tie behavior, grouped order, or the
position of every virtualized web row.

The first attempt ended when the browser became unavailable. On resumption,
browser navigation succeeded after one permitted retry of an automatic access
check timeout. No authentication, permissions, or browser setup was changed.
The API baseline and selected endpoint roles were checked again before the
comparisons below.

### Resumed placement comparisons

The comparison used screenshots and the actual rendered DOM: calendar column
`datetime` attributes, day widths, date-cell attributes, bar styles/rectangles,
and displayed date tooltips. It did not read application state, script payloads,
internal endpoints, or browser credentials. Quarter zoom used 16 pixels per day
with weekly labels 112 pixels apart.

| Existing case | Web/API comparison | Supported observation |
| --- | --- | --- |
| Two endpoints across a month boundary | API endpoints matched the rendered cell and tooltip; the start aligned with the calendar day, and an 11-day inclusive span measured 176 pixels | Both endpoint days are included in this range |
| Two endpoints across a year boundary | API endpoints matched the rendered cell and tooltip; the start aligned with the calendar day, and a 72-day inclusive span measured 1,152 pixels | Calendar placement continues across the year boundary without dropping a day |
| Longer range across multiple months/year | API endpoints matched the rendered cell; a 481-day inclusive span measured 7,696 pixels | Longer bars use the same inclusive daily scale; part of the bar was outside the viewport |
| Same-day endpoints | Both API dates matched; the rendered start/end attributes were that date, the inline width was one day, and the tooltip showed one date | One-day placement, without an invented duration |
| Target only | Start was absent in the API; the rendered start/end attributes both used the populated target, with a one-day inline width and single-date tooltip | Placement on the target date |
| Start only | Target was absent in the API; the rendered start/end attributes both used the populated start, with a one-day inline width and single-date tooltip | Placement on the start date |
| Neither endpoint populated | Both API values were absent; the title row remained, with no date cell, bar, or navigation button | No timeline placement |

Single-day marks can have an 18-pixel outer rectangle despite a 16-pixel inline
day width because of the rendered label styling. This is not evidence for a
two-day duration. The terminal's point glyph is a coarse representation of the
observed one-day placement, not a reproduction of GitHub's label geometry.

Temporary title filters located virtualized samples. They were discarded, and
the original URL, empty filter, and absence of a Discard button were confirmed.
No saved filter/sort, endpoint role, item, or date was changed. These temporary
searches do not establish parity for a nonempty **saved** filter.

`TestTimelineObservedDatePlacementRules` encodes the inclusive month/year and
same-day/partial rules with invented dates. Existing tests preserve separate
undated, invalid, unavailable, and clipping policies. Fixture coverage does not
convert the unobserved cases below into verified GitHub behavior.

## Remaining checks

A bounded discovery of five published public projects found one other saved
roadmap with a nonempty supported label filter. Its grouping and sort metadata
were empty. An exact-filter GraphQL `POSITION ASC` query returned zero nodes in
one complete page (`hasNextPage=false`, no continuation cursor); the documented
REST saved-view item GET also returned zero items. The web showed that same
saved filter, no unsaved-change control, and no item rows. This establishes one
empty-result membership comparison, not populated filtered membership or
multi-page parity. No suitable saved-sort roadmap emerged from this search.

- Obtain existing unavailable/reversed cases, or preserve an explicit policy
  boundary for cases the sample does not cover; never manufacture project data
  to fill the gaps.
- Verify any saved sorting/null/tie behavior before admitting sorted views.
- For populated nonempty saved filters, retain the separate server-filter
  contract evidence and compare a real saved roadmap's membership. The first
  sample's empty filter and the second sample's empty result do not establish
  populated filtered membership or multi-page web parity.

The basic DATE bar/point rules now have populated web/API evidence. The complete
live contract still lacks populated saved-filter/sort comparisons and sample-gap policies.
Local endpoint mapping and this progress do not complete #26 or enable live
integration in #28. Grouping and iteration
placement remain separate verification work for #29 and #30.
