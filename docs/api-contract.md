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

The `items.query` argument is present in the target schema. This supersedes the earlier assumption in `PLAN.md` that server-side filtering might be absent, but behavior still requires representative saved-view validation.

## Read Probe Results

Results below are intentionally aggregate and contain no project names, item titles, IDs, or bodies:

- User-owned project discovery returned one project; its only sampled view was `TABLE_LAYOUT`.
- Organization membership discovery returned one organization and did not require another page.
- The accessible organization project list returned one open project and did not require another page.
- That project exposed five views: two boards, two tables, and one roadmap.
- The unfiltered position-ordered item probe returned 49 items, all issues or pull requests, on one page.
- The sampled saved filter `iteration:@current` was accepted by `items(query:)` and returned zero items.
- Additional read-only filter probes were accepted by `items(query:)`: `status:"Todo"` returned one item, `no:status` returned three, `-status:"Todo"` returned 48, and `assignee:@me` returned zero. These counts validate the query shapes on the target project but do not establish semantics for every field value or compound expression.
- The board metadata probe returned visible-field metadata for all five views. Grouping and sorting metadata were present on the board views, including a board with vertical grouping and position sorting.
- Representative board probes preserve saved metadata order: the vertical `Status` options were returned as `Todo`, `In Progress`, `Done`, `Staged`; the two-axis board exposed `Priority` columns (`P0`, `P1`, `P2`) and `Status` vertical groups; the single-axis board reported an ascending `Priority` sort.
- The first board's field configuration decoded as title, assignees, three single-select fields, a number field, and an iteration field. Single-select options and iteration configuration are returned as inline lists, not cursor connections.
- `ProjectV2View` introspection exposes `groupByFields`, `verticalGroupByFields`, `sortByFields`, `filter`, `layout`, and `configuration.visibleFields`. The sampled project's `fields` and `configuration.visibleFields` connections contained the same nodes in the same order; the client now reads the explicit `configuration.visibleFields` connection and paginates it. `ProjectV2ViewConfiguration` exposes only `visibleFields`; collapsed-group state is not exposed. The client preserves returned visible-field, grouping-field, option, active-iteration, completed-iteration, and sort metadata order. This confirms the available metadata path, not parity for a user's custom display settings.
- Item reads use owner-specific GraphQL branches, position ordering, and the saved filter. Scalar project and issue-backed field values plus issue/pull-request repository, state, and aggregate sub-issue progress metadata are loaded during board reads. Board reads request up to 10 assignees per value with an explicit truncation marker to stay within GitHub's GraphQL node limit. Detail reads additionally request 100 labels, users, reviewers, and linked pull requests per nested connection page, followed by further pages when present, plus repositories and milestones; unavailable nested objects remain explicitly marked unavailable.

These results establish that the intended discovery, view metadata, position ordering, and server-side filter read shapes work on the target host. They do not establish that every GitHub filter expression or saved-view display rule has matching semantics.

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
- Sort null placement and tie behavior against representative GitHub-rendered views (the current client places unset values last and uses stable project-position order for ties)
- Grouping-axis mapping and display settings beyond the sampled board views; collapsed-group state is not exposed by the introspected view schema
- Live readback of nested multi-valued pagination beyond the first 100 values (fixture coverage is present)
- Mutation failure, partial-completion, timeout reconciliation, and stale-anchor handling

## Gate To Continue

Run the remaining probes against representative board views. Sandbox write semantics are verified; single-axis unfiltered boards expose guarded lane moves, and position-sorted views additionally expose manual reordering. Unsupported or saved-filtered paths remain read-only until mutation serialization, reconciliation, and broader partial-failure handling are implemented and tested.
