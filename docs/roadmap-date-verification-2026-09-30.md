# Existing populated roadmap verification — 2026-09-30

**Result: useful populated evidence, but #26 is not complete.** This read-only
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

The initial web view exposed a navigation date matching the populated start of
one two-endpoint row and the populated target of the target-only row. Those
navigation labels provide a limited anchor check. They do not demonstrate an
inclusive end boundary, the visual width of same-day or partial-date items, or
the complete bar/point placement contract.

The browser became unavailable before further placement comparisons could be
performed; its available-surface inventory then contained no browser. API
pagination completed independently. No authentication or browser setup was
changed to bypass that limitation.

## Remaining checks

- Compare two-endpoint and same-day geometry with actual API dates, including
  month/year boundaries and whether the target day is inclusive.
- Compare each one-endpoint case and undated placement in the web. Date
  population and a navigation anchor alone do not prove the proposed point rule.
- Obtain existing unavailable/reversed cases, or preserve an explicit policy
  boundary for cases the sample does not cover; never manufacture project data
  to fill the gaps.
- Verify any saved sorting/null/tie behavior before admitting sorted views.
- For nonempty saved filters, retain the separate server-filter contract
  evidence and compare a real saved roadmap's membership. This roadmap's empty
  saved filter supplies no nonempty-filter parity evidence.

The synthetic renderer's bar/point conventions therefore remain provisional.
Local endpoint mapping removes the field-selection ambiguity, but it does not
complete #26 or authorize live integration in #28. Grouping and iteration
placement remain separate verification work for #29 and #30.
