# API Contract Gate

Status: partial. This record covers the GitHub.com schema and read probes observed on 2026-09-17 and 2026-09-26, mutation schema introspection observed on 2026-09-21, saved-view schema introspection observed on 2026-09-22, and the dated follow-up checks below through 2026-09-30.

## Environment

- Host: `github.com`
- Client path: `gh api graphql`
- Authenticated user: configured through `gh auth`; identity is intentionally not recorded
- Observed token scopes: `project`, `read:org`, `repo`
- No mutation was submitted to a work project; disposable personal-project mutations were submitted and reconciled

## Verified Schema Capabilities

The live GraphQL schema reports the following fields:

- `ProjectV2.items(after, before, first, last, orderBy, archivedStates, query)`
- `ProjectV2.views(after, before, first, last, orderBy)`
- `ProjectV2View.layout`, `filter`, `fields`, `groupByFields`, `verticalGroupByFields`, and `sortByFields`
- `ProjectV2.layout` values: `BOARD_LAYOUT`, `TABLE_LAYOUT`, `ROADMAP_LAYOUT`
- `ProjectV2FieldConfiguration` union members: `ProjectV2Field`, `ProjectV2IterationField`, `ProjectV2MultiSelectField`, and `ProjectV2SingleSelectField`
- `ProjectV2ItemFieldValue` includes text, number, date, iteration, single-select, multi-select, issue, pull request, and other field-value unions
- `ProjectV2Item.fieldValueByName(name:)` exposes a specific field value; nested labels, users, reviewers, and linked pull requests each expose cursor-paginated connections
- Mutations: `updateProjectV2ItemFieldValue`, `clearProjectV2ItemFieldValue`, and `updateProjectV2ItemPosition`
- `ProjectV2.viewerCanUpdate`

The mutation input and payload types report these fields:

- `UpdateProjectV2ItemFieldValueInput`: optional `clientMutationId`; required `projectId: ID!`, `itemId: ID!`, `fieldId: ID!`, and `value: ProjectV2FieldValue!`
- `ClearProjectV2ItemFieldValueInput`: optional `clientMutationId`; required `projectId: ID!`, `itemId: ID!`, and `fieldId: ID!`
- `UpdateProjectV2ItemPositionInput`: optional `clientMutationId`; required `projectId: ID!` and `itemId: ID!`; optional `afterId: ID`
- `ProjectV2FieldValue`: optional `text: String`, `number: Float`, `date: Date`, `singleSelectOptionId: String`, `multiSelectOptionIds: [String!]`, and `iterationId: String`
- The field-value mutation payloads return `clientMutationId` and `projectV2Item`; the position mutation payload returns `clientMutationId` and `items: ProjectV2ItemConnection`

These schema observations were followed by write probes against a disposable personal project; work projects were not modified.

The `items.query` argument is present in the target schema. It is the only filter evaluator used by the app; the client accepts only the bounded grammar recorded below. Other GitHub filter grammar remains unsupported.

## Read Probe Results

Results below are intentionally aggregate and contain no project names, item titles, IDs, or bodies:

- User-owned project discovery returned one project; its only sampled view was `TABLE_LAYOUT`.
- Organization membership discovery returned one organization and did not require another page.
- The accessible organization project list returned one open project and did not require another page.
- That project exposed five views: two boards, two tables, and one roadmap.
- The unfiltered position-ordered item probe returned 49 items, all issues or pull requests, on one page.
- The sampled saved filter `iteration:@current` was accepted by `items(query:)` and returned zero items.
- Additional read-only filter probes were accepted by `items(query:)`: `status:"Todo"` returned one item, `no:status` returned three, `-status:"Todo"` returned 48, and `assignee:@me` returned zero. These counts validate the query shapes on the target project but do not establish semantics for every field value or compound expression.
- On the disposable user-project sandbox, read-only probes returned two items for `status:"Done"`, one for `assignee:@me`, and one for `status:"Done" assignee:@me`; the compound result matched the intersection of its terms. `-status:"Todo"` returned two and `-status:"Todo" assignee:@me` returned one, also matching the intersection. `iteration:@current` returned zero, and `iteration:@current status:"Todo"` returned zero. These samples exercise positive, negative, empty-value, relative, and conjunction query shapes; the earlier organization probes exercise a non-empty `no:status` result.
- The initial app accepted only the verified two-term conjunctions `status:"value" assignee:@me`, `-status:"value" assignee:@me`, and `iteration:@current status:"value"`. The 2026-09-26 probes below expand this gate for independently verified term families and conjunction shapes.
- The board metadata probe returned visible-field metadata for all five views. Grouping and sorting metadata were present on the board views, including a board with vertical grouping and position sorting.
- Representative board probes preserve saved metadata order: the vertical `Status` options were returned as `Todo`, `In Progress`, `Done`, `Staged`; the two-axis board exposed `Priority` columns (`P0`, `P1`, `P2`) and `Status` vertical groups; the single-axis board reported an ascending `Priority` sort.
- The first board's field configuration decoded as title, assignees, three single-select fields, a number field, and an iteration field. Single-select options and iteration configuration are returned as inline lists, not cursor connections.
- `ProjectV2View` introspection exposes `groupByFields`, `verticalGroupByFields`, `sortByFields`, `filter`, `layout`, and `configuration.visibleFields`. The sampled project's `fields` and `configuration.visibleFields` connections contained the same nodes in the same order; the client now reads the explicit `configuration.visibleFields` connection and paginates it. `ProjectV2ViewConfiguration` exposes only `visibleFields`; collapsed-group state is not exposed. The client preserves returned visible-field, grouping-field, option, active-iteration, completed-iteration, and sort metadata order. This confirms the available metadata path, not parity for a user's custom display settings.
- Item reads use owner-specific GraphQL branches, position ordering, and the saved filter unchanged. Scalar project and issue-backed field values plus issue/pull-request repository, state, and aggregate sub-issue progress metadata are loaded during board reads. Board reads request up to 10 assignees per value with an explicit truncation marker to stay within GitHub's GraphQL node limit. Detail reads additionally request 100 labels, users, reviewers, and linked pull requests per nested connection page, followed by further pages when present, plus repositories and milestones; unavailable nested objects remain explicitly marked unavailable.
- Saved-filtered boards page the saved `items(query:)` expression unchanged. The active loader uses one paginated stream for both filtered and unfiltered views; it does not append lane qualifiers to saved filters.
- A read-only probe on 2026-09-28 against an organization board with 67 issue items accepted `type:Epic`, `type:"Epic"`, and `-type:Epic`; the positive forms each returned 8 items and the negative form returned 59. The results sum to the unfiltered 67-item set.
- The disposable sandbox has two probe views, a numeric `TUI Probe Points` field, and an iteration field containing a current and completed iteration. Saved view #3 is grouped vertically by Status and sorted descending by Points. A GitHub web screenshot and the TUI show the Done-lane values 5, 2, 2 in the same order; the tied 2-point items also retain the same relative order. This verifies the sampled numeric DESC ordering/tie case and vertical-Status-to-board-lane projection. A value of 1 was subsequently assigned to a later Todo item to create a mixed populated/unset lane; that updated lane still needs a web/TUI comparison to verify null placement.
- Saved view #4 is currently grouped by Status, but has an ascending sort on `TUI Probe Iteration`. GitHub web shows the completed iteration item before the current iteration item, followed by the unset item (#3, #2, #4). The previous TUI ordering was #2, #3, #4; after changing iteration comparisons to use start dates, the user confirmed the TUI order now matches GitHub. The GraphQL view update input exposes only `visibleFieldIds` and `filter`, not group or sort configuration.
- The personal sandbox's saved board has no filter or explicit sort and exposes a single-select Status field through `verticalGroupByFields`, with options in Todo, In Progress, Done order. The TUI renders those as lanes; the GitHub web board screenshot supplied during review shows Status columns too. The current API read returns 13 items (9 Todo, 1 In Progress, 3 Done, none unset). These counts are not a simultaneous web/TUI count comparison, and this view cannot establish sort or iteration parity.

These results establish that the intended discovery, view metadata, position ordering, and server-side filter read shapes work on the target host. They do not establish that every GitHub filter expression or saved-view display rule has matching semantics.

## Saved Filter Grammar and Evidence

GitHub's [Projects filtering documentation](https://docs.github.com/en/issues/planning-and-tracking-with-projects/customizing-views-in-your-project/filtering-projects) defines AND between whitespace-separated qualifiers, OR between comma-separated values **within one qualifier**, and AND for repeated qualifiers. Cross-field `OR` is not supported by GitHub. The app validates syntax before opening a saved view; accepted strings are forwarded unchanged as the `items(query:)` variable on every page. It does not implement the filter locally.

Read-only probes on 2026-09-26 used the disposable user-owned sandbox (#2) and public `github` organization projects on `github.com`; additional parent, closure-date, reviewer, reason, wildcard, and quoting probes were added on 2026-09-29. The sandbox had 17 issue items on one page at probe time. No item titles, IDs, or body data were retained in this document. The counts below are snapshots, not promised counts for future runs. Contrast queries with the unfiltered baseline and the documented item composition before treating a zero-result shape as semantically verified.

The [2026-09-30 audit](filter-audit-2026-09-30.md) rechecked a complete 26-item sandbox baseline with 29 representative expression cases and forced two-item pages. All membership comparisons passed. For `updated:`, all four sampled date-group result sets matched `ProjectV2Item.updatedAt`; two differed from `Issue.updatedAt`, with no baseline changes during the comparison. This supports the project-item timestamp as the source on this snapshot. Earlier coincident issue-timestamp counts do not establish that source, and this is not a universal host/item-kind claim. Production continues to forward filters unchanged to GitHub.

| Form | Accepted syntax and example | Probe evidence / boundary |
| --- | --- | --- |
| Status | `status:Todo`, `status:"In Progress"`, `status:"Todo","Done"`, leading `-` | `status:Todo` → 1, `status:"Done"` → 16, both values → 17, `-status:"Todo"` → 16, negated both → 0. No space after a comma: `status:"Todo", "Done"` returned 0. |
| Assignee | `assignee:@me`, `assignee:USERNAME`, comma-separated values, leading `-` | `@me` and the sandbox username each → 1; two comma-separated equivalents → 1; `-assignee:@me` → 16. Repeated equivalent assignee terms → 1; distinct-assignee AND remains documented, but no sandbox item had two assignees to establish a nonempty intersection. |
| Reviewers | `reviewers:USERNAME`, comma-separated usernames, repeated terms, leading `-` | Public project #20381 had 118 items over two baseline pages. Two sampled users matched 65/10 items; their comma-separated OR → 71, repeated-qualifier AND → 4, negated first → 53, negated OR → 47. Every ID matched the complete `ProjectV2ItemFieldReviewerValue.reviewers` baseline, including review requests; the first user's submitted-review history alone matched only 32. `@me` and team references remain gated for reviewers. |
| Label | `label:bug`, `label:"name with spaces"`, `label:bu*`, `label:*ug`, `label:*ug*`, comma-separated values, leading `-` | `label:bug` and `label:"bug"` → 2, `label:bug,support` → 2, `-label:bug` → 15; repeated `label:bug` → 2. Each wildcard shape and wildcard OR with support → 2, negated wildcard → 15, with membership checked against complete labels. No matching space-containing label was available. Internal or repeated wildcard characters remain gated. |
| Location | `repo:OWNER/REPO`, leading `-` | Sandbox repository → 17; its negation → 0. Multiple repositories in a single qualifier are still gated. |
| Item state/kind | One of `is:open`, `is:closed`, `is:issue`, `is:pr`, `is:draft`, `is:merged`; leading `-` | Sandbox issue/open/closed counts: 17/1/16, `-is:closed` → 1. Public projects #12106/#20381 yielded matching PR/merged/draft results; #20381 returned one draft PR. |
| Close reason | `reason:completed`, `reason:"completed"`, `reason:"not planned"`, comma-separated values, leading `-` | On public project #20381, completed → 40 (13 completed issues and 27 closed, unmerged PRs), not planned → 2 issues; their OR → 42, negated completed → 78, negated not planned → 116 (100/16 pages), negated OR → 76. Adding `is:issue` to completed → 13. The 51 merged PRs were excluded from completed in this snapshot. `reason:reopened` and other values remain gated without suitable populated samples. |
| Issue type | `type:Epic`, `type:"Epic"`, and leading `-` | On an organization board, `type:Epic` and `type:"Epic"` each returned 8 of 67 issues; `-type:Epic` returned the remaining 59. Only one issue type per term is currently accepted. |
| Parent issue | One `parent-issue:OWNER/REPO#NUMBER` reference, optionally double-quoted; leading `-` and whitespace conjunctions | Quoted and unquoted references each returned the same two linked children in the sandbox; their negations returned the other 15 items, matching the complete parent metadata baseline. Adding `is:closed` returned the same two, and `is:open` returned none because both children were closed. Comma-separated references remain gated. |
| Presence | `has:status`, `no:status`, `has:assignee`, `no:assignee`, `has:label`, `no:label`, `has:FIELD`, `no:FIELD`, or `-no:` equivalents for verified single-select/number/iteration fields | Status has/no → 17/0, assignee has/no → 1/16, label has/no → 16/1; `-no:assignee` → 1 and `-no:label` → 16. Sandbox Points has/no → 4/13, Iteration has/no → 3/14; public project #12106 Phase has → 60 and `-no:phase` → 60. Other field types are gated. |
| Additional presence | `has:`/`no:`/`-no:` with `reviewers`, `parent-issue`, or `closed` | Sandbox parent has/no → 2/15 and closed has/no → 16/1; `-no:` matched their has-results. Public project #20381 reviewer has/no → 88/30, `-no:reviewers` → 88, checked against the complete reviewer-field baseline. |
| Single-select field | Hyphenated project field name, e.g. `phase:"Phase I","Phase II"`, `-phase:"Phase I"` | Public project #12106 Phase I → 37, Phase II → 14, their comma-separated OR → 51; `has:phase` → 60. Only fields reported as SINGLE_SELECT in the project field metadata qualify. |
| Number field | Hyphenated project field name, e.g. `tui-probe-points:>=2`, `tui-probe-points:1..2`, `tui-probe-points:*..2`, `tui-probe-points:2,5`, `-tui-probe-points:2` | Sandbox Points values: 1 item at 1, 2 at 2, 1 at 5, 13 unset. `>=2` → 3, `>2` → 1, `1..2` → 3, `*..2` → 3, `2,5` → 3, negated 2 → 15. Only fields reported as NUMBER in the paginated project field metadata qualify. |
| Iteration field | Hyphenated field name with a quoted title, `@previous`, `@current`, `<@current`, `>=@previous`, or a range between previous/current | The sandbox's `tui-probe-iteration:@current` → 2, `@previous` → 1, `<@current` → 1, `@previous..@current` → 3, quoted `"Probe current"` → 2. A standalone `iteration:@current` remains accepted from the original probes, but returned 0 on a project without an Iteration field of that name. Next/offset forms remain gated. |
| Created/updated dates | `created:2026-09-23`, `updated:>=2026-09-24`, `updated:@today-1d`, `updated:@today-3d..@today-1d`, `created:*..2026-09-24`; optional leading `-` | Issue timestamps in the sandbox were created Sep 23/24/25 (13/3/1) and updated Sep 23/24/25 (6/1/10). Filter counts matched: created Sep 23 → 13, created >= Sep 24 → 4, created Sep 23..24 → 16; updated Sep 25 → 10, updated >= Sep 24 → 11, updated Sep 23..24 → 7, `@today-1d` → 10, `@today-3d..@today-1d` → 17 (as of Sep 26). GitHub evaluates relative dates at request time. |
| Closed dates | `closed:2026-09-23`, comparisons, inclusive ranges, `*` bounds, relative `@today` offsets; optional leading `-` | On Sep 29, sandbox closure dates were Sep 23/24/25 (5/1/10), with one unset. Exact Sep 23 → 5; `>=` Sep 24 → 11; `>` Sep 24 → 10; `<` Sep 25 and `<=` Sep 24 → 6; Sep 23..24 and `*..` Sep 24 → 6; Sep 24..`*` → 11. Negated Sep 23 → 12, including the unset item. `@today-4d`, `@today-4`, `>=@today-4d`, and an equal-bound relative range each → 10; adding `is:closed` to Sep 25 → 10. These are closure-date filters, not a replacement for `is:closed`. PR/draft closure-date semantics remain unprobed. |
| Title and general text | `title:"Exact title"`, `title:Expand*`, `title:*filter*`, unqualified word searches such as `filter grammar`; leading `-` on `title:` only | An existing sandbox issue's exact title → 1, a shorter quoted title → 0, negated exact title → 16. `title:Expand*` → 1, `title:*filter*` → 3; bare `filter` → 3, `ilter` → 0, `grammar` → 1, `filter grammar` → 1. General search matches beginnings of words rather than arbitrary substrings as documented by GitHub. |
| Quoted punctuation | Commas inside an exact double-quoted `title:` value; optional leading `-` | A public-project title containing a comma matched its one known item; its negation matched the remaining 117 across two pages (100/17). A known title containing embedded quotation marks matched zero when backslash-escaped, despite a baseline member, so escaped quotes remain gated. This does not verify commas inside label or single-select names. |
| AND combinations | Whitespace-separated supported terms, including repeated qualifiers; no explicit `AND` | `is:issue is:open` → 1, `label:bug is:closed` → 2, `label:bug is:issue is:closed` → 2, `has:status label:bug` → 2, `no:label is:closed` → 1, `status:"Todo","Done" assignee:@me` → 1. Previous Status/assignee probes had nonempty intersections. No arbitrary OR expressions are accepted. |

The structural grammar below is narrowed by the qualifier-specific rules in the evidence table. Values are kept verbatim, not unquoted or normalized before submission:

```text
filter        := whitespace? (term (whitespace term)*)? whitespace?
term          := text-word | "-"? qualifier ":" values
values        := value ("," value)*
value         := unquoted-value | '"' quoted-value '"'
qualifier     := verified lower-case built-in or unique hyphenated project-field name
```

- A text-word is unquoted and unnegated. Standalone `AND`/`OR` words are never general search terms. Whitespace expresses AND; no explicit Boolean operators, unquoted grouping parentheses, or cross-field OR are accepted. Literal parentheses in double-quoted values remain accepted. Duplicate/repeated terms are allowed, with GitHub determining membership.
- Multiple values are enabled only for single-select, assignee, reviewers, label, number, and verified reason values. There is no whitespace after a comma. Single-select and label values are nonempty words or double-quoted names; quotes group spaces. Commas inside quotes are enabled only for exact titles, not label/single-select names. Backslash escapes and single quotes remain gated.
- Assignees are `@me` or unquoted usernames; reviewers are unquoted usernames only. Repo is one unquoted `OWNER/REPO`; `is:` is one verified keyword; `type:` is one word or double-quoted issue type. Parent is one `OWNER/REPO#NUMBER` reference with a positive issue number, optionally double-quoted. Close reason is `completed` (optionally quoted) or quoted `"not planned"`.
- Title accepts an exact word, a double-quoted exact title, or leading/trailing `*` around one word. Label accepts the same edge-wildcard word shapes. Internal/repeated stars and other wildcard forms remain gated.
- Custom single-select/number/iteration qualifiers must match exactly one hyphenated ASCII project-field name in the **complete**, cursor-paginated project-field metadata, even when hidden in the saved view. Custom DATE/TEXT/MULTI_SELECT fields and ambiguous name collisions remain gated.
- Number values are decimal literals, `>`, `>=`, `<`, `<=` comparisons, or inclusive `..` ranges with an optional `*` bound. Iteration accepts a quoted title, `@previous`, `@current`, comparisons or ranges between those keywords. Only `@current` is verified on the standalone `iteration:` qualifier.
- Created/updated/closed dates accept valid `YYYY-MM-DD` dates or `@today` with optional numeric `+`/`-` offsets (days or weeks), comparisons/ranges, and `*` bounds. Both-wildcard ranges, empty values/components, malformed dates, and partial quotes are rejected.
- `has:`/`no:` accept Status, assignee, label, reviewers, parent-issue, closed, or verified single-select/number/iteration project fields. `-no:` selects present values. `-has:` is gated (use `no:`); negative built-in/custom iteration filters remain gated. Other supported qualifiers accept leading `-`.

| Documented but gated (needs matching probes) | Examples / evidence needed |
| --- | --- |
| Other custom field types/names | Custom DATE, TEXT, MULTI_SELECT, and field names with punctuation (for example `Estimate (days)`); their exact qualifier spelling and matching value syntax remain unverified on suitable samples. |
| Remaining relative forms | `iteration:@next`, `iteration:@current+3`, and custom iteration offsets/next ranges; the sandbox has no next-iteration member. Relative created/updated queries are verified against Sep 26 snapshot timestamps; other hosts/time-zone boundaries remain unprobed. |
| Specialized filters and additional text shapes | Custom text-field qualifiers, quoted general phrases, wildcard shapes other than verified title/label edges, milestone, reviewer `@me`/team references, and `reason:reopened`. Existing sampled projects had no populated custom DATE/TEXT values or milestones suitable for matching probes; no project data was created to manufacture samples. Other reason keywords remain gated even if exposed by issue metadata. |
| Additional Boolean and value shapes | Commas for other qualifiers (including parent references), mixed multi-assignee membership, commas inside label/single-select values, escaped quotes, single-quoted values, and cross-field `OR` (GitHub documents the last as unsupported). A sample `status:"Todo" OR label:bug` returned 3, but this alone cannot establish what the server interpreted; it stays blocked. |
| Pagination | On public `github` project #12106, the `is:pr` saved-query shape returned 338 matching PR items over four cursor pages (100/100/100/38), with 338 unique IDs and `hasNextPage=false` on the last page. This verifies a representative multi-page server-filtered result; the adapter fixture also verifies the filter variable remains byte-for-byte identical on each page. |
| Closed-date pagination | The sandbox's `closed:>=2026-09-24` returned 11 unique items across six deliberately small pages (2/2/2/2/2/1), with every returned closure date matching the comparison and `hasNextPage=false` on the last page. The unchanged-filter adapter fixture also covers closure-date comparisons, negation, and relative ranges, plus quoted parent-issue references. |

These boundaries are deliberate: API acceptance with zero results is not evidence that GitHub applied the intended semantics. Revisit gated entries with read-only probes on suitable existing projects rather than silently opening them as unfiltered views.

## Current Compatibility Matrix

“Fixture” means the behavior is exercised by local adapter/model tests, not that its display has been independently compared with a representative GitHub web view. The table distinguishes what the app currently opens from what is empirically proven.

| Capability | Current client behavior | Empirical status / remaining limit |
| --- | --- | --- |
| Layout | Saved boards and tables open; roadmaps are blocked. | Board and table reads were probed. Exact table/roadmap rendering is outside the board MVP. If no saved board is compatible, unsupported views remain blocked; the app does not create an unfiltered substitute. |
| Saved filters | The bounded grammar in [Saved Filter Grammar and Evidence](#saved-filter-grammar-and-evidence) runs through `items(query:)` without local emulation. Unsupported expressions are blocked with a reason. | Common metadata, parent references, reviewer usernames, two close reasons, edge title/label wildcards, single-select/number/iteration fields, created/updated/closed date forms, and >100-result live pagination were probed. Custom date/text/multi-select fields, additional iteration offsets, and the specifically listed specialized filters remain gated. |
| Board axes | One single-select or iteration field per axis; `groupByFields` maps to columns and `verticalGroupByFields` maps to swimlanes when both exist. A sole vertical field is projected into lanes. Unset values get a lane; single-select options and active/completed iterations preserve metadata order. Multi-valued and multiple grouping fields are blocked. | Sole vertical Status → lanes/columns matches the supplied GitHub web and TUI screenshots. Iteration grouping has fixture coverage, but probe view #4 uses Status as its grouping field; direct web comparison of iteration lanes remains outstanding. |
| Table grouping | One saved single-select or iteration `groupByFields` field renders ordered row sections with loaded-item counts. Only populated sections are shown, including an unset section when populated. Saved sorts apply within sections; local search recalculates counts. Table vertical/multiple grouping is blocked with a picker explanation. | Fixture-backed projection of API field/option order; collapsed-group state and any custom display order are not exposed by the API. Counts are for loaded items while pagination is in progress. |
| Table cells | Saved visible fields remain the column boundary. Issue/PR/draft icons and issue numbers identify titles; available single-select values are accented, and issue sub-issue progress uses the lightweight content summary. Unset and explicitly unavailable values differ. The title receives the remaining width after other columns on narrow terminals. | No full-detail read is triggered for table cells. Linked pull requests and other nested fields are not enriched beyond the selective item response. |
| Table actions | On writable, fully loaded views, a confirmed single row can be archived or removed from its project. Group changes use the saved single-select/iteration field; row reorder uses project-wide position only with position sorting. Writes are resolved against project readback and blocked during local search. | GraphQL schema exposes `archiveProjectV2Item` and `deleteProjectV2Item` with project/item IDs; archive returns `isArchived`, delete returns `deletedItemId`. Other GitHub UI actions (bulk cell editing, item creation, archive restoration, and arbitrary field editing) are not implemented. |
| Roadmap | All saved roadmaps remain unsupported and read-only, even when a project contains date fields or they are visible. | See [Roadmap layout investigation](#roadmap-layout-investigation): the API returns layout and generic field/item data, but not the roadmap's start/target-field mapping or timeline display state. |
| Sorting | Project-position order is the read baseline. Title, text, number, date, single-select, and iteration ASC/DESC field sorts currently use local comparisons, with unset values last and project-position order for ties. Other field types/directions are blocked; explicit field sorts disable manual reorder. | Numeric DESC ordering and a tied-value relative order matched GitHub web in saved sandbox view #3. Iteration ASC completed/current/unset ordering matches the user's GitHub/TUI comparison after switching the TUI comparator to iteration start dates. Null placement in a mixed populated/unset numeric lane, numeric ASC, and text/date/single-select parity remain unverified. |
| Visible fields and detail | Cards use `configuration.visibleFields` in API order and omit values used as grouping axes. Board assignee summaries fetch up to 10 names with a truncation indicator; detail loads all project definitions, unset fields, accessible body, and paginated nested values on demand. | The saved visible-fields connection was probed. Board-only nested field values that are not requested can be omitted from card summaries; inaccessible values are distinguished in detail. The >100 nested-value case has fixture coverage rather than a live project with that many values. |
| Display settings | Known API-visible metadata is rendered; the TUI states that collapsed-group state is unavailable. | `ProjectV2ViewConfiguration` exposes `visibleFields` but not collapsed-group or custom group-order settings; exact web display parity is not possible for those settings. |
| Writes | `viewerCanUpdate` plus a complete single-axis or supported combined single-select/iteration board enables column/lane moves; position-sorted views also allow reorder on single-axis boards. Sorted-by-field moves are field-only. Supported saved-filtered views resolve writes against unfiltered project state. Combined-board reorder and swimlane changes are read-only. | Set/clear and top/after-anchor behavior were confirmed on disposable sandbox items. Queue serialization, canonical intent resolution, partial failures, ambiguous outcomes, readback failure, view switching, and combined-column moves that preserve the row have fixture coverage. Concurrent external edits during submission and unsupported-axis mutations remain outside verified semantics. |

## Roadmap Layout Investigation

Read-only GitHub.com probes used an existing accessible organization project with one saved roadmap. No project data was changed. The observations are sanitized: project and field names, item IDs/titles, field values, and exact dates are omitted.

Schema-only probes were repeated on 2026-09-29 and 2026-09-30 and confirmed the configuration still exposes only `visibleFields`. The prior organization-project observations below were not repeated: the token's project lookup on 2026-09-29 was blocked by organization SAML enforcement. Reauthorize the token for that organization before rerunning project-level probes; this failure is not evidence of changed roadmap semantics. The 2026-09-30 check used existing authentication and retained only schema field names; it did not repeat project-level placement probes.

An accessible public organization roadmap was subsequently probed on 2026-09-29 using `TestLiveRoadmapReadContract`. It returned 18 project field definitions (two DATE, no ITERATION), empty configured visible fields/grouping/vertical grouping/sorting, and an empty filter. Its unfiltered `POSITION ASC` baseline had 11 unique items on one page, with zero populated date/iteration values. The live test passed, independently confirming the current read path and metadata boundary without SAML access. Neither sampled roadmap establishes nonempty placement behavior. No project data was changed or copied into fixtures.

### Verified metadata and item data

- The saved view reports `layout: ROADMAP_LAYOUT`. Its GraphQL `ProjectV2View` fields are `configuration`, `fields`, `filter`, `groupByFields`, `layout`, `name`, `number`, `sortByFields`, and `verticalGroupByFields` (plus identity/timestamps). The legacy `fields` connection contained three generic display fields (Title, Assignees, and one single-select); `configuration.visibleFields` was empty. Grouping, vertical grouping, sorting, and filter were also empty.
- Schema introspection reports `ProjectV2ViewConfiguration` exposes only `visibleFields`. It has no start-date/target-date field IDs, timeline range, zoom, marker, or slice-by settings. The `ProjectV2View` type also has no roadmap-specific connection or field.
- The project exposes 19 field definitions: 15 `ProjectV2Field`, one iteration field, and three single-select fields. Two generic fields have `dataType: DATE`. These are project definitions only; the view API does not say which, if either, is the roadmap start or target field.
- The project item query returned 49 items in one `POSITION ASC` page. It supports `orderBy: POSITION`, which provides project-wide ordering, not a date-axis coordinate. The sampled items had zero date-value nodes and zero iteration-value nodes, so no nonempty roadmap placement or null-date behavior could be compared.
- Generic item metadata does expose date field values (`ProjectV2ItemFieldDateValue.date`) and iteration values (`iterationId`, `title`). The project iteration definition can expose `startDate` and `duration`. These data shapes do not resolve which values GitHub assigned to roadmap endpoints or how the web view maps them to a date range.
- GitHub's roadmap documentation describes user-selected date/iteration start and target fields, Month/Quarter/Year zoom, markers, and field slicing. These settings have no corresponding fields in the introspected Projects GraphQL view schema, so their saved values and exact state cannot currently be recovered through this client API.

The public [roadmap documentation](https://docs.github.com/en/issues/planning-and-tracking-with-projects/customizing-views-in-your-project/customizing-the-roadmap-layout) was checked on 2026-09-29. It also describes grouping, primary/secondary sorts, and field sums. Generic grouping/sort connections exist, but populated roadmap grouping/sort parity remains unverified; field-sum settings are not exposed by the introspected view configuration.

### Fixture-backed compatibility and proposed bounded timeline

`TestViewCompatibilityRejectsUnsupportedSemantics` includes a roadmap carrying plausible visible DATE fields and confirms the layout is still rejected. `TestIncompatibleViewStaysInPicker` confirms opening a roadmap does not enter the board or start item loading. `TestRoadmapDateAndIterationMetadataDoesNotEnableLoading` repeats that check with DATE/ITERATION metadata and write permission. This intentionally prevents generic date fields from being guessed as start/target endpoints.

`TestOpenRoadmapPreservesMetadataWithoutInferringEndpoints` covers both user and organization adapters with a synthetic fixture matching the observed response shape: two project DATE definitions, empty configured visible fields/grouping/sorting, and legacy display fields that are not used as an endpoint fallback. Fixture IDs and names are invented. These tests verify safe decoding/rejection, not web placement parity.

`TestRoadmapCandidateValuesPreserveDatesIterationsAndUnsetItems` exercises invented two-date, one-date, undated, and iteration values for both owner kinds, including nested field-value pagination and a second item page. Date strings and iteration identity survive decoding unchanged; missing dates are not synthesized. This does not prove how GitHub uses those values as timeline endpoints.

Proposed first read-only timeline, for review after field-mapping semantics are verified:

- Require explicit, verified start and target field identifiers from roadmap configuration. Until the API supplies them (or a separately verified supported configuration source does), keep the view blocked; never infer endpoints from visible DATE fields, names, or values.
- Use a bounded three-month viewport with month headers, horizontal month navigation, and one item row per issue/PR/draft. A start and target date create an inclusive bar; one endpoint creates a point marker, subject to web verification. Preserve verified saved row sorts with project-position ties, or project position when no explicit sort exists; do not impose an invented start-date sort.
- Keep items without usable endpoints in an explicit undated count/list rather than assigning invented dates. Report dates outside the visible range through clipped-bar markers and counts.
- Defer iteration-to-date placement, markers, slicing, custom zoom restoration, and drag-to-reschedule. Iteration `startDate`/`duration` presence alone does not prove roadmap-specific placement rules.

The pure DATE renderer in `internal/ui/date_timeline.go` is implemented with explicit synthetic endpoints (#27). Its three-month buckets, clipping, exact selected endpoint readout, ordered row window, and narrow date-list fallback have fixture coverage. Inclusive bars and one-endpoint points are provisional conventions, not verified GitHub semantics. It performs no API calls and is unreachable from saved view selection. Roadmaps remain explicitly unsupported and read-only until endpoint mapping and representative nonempty placements can be verified against GitHub.

The [reviewable timeline proposal](roadmap-proposal.md) expands the compatibility gates, date/null/error cases, ordering rules, narrow-terminal fallback, and verification checklist. The picker now explains the missing endpoint/display configuration and directs users to open the view in GitHub.

### Reproducible read-only probe

Select an existing saved roadmap accessible to the authenticated `gh` account. Organization ownership is the default; use `GH_PROJECTS_TUI_LIVE_OWNER_KIND=user` for a personal project.

```sh
GH_PROJECTS_TUI_LIVE_ROADMAP=1 \
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
GH_PROJECTS_TUI_LIVE_VIEW=ROADMAP_VIEW_NUMBER \
go test -tags live ./internal/github -run '^TestLiveRoadmapReadContract$' -count=1 -v
```

The probe introspects the view/configuration schema, reads the selected view through the adapter, and paginates an **unfiltered** `POSITION ASC` item baseline, including nested field-value pages. Logs contain only schema field names and aggregate metadata/item/date/iteration counts; project/item content stays in memory. It submits no mutations and fails if the view or configuration schema fields change so the API boundary can be revisited. GitHub permits at most two `__Type.fields` occurrences per introspection query.

An empty/populated date count alone does not verify start/target selection, inclusive/exclusive endpoints, one-endpoint behavior, undated placement, or iteration-to-date conversion. The proposed timeline rules above require separate nonempty web comparisons; a passing probe does not enable roadmap views.

### Saved endpoint mapping decision (#25)

**Decision as of 2026-09-29, reconfirmed on 2026-09-30: no-go for enabling saved roadmaps.** No supported endpoint-mapping source was found in the inspected GitHub.com GraphQL schema or the documented REST Projects APIs. The recheck repeated view/configuration introspection and reviewed the REST view documentation; project/field GET observations in the table remain the 2026-09-29 samples. This is a dated API availability finding, not a claim that an alternate source can never exist.

| Supported surface reviewed | Evidence | Endpoint-mapping result |
| --- | --- | --- |
| GraphQL `ProjectV2View` / `ProjectV2ViewConfiguration` | Live introspection of field names and return types; configuration contains only `visibleFields` | No saved start/target field IDs or alternate configuration object |
| GraphQL `ProjectV2` / `ProjectV2Field` | Live introspection of project and field names/types | Generic field/view connections and definitions, not selected timeline roles; project status updates are not view configuration |
| [REST Project views](https://docs.github.com/en/rest/projects/views) | Documentation reviewed for API version `2026-03-10`; documented view operations on that page are POST creation for user/org projects | No documented read operation for saved endpoint configuration, and creation parameters/response omit endpoint mapping. No POST was submitted |
| [REST Projects](https://docs.github.com/en/rest/projects/projects) | Read-only GET of the public sampled project; retained only response key names | Project metadata, not view endpoint IDs. `latest_status_update` start/target dates, when present, describe a status update, not roadmap field selection |
| [REST Project fields](https://docs.github.com/en/rest/projects/fields) | Read-only GET with `per_page=100`; 16 returned definitions including two DATE fields, no configuration keys in this sample | Definitions do not identify which fields a saved view selected. REST/GraphQL field counts differ (16/18); cross-API field-set parity is not established |
| REST saved-view items | Broader 2026-09-30 review of the [official OpenAPI description](https://github.com/github/rest-api-description/blob/67abf4515e6df41e32e1c182fd097b9599e1a32f/descriptions/api.github.com/api.github.com.json) found organization/user `GET .../projectsV2/{project_number}/views/{view_number}/items` operations | These read a saved view's filtered items, but their ordinary item/field-value response does not expose endpoint-role configuration. They do not resolve the mapping |

A broader 2026-09-30 live introspection reviewed all 160 `ProjectV2`-named types, including output and input definitions. `ProjectV2ViewConfiguration` still has only `visibleFields`; `ProjectV2ViewConfigurationInput` has only `visibleFieldIds`. Other start/target/date fields belong to project status updates or iteration data, not saved endpoint selection. The official REST OpenAPI revision above includes view creation and saved-view item reads, but no saved-view configuration GET or endpoint IDs in its view schema. This broader check used schema/official-description data only, without project reads or changes.

Explicit local mappings were approved on 2026-09-30 and are available through `--roadmap-mappings PATH`. The [configuration guide](local-roadmap-mappings.md) describes the host/project/view node-ID scope and user-configured source label. `ResolveRoadmapDateFields` validates their explicitly supplied start/target IDs against complete project definitions, rejecting absent/partial mappings, stale or duplicate IDs, and non-DATE types without a partial result. Names and display order never substitute for IDs. Picker and direct startup validation can read mapped view metadata, but the live placement gate still prevents item loading and writes. Local selections never replace cached API metadata or claim to recover GitHub's saved settings.

The REST GET probes used `Accept: application/vnd.github+json` and `X-GitHub-Api-Version: 2026-03-10` with existing `gh` authentication. No additional permissions were requested, no authentication was changed, and no work-project data or browser credentials were used. Private projects still require appropriate project access/SSO authorization; missing schema configuration cannot be repaired by granting more token scopes.

`TestRoadmapUnavailableEndpointMappingsStayBlocked` covers absent definitions, a stale visible DATE definition, ambiguous endpoint-like names, two plausible DATE fields, and unsupported TEXT definitions without a local mapping. All remain in the picker with no item-loading command even with write permission. Separate resolver and model tests cover the approved local source, including stale/duplicate/non-DATE rejection and scope isolation; a valid local mapping still leaves placement gated.

Revisit automatic recovery when a supported view read API exposes selected endpoint IDs. Require an existing saved-view comparison, unique field-ID resolution, and explicit missing/stale/ambiguous/type validation before relaxing that gate. #26/#28 remain blocked for live enablement; #27 implements only a pure renderer using synthetic inputs. The approved local source supplies user-selected endpoint roles for future verification, while retaining the distinction from recovered saved settings.

The investigation in #22 and the go/no-go decision in #25 are complete. They did not produce an automatic saved-endpoint source. The subsequent approved local configuration supplies explicit user-selected roles; #26 still requires populated date/filter/order comparisons, and #28 still requires verified placement and read-only lifecycle/navigation integration. #29 (grouping) and #30 (iteration endpoints) additionally require the initial verified timeline. These four feature issues stay open until their remaining prerequisites can be established.

The current compatibility gate accepts the field-sort and iteration projections in the table despite their outstanding web-parity checks. Do not interpret fixture coverage or schema availability as a confirmed match to GitHub's display rules.

## Mutation Reconciliation

- Mutation sessions are independent of cancellable view/item reads and remain attached to their originating owner/project while the user changes views. Results project their data into the visible board only when it is still showing the original project, view, and read generation. Overlapping refreshes finish before a fresh read loads the saved state.
- Before the first write, the client reads lean unfiltered project order/grouping state. It reads back submitted mutations with queued dependents, failed writes, and ambiguous outcomes; a definitive final success applies its plan to the session-owned baseline without a post-read. Field-only verification can read just the target item. Lane moves and reorders are queued as item/direction intents and resolved against the latest canonical state; rapid unsent moves of the same card collapse to the final placement. Queued commands do not retain mutation closures or position anchors.
- Field changes followed by position changes remain serial. A second-step failure is reconciled as a partial move, and the canonical project state is shown before the queue continues.
- GraphQL errors and HTTP 4xx responses are treated as definitive rejections; HTTP 5xx, transport/timeouts, malformed/missing mutation payloads, and unknown errors are ambiguous. The client reads back before proceeding. An ambiguous write is accepted when the intended state appears; repeated unchanged readbacks alone do not prove a timed-out write finished, so dependent writes stay blocked. Pressing `r` retries readback only and never blindly resubmits the write.
- If a readback fails, the session remains blocked until a later readback succeeds. Stale movement targets are discarded before submission when canonical state no longer permits the requested move.
- Board loads use one paginated stream with selective grouping/sort/card fields instead of per-lane fan-out. Shared transport pacing serializes requests, spaces writes, honors Retry-After/reset cooldowns, and surfaces remaining quota/cooldown plus a per-operation request ledger. Recent board/view/metadata reads are cached briefly with in-flight deduplication; mutation reconciliation stays authoritative.
- Discovery lists owners without preloading every org's projects; owner projects load on demand when an owner is selected. Saved views are cached per project for 5 minutes so revisiting a project costs no API; view-field pagination fetches only the advancing connection. Board refresh reloads items only and reuses the cached view.

## Sanitized Read Queries

This query shape is the minimum discovery/view probe. Values are placeholders and must be supplied only at runtime:

```graphql
query ProjectViews($owner: String!, $number: Int!) {
  organization(login: $owner) {
    projectV2(number: $number) {
      id
      number
      title
      viewerCanUpdate
      views(first: 100) {
        nodes {
          number
          name
          layout
          filter
          fields(first: 100) { nodes { __typename } }
          groupByFields(first: 20) { nodes { __typename } }
          verticalGroupByFields(first: 20) { nodes { __typename } }
          sortByFields(first: 20) { nodes { __typename } }
        }
      }
    }
  }
}
```

The owner lookup must branch between `user(login:)` and `organization(login:)`; the example uses an organization only to keep the query short.

The item page uses the same owner branching and this connection shape:

```graphql
query ProjectItems($login: String!, $project: Int!, $filter: String, $after: String) {
  organization(login: $login) {
    projectV2(number: $project) {
      items(first: 100, after: $after, query: $filter,
            orderBy: {field: POSITION, direction: ASC}) {
        nodes { id type content { __typename ... on Issue { repository { name } state subIssuesSummary { total completed } } ... on PullRequest { repository { name } state isDraft merged } } fieldValues(first: 100) { ... } }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}
```

REST membership paths are passed to `go-gh` without a leading slash, for example
`user/orgs?per_page=100&page=1`, so configured `/api/v3/` prefixes and host routing remain intact.

## Sanitized Mutation Shapes

The following shapes match the introspected mutation inputs and payloads. Field-value and position mutations were submitted only against the disposable project:

```graphql
mutation UpdateProjectItemFieldValue($input: UpdateProjectV2ItemFieldValueInput!) {
  updateProjectV2ItemFieldValue(input: $input) {
    clientMutationId
    projectV2Item { id }
  }
}

mutation ClearProjectItemFieldValue($input: ClearProjectV2ItemFieldValueInput!) {
  clearProjectV2ItemFieldValue(input: $input) {
    clientMutationId
    projectV2Item { id }
  }
}

mutation UpdateProjectItemPosition($input: UpdateProjectV2ItemPositionInput!) {
  updateProjectV2ItemPosition(input: $input) {
    clientMutationId
    items(first: 100) { nodes { id } }
  }
}
```

## Disposable Sandbox Write Results

The following results are intentionally aggregate and contain no project names, item titles, IDs, or bodies:

- A private personal project with the default single-select `Status` field was used with three temporary draft items.
- Setting one item's `Status` to `In Progress` persisted and was returned by a subsequent read.
- Clearing that `Status` removed the value and the subsequent item read omitted the field value.
- Moving an item with `afterId` placed it after the anchor in project position order.
- Omitting `afterId` moved an item to the project top. The temporary items were restored to their original order afterward.
- The position mutation payload requires a pagination boundary when its `items` connection is selected. The client therefore requests only `clientMutationId` and refetches canonical item order.

## Remaining Empirical Checks

The following items remain unverified:

- Iteration state and unset values beyond the sampled single-select boards
- Filter semantics outside the bounded grammar above, and the explicitly noted zero-result forms within it
- Sort null placement and tie behavior against representative GitHub-rendered views (the current client places unset values last and uses stable project-position order for ties)
- Grouping-axis mapping and display settings beyond the sampled board views; collapsed-group state is not exposed by the introspected view schema
- Mutation failure, partial-completion, timeout reconciliation, and stale-anchor handling

## Gate To Continue

Run the remaining probes against representative board views. Sandbox write semantics are verified; single-axis unfiltered boards expose guarded lane moves, and position-sorted views additionally expose manual reordering. Unsupported grouping/reordering paths remain read-only. Supported saved-filtered board mutations use canonical unfiltered project state and are covered by fixture tests; those tests do not establish additional live filter semantics.
