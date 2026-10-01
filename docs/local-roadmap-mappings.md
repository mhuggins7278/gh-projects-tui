# User-configured roadmap endpoint mappings

Explicit local mappings were approved on 2026-09-30. They identify which DATE
fields the user wants to use for a specific project/view. They do not recover
GitHub's saved endpoint selections or other timeline settings.

## Configure

Copy [the example JSON](roadmap-mappings.example.json), replace its placeholder
node IDs, and pass the file explicitly:

```sh
gh projects-tui --roadmap-mappings ./roadmaps.json
# Or, from the repository:
go run ./cmd/gh-projects-tui --roadmap-mappings ./roadmaps.json
```

The file can contain multiple entries. Each requires `host`, `project_id`,
`view_id`, `start_field_id`, and `target_field_id`. These are opaque node IDs,
not project/view numbers or field names. Hosts use the app's usual normalization;
project/view/field IDs match exactly. A mapping cannot carry over to another
host, project, or view. Renaming a field or reordering its display does not
change its role. Recreated fields/views require new IDs.

The flag is optional. The app never discovers or writes a mapping file. Invalid
JSON, unknown keys, missing values, whitespace in IDs, and duplicate scopes fail
at startup before GitHub reads. Edit the file and restart to load changes.

## Get IDs using existing authentication

For an organization project, read the project and view node IDs:

```sh
gh api graphql -f owner=OWNER -F project=PROJECT_NUMBER -F view=VIEW_NUMBER \
  -f query='query($owner: String!, $project: Int!, $view: Int!) {
    organization(login: $owner) {
      projectV2(number: $project) { id view(number: $view) { id name } }
    }
  }'
```

Use `user(login: $owner)` instead of `organization(login: $owner)` for a personal
project. Use the same GitHub host as the app; `GH_HOST` can select that host for
both commands. List field IDs through the GitHub CLI:

```sh
gh project field-list PROJECT_NUMBER --owner OWNER --limit 1000 --format json
```

Choose the desired start and target fields explicitly. On each mapped view open,
the app resolves each ID uniquely against its complete, paginated project field
definitions and requires DATE types. Missing/stale/duplicate IDs and iteration,
text, or other types are rejected. Visible columns and endpoint-like names never
substitute for an ID. Selecting one DATE field for both roles is allowed by the
local resolver; this is not a claim about GitHub's endpoint-picker behavior.

## Current boundary

The saved-view picker labels matching entries as **user-configured mapping**.
Opening one reads complete project definitions and validates both selected IDs.
Direct `--owner/--project/--view` startup uses the same validation. Saved API
metadata and caches are not rewritten by the mapping.

## Supported read-only slice — 2026-10-01

A valid mapping opens a timeline when the saved filter is empty or exactly
`is:issue`, grouping is absent, and sorting is absent or one ascending sort on
the mapped start DATE field. Other roadmaps remain
in the picker with an explanation. This scope was approved on 2026-10-01;
the subsequent [populated comparison](roadmap-filter-order-verification-2026-10-01.md)
verified the bounded `is:issue` / start DATE ASC extension. Other filters/sorts
and live multi-page filtered comparisons remain deferred in #26.

The initial local range starts at the current month and spans three calendar
months. `j/k` or up/down selects rows; `h/l` or left/right shifts the range by one
month; `g` returns to the current month. `/` searches loaded rows, `enter` opens
detail, `o` opens the selected item (or project for drafts/unavailable URLs),
`r` rereads saved metadata and reloads items, and `v` returns to saved views.
Narrow terminals show a date list. Refresh preserves selected identity and the
local range; newly unsupported metadata returns the view to the picker.

Both endpoint days are included in valid bars. Same-day and single-endpoint
values show a point. Unset endpoints stay undated; unavailable content/values
stay unavailable; invalid or reversed dates get an invalid-date row without
swapping dates. The latter cases are conservative client policies, not claims
about GitHub's unobserved placement. All rows remain selectable, including dates
outside the local range. Counts describe loaded rows while pagination runs.

No timeline controls submit writes, including issue comments/close/reopen in
detail, project moves/reordering, archive/remove, or date rescheduling, even when
the viewer has write permission. Full item field connections are read by ID,
including nested pagination; the name-based selective board projection is not
used for endpoint resolution.

Saved zoom/range, markers, slicing, and field sums are unavailable and are
explicitly disclosed. Iteration endpoints and grouping remain deferred. These
local mappings do not recover GitHub's saved endpoint choices or establish exact
web parity. See [the API investigation](api-contract.md#saved-endpoint-mapping-decision-25)
and [the current roadmap scope](roadmap-proposal.md).
