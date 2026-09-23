# API Contract Gate

Status: partial. This record covers the GitHub.com schema and read probes observed on 2026-09-17, mutation schema introspection observed on 2026-09-21, and saved-view schema introspection observed on 2026-09-22.

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

The `items.query` argument is present in the target schema. It is the only filter evaluator used by the app; the client accepts only the verified allowlist recorded below. Other GitHub filter grammar remains unsupported.

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
- The app currently accepts the verified two-term conjunctions `status:"value" assignee:@me`, `-status:"value" assignee:@me`, and `iteration:@current status:"value"`. Other combinations—including `no:status` combined with another term and conjunctions of more than two terms—remain unsupported until probed.
- The board metadata probe returned visible-field metadata for all five views. Grouping and sorting metadata were present on the board views, including a board with vertical grouping and position sorting.
- Representative board probes preserve saved metadata order: the vertical `Status` options were returned as `Todo`, `In Progress`, `Done`, `Staged`; the two-axis board exposed `Priority` columns (`P0`, `P1`, `P2`) and `Status` vertical groups; the single-axis board reported an ascending `Priority` sort.
- The first board's field configuration decoded as title, assignees, three single-select fields, a number field, and an iteration field. Single-select options and iteration configuration are returned as inline lists, not cursor connections.
- `ProjectV2View` introspection exposes `groupByFields`, `verticalGroupByFields`, `sortByFields`, `filter`, `layout`, and `configuration.visibleFields`. The sampled project's `fields` and `configuration.visibleFields` connections contained the same nodes in the same order; the client now reads the explicit `configuration.visibleFields` connection and paginates it. `ProjectV2ViewConfiguration` exposes only `visibleFields`; collapsed-group state is not exposed. The client preserves returned visible-field, grouping-field, option, active-iteration, completed-iteration, and sort metadata order. This confirms the available metadata path, not parity for a user's custom display settings.
- Item reads use owner-specific GraphQL branches, position ordering, and the saved filter unchanged. Scalar project and issue-backed field values plus issue/pull-request repository, state, and aggregate sub-issue progress metadata are loaded during board reads. Board reads request up to 10 assignees per value with an explicit truncation marker to stay within GitHub's GraphQL node limit. Detail reads additionally request 100 labels, users, reviewers, and linked pull requests per nested connection page, followed by further pages when present, plus repositories and milestones; unavailable nested objects remain explicitly marked unavailable.
- Saved-filtered boards page the saved `items(query:)` expression unchanged. Per-Status-lane parallel requests are reserved for unfiltered views, since appending lane qualifiers to a saved filter would create unverified filter expressions.
- The disposable sandbox has two probe views, a numeric `TUI Probe Points` field, and an iteration field containing a current and completed iteration. Saved view #3 is grouped vertically by Status and sorted descending by Points. A GitHub web screenshot and the TUI show the Done-lane values 5, 2, 2 in the same order; the tied 2-point items also retain the same relative order. This verifies the sampled numeric DESC ordering/tie case and vertical-Status-to-board-lane projection. A value of 1 was subsequently assigned to a later Todo item to create a mixed populated/unset lane; that updated lane still needs a web/TUI comparison to verify null placement.
- Saved view #4 is currently grouped by Status, but has an ascending sort on `TUI Probe Iteration`. GitHub web shows the completed iteration item before the current iteration item, followed by the unset item (#3, #2, #4). The previous TUI ordering was #2, #3, #4; after changing iteration comparisons to use start dates, the user confirmed the TUI order now matches GitHub. The GraphQL view update input exposes only `visibleFieldIds` and `filter`, not group or sort configuration.
- The personal sandbox's saved board has no filter or explicit sort and exposes a single-select Status field through `verticalGroupByFields`, with options in Todo, In Progress, Done order. The TUI renders those as lanes; the GitHub web board screenshot supplied during review shows Status columns too. The current API read returns 13 items (9 Todo, 1 In Progress, 3 Done, none unset). These counts are not a simultaneous web/TUI count comparison, and this view cannot establish sort or iteration parity.

These results establish that the intended discovery, view metadata, position ordering, and server-side filter read shapes work on the target host. They do not establish that every GitHub filter expression or saved-view display rule has matching semantics.

## Current Compatibility Matrix

“Fixture” means the behavior is exercised by local adapter/model tests, not that its display has been independently compared with a representative GitHub web view. The table distinguishes what the app currently opens from what is empirically proven.

| Capability | Current client behavior | Empirical status / remaining limit |
| --- | --- | --- |
| Layout | Saved boards open; tables have a read-only preview; roadmaps are blocked. | Board and table reads were probed. Exact table/roadmap rendering is outside the board MVP. If no saved board is compatible, unsupported views remain blocked; the app does not create an unfiltered substitute. |
| Saved filters | Empty, `iteration:@current`, quoted `status:"value"`, `no:status`, negated quoted Status, `assignee:@me`, and the documented two-term conjunctions run through `items(query:)` without local emulation. Unsupported expressions are blocked. | Positive/negative Status and assignee intersections were observed on the sandbox; `no:status` had nonempty results on a separate sampled project. The sampled current-iteration and its conjunction returned zero, so relative-filter membership parity is still unverified. Broader grammar is tracked separately. |
| Board axes | One single-select or iteration field per axis; `groupByFields` maps to columns and `verticalGroupByFields` maps to swimlanes when both exist. A sole vertical field is projected into lanes. Unset values get a lane; single-select options and active/completed iterations preserve metadata order. Multi-valued and multiple grouping fields are blocked. | Sole vertical Status → lanes/columns matches the supplied GitHub web and TUI screenshots. Iteration grouping has fixture coverage, but probe view #4 uses Status as its grouping field; direct web comparison of iteration lanes remains outstanding. |
| Sorting | Project-position order is the read baseline. Title, text, number, date, single-select, and iteration ASC/DESC field sorts currently use local comparisons, with unset values last and project-position order for ties. Other field types/directions are blocked; explicit field sorts disable manual reorder. | Numeric DESC ordering and a tied-value relative order matched GitHub web in saved sandbox view #3. Iteration ASC completed/current/unset ordering matches the user's GitHub/TUI comparison after switching the TUI comparator to iteration start dates. Null placement in a mixed populated/unset numeric lane, numeric ASC, and text/date/single-select parity remain unverified. |
| Visible fields and detail | Cards use `configuration.visibleFields` in API order and omit values used as grouping axes. Board assignee summaries fetch up to 10 names with a truncation indicator; detail loads all project definitions, unset fields, accessible body, and paginated nested values on demand. | The saved visible-fields connection was probed. Board-only nested field values that are not requested can be omitted from card summaries; inaccessible values are distinguished in detail. The >100 nested-value case has fixture coverage rather than a live project with that many values. |
| Display settings | Known API-visible metadata is rendered; the TUI states that collapsed-group state is unavailable. | `ProjectV2ViewConfiguration` exposes `visibleFields` but not collapsed-group or custom group-order settings; exact web display parity is not possible for those settings. |
| Writes | `viewerCanUpdate` plus a complete, unfiltered single-axis or supported combined single-select/iteration board enables column/lane moves; position-sorted views also allow reorder on single-axis boards. Sorted-by-field moves are field-only. Saved-filtered views, combined-board reorder, and swimlane changes are read-only. | Set/clear and top/after-anchor behavior were confirmed on disposable sandbox items. Queue serialization, canonical intent resolution, partial failures, ambiguous outcomes, readback failure, view switching, and combined-column moves that preserve the row have fixture coverage. Concurrent external edits during submission and filtered/unsupported-axis mutations remain outside supported writes. |

The current compatibility gate accepts the field-sort and iteration projections in the table despite their outstanding web-parity checks. Do not interpret fixture coverage or schema availability as a confirmed match to GitHub's display rules.

## Mutation Reconciliation

- Mutation sessions are independent of cancellable view/item reads and remain attached to their originating owner/project while the user changes views. Results update the visible board only when it is still showing that project.
- Before the first write, and after every submitted mutation, the client paginates lean unfiltered project order/grouping state. Lane moves and reorders are queued as item/direction intents and resolved against the latest canonical state; rapid unsent moves of the same card collapse to the final placement. Queued commands do not retain mutation closures or position anchors.
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
- Filter semantics outside the supported atoms and conjunction forms above
- Sort null placement and tie behavior against representative GitHub-rendered views (the current client places unset values last and uses stable project-position order for ties)
- Grouping-axis mapping and display settings beyond the sampled board views; collapsed-group state is not exposed by the introspected view schema
- Mutation failure, partial-completion, timeout reconciliation, and stale-anchor handling

## Gate To Continue

Run the remaining probes against representative board views. Sandbox write semantics are verified; single-axis unfiltered boards expose guarded lane moves, and position-sorted views additionally expose manual reordering. Unsupported or saved-filtered paths remain read-only until mutation serialization, reconciliation, and broader partial-failure handling are implemented and tested.
