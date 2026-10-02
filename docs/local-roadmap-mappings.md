# User-configured roadmap endpoint mappings

Explicit local mappings were approved on 2026-09-30. They identify which DATE or ITERATION
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
definitions and requires DATE or ITERATION types. Iteration definitions must have unique
IDs, valid calendar start dates, and positive durations with supported end dates.
Missing/stale/duplicate IDs, unresolved iteration definitions, text, or other
types are rejected. Visible columns and endpoint-like names never
substitute for an ID. Selecting one DATE field for both roles is allowed by the
local resolver; this is not a claim about GitHub's endpoint-picker behavior.

## Current boundary

The saved-view picker labels matching entries as **user-configured mapping**.
Opening one reads complete project definitions and validates both selected IDs.
Direct `--owner/--project/--view` startup uses the same validation. Saved API
metadata and caches are not rewritten by the mapping.

## Supported read-only timeline — 2026-10-01

A valid mapping opens DATE, ITERATION, or mixed endpoints with the same supported
saved-filter grammar as boards/tables. The exact saved filter is forwarded to
GitHub on every page. No filter substitution or local saved-filter evaluation is
used. Sorting supports up to two distinct supported fields, ASC or DESC, with
unset values last and stable project-position ties. Selection follows item ID.
Grouping is absent or one single-select field without vertical grouping.
Unsupported filter syntax, types, directions, definitions or grouping remain
in the picker with an explanation.

An iteration start role resolves to its configured start day; a target role
resolves to start day plus duration minus one. Both current and completed
iterations are resolved by ID against complete project definitions, even when
not visible in the view. Missing membership stays unset; unknown, duplicate or
unavailable values stay unavailable. No current-iteration guess is made.

The authorized sandbox comparisons and regression coverage are recorded in
[the completion note](roadmap-completion-2026-10-01.md).

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
explicitly disclosed. Multiple/vertical/iteration grouping remains unsupported. These
local mappings do not recover GitHub's saved endpoint choices or establish exact
web parity. See [the API investigation](api-contract.md#saved-endpoint-mapping-decision-25)
and [the current roadmap scope](roadmap-proposal.md).

## Single-select grouped timelines

The user requested general enablement on 2026-10-01 without requiring access to
an existing work board from this computer. A mapped timeline can now open
with one saved single-select grouping field. Its field ID must resolve uniquely
against project definitions; the saved grouping connection must supply
nonempty, unique option IDs.

This uses a disclosed local display policy: API field-option order, a final
“No value” section, and all groups expanded. Empty options remain visible with
zero loaded items. Unknown, duplicate or unavailable group values remain in an
“Unavailable group” section. Group membership matches option IDs, never names.
Within each section, incoming project position or the supported mapped start
DATE ASC order is retained. This policy is not a verified restoration of saved
custom group order, collapsed state, or web unset-section placement.

No extra grouping flag is needed. On your work computer, use its existing GitHub
CLI authentication and create the DATE mapping there using the instructions
above. Then select the roadmap or launch it directly:

```sh
go run ./cmd/gh-projects-tui --owner OWNER --project NUMBER --view NUMBER --roadmap-mappings ./roadmaps.json
```

Selection, search, progressive pages, lazy detail, refresh, range controls and
read-only guards work for grouped and ungrouped timelines. Multiple, vertical,
iteration and multi-valued grouping are rejected with an explanation.
