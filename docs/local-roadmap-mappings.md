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
Opening one reads its metadata and reports the resolved start/target names or an
actionable validation error. Direct `--owner/--project/--view` startup uses the
same validation. Saved API metadata and caches are not rewritten by the mapping.

**The live timeline is still gated on placement verification.** A valid mapping
reports that remaining gate and stays in the picker. It starts no item load or
mutation path, even with write permission. The synthetic three-month renderer
remains fixture-only. These mappings provide an explicit local source for future
verification/integration; they do not complete #26/#28 or imply saved-view parity.

The [API investigation](api-contract.md#saved-endpoint-mapping-decision-25)
records the unavailable automatic source. The
[roadmap proposal](roadmap-proposal.md) tracks placement, integration, grouping,
and iteration prerequisites.
