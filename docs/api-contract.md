# API Contract Gate

Status: partial. This record covers the GitHub.com schema and read probes observed on 2026-09-17.

## Environment

- Host: `github.com`
- Client path: `gh api graphql`
- Authenticated user: configured through `gh auth`; identity is intentionally not recorded
- Observed token scopes: `read:org`, `read:project`, `repo`
- No project mutation was submitted

## Verified Schema Capabilities

The live GraphQL schema reports the following fields:

- `ProjectV2.items(after, before, first, last, orderBy, archivedStates, query)`
- `ProjectV2.views(after, before, first, last, orderBy)`
- `ProjectV2View.layout`, `filter`, `fields`, `groupByFields`, `verticalGroupByFields`, and `sortByFields`
- `ProjectV2.layout` values: `BOARD_LAYOUT`, `TABLE_LAYOUT`, `ROADMAP_LAYOUT`
- `ProjectV2FieldConfiguration` union members: `ProjectV2Field`, `ProjectV2IterationField`, `ProjectV2MultiSelectField`, and `ProjectV2SingleSelectField`
- `ProjectV2ItemFieldValue` includes text, number, date, iteration, single-select, multi-select, issue, pull request, and other field-value unions
- Mutations: `updateProjectV2ItemFieldValue`, `clearProjectV2ItemFieldValue`, and `updateProjectV2ItemPosition`
- `ProjectV2.viewerCanUpdate`

The `items.query` argument is present in the target schema. This supersedes the earlier assumption in `PLAN.md` that server-side filtering might be absent, but behavior still requires representative saved-view validation.

## Read Probe Results

Results below are intentionally aggregate and contain no project names, item titles, IDs, or bodies:

- User-owned project discovery returned one project; its only sampled view was `TABLE_LAYOUT`.
- Organization membership discovery returned one organization and did not require another page.
- The accessible organization project list returned one open project and did not require another page.
- That project exposed five views: two boards, two tables, and one roadmap.
- The unfiltered position-ordered item probe returned 49 items, all issues or pull requests, on one page.
- The sampled saved filter `iteration:@current` was accepted by `items(query:)` and returned zero items.
- The board metadata probe returned visible-field metadata for all five views. Grouping and sorting metadata were present on the board views, including a board with vertical grouping and position sorting.
- The first board's field configuration decoded as title, assignees, three single-select fields, a number field, and an iteration field. Single-select options and iteration configuration are returned as inline lists, not cursor connections.
- Item reads use owner-specific GraphQL branches, position ordering, and the saved filter. Scalar project and issue-backed field values plus issue/pull-request repository, state, and aggregate sub-issue progress metadata are loaded during board reads; multi-valued label, user, reviewer, repository, milestone, and pull-request project-field values are identified but marked unavailable until their nested connections are deliberately loaded.

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

## Remaining Empirical Checks

The following items remain unverified:

- Field configuration option order, iteration state, and unset values
- Sort null placement and tie behavior
- Grouping-axis mapping and display settings
- Nested multi-valued field loading beyond the explicit unavailable marker
- Mutation permission and position-anchor semantics in a disposable sandbox

## Gate To Continue

Run the remaining probes against representative board views. A disposable sandbox with project write access is required before enabling or testing mutations. Until those checks are recorded, the implementation must remain read-only and must not claim rendering or write compatibility.
