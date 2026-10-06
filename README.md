# gh-projects-tui

A terminal UI for GitHub Projects v2, built with Go and Bubble Tea.

**Status: pre-alpha.** Releases are private and installable by users with access
to this repository. Features, keybindings, and supported view types are still
evolving.

## Screenshots

Captured from the running TUI against a disposable GitHub Projects sandbox.
The board and table show the same saved project with live issues and fields;
the detail panel loads an issue on demand.

### Board: saved Status lanes and issue cards

![Board view](docs/screenshots/board.png)

### Table: grouped rows, saved columns, and sub-issue progress

![Table view](docs/screenshots/table.png)

### Issue details: project fields and Markdown body

![Issue detail overlay on a board](docs/screenshots/detail.png)

## Install the GitHub CLI extension

The repository is private, so first authenticate with an account that can read
it and check the active host:

```sh
gh auth login
gh auth status
```

Install and run the extension:

```sh
gh extension install mhuggins7278/gh-projects-tui
gh projects-tui
```

Update it when a new release is available:

```sh
gh extension upgrade projects-tui
```

Remove it with `gh extension remove projects-tui`. Release tags trigger the
release workflow, which builds precompiled binaries and attaches them to the
private GitHub Release. The first-party `gh-extension-precompile` action
publishes assets that `gh extension install` can select for the local platform.

## Run locally

### Prerequisites

- Go **1.25 or newer**.
- [GitHub CLI (`gh`)](https://cli.github.com/), authenticated with an account that
  can access the projects you want to use.
- Access to this repository and an interactive terminal.

Authenticate and check your login:

```sh
gh auth login
gh auth status
```

For a GitHub CLI OAuth login, grant project access and organization discovery
scopes if they are missing:

```sh
gh auth refresh -s project -s read:org -s repo
```

The `project` scope enables project writes; `repo` is needed for private repository
content. The app uses your existing GitHub CLI authentication. Organizations
blocked by SAML enforcement for your token are skipped during discovery.

### Clone and start

```sh
gh repo clone mhuggins7278/gh-projects-tui
cd gh-projects-tui
go run ./cmd/gh-projects-tui
```

Running without flags discovers your personal projects and accessible organization
projects, then opens the owner picker. The app does not persist or automatically
restore the previous owner, project, or view.

To open a specific project/view directly:

```sh
go run ./cmd/gh-projects-tui --owner OWNER --project NUMBER --view NUMBER
```

You can also pass only `--owner` to start at that owner's project picker.
`--project` requires `--owner`, and `--view` requires both `--owner` and
`--project`. Pass `--debug` (or set `GH_PROJECTS_TUI_DEBUG=1`) to show API
telemetry in the header and enable the `A` inspector overlay, including measured
GraphQL points by operation.

Roadmap views currently appear in the saved-view picker as unsupported and
cannot be opened. The API does not expose enough saved-view configuration to
render them faithfully; open roadmap views in GitHub.

### Build a local binary

From the repository directory:

```sh
go build -o gh-projects-tui ./cmd/gh-projects-tui
./gh-projects-tui
# Or:
./gh-projects-tui --owner OWNER --project NUMBER --view NUMBER
```

After pulling updates, rerun `go run` or rebuild your local binary.

## Current features

- Owner, project, and saved-view pickers with filtering.
- Progressive board loading through one paginated stream with selective fields.
- Single-select, multi-select, and iteration grouping, including combined columns/swimlanes.
  Multi-select items appear in each selected option group; group moves and
  manual reordering remain disabled for these views.
- Saved visible fields, supported field sorting, and local card search.
- Tables render saved fields, single-field groups, linked PRs, and expandable
  sub-issue hierarchies. Nested children load on demand, including children
  outside the saved filter. Writable tables can archive or remove project items,
  change groups, edit saved text/number/date/select/iteration columns, and
  reorder position-sorted top-level rows. Standard columns include labels,
  milestone, repository, reviewers, assignees, and linked PRs.
- Issue, pull request, and draft details, including Markdown bodies and project
  fields. Details are cached in memory until refresh or view changes.
- Optimistic card moves and reordering on supported writable boards.

### Card moves and saves

Moves appear immediately. Additional moves can be queued while GitHub saves them
one at a time, and card navigation stays responsive. A definitive final success
on the same unfiltered board keeps the session's saved state without a reload.
Failed or partially completed writes are reconciled before dependent moves
continue; unknown outcomes block further writes, and `r` checks the outcome
without resubmitting the write.

View changes and manual refresh can continue during saves. Submitted writes stay
attached to their original project and view. If a save overlaps an item refresh,
the refresh finishes and a fresh read follows it so stale pages cannot replace
the saved state. Use `r` to pick up external changes.

## Keys

| Key | Action |
| --- | --- |
| `j/k` or `↑/↓` | Move through picker entries or cards |
| `h/l` or `←/→` | Navigate board lanes |
| `h/l` or `←/→` (table) | Collapse/select the parent / expand sub-issues |
| `space` (table) | Toggle the selected row's sub-issues |
| `enter` | Select a picker entry or open item details |
| `H/L` (uppercase) | Move the selected card to an adjacent lane |
| `J/K` (uppercase) | Reorder the selected card within its lane |
| `e` (table) | Choose a saved field to edit; enter saves, esc cancels; space toggles multi-select options |
| `m` (table) | Pick a saved group for the selected row; `j/k` choose and `enter` moves |
| `J/K` (table) | Reorder the selected top-level row within its group when project-position sorted |
| `a` / `D` (table) | Archive / remove the selected item from this project (confirmation required) |
| `/` | Filter picker entries or search loaded cards |
| `enter` / `esc` while searching | Finish search / clear search |
| `esc` or `backspace` | Go back |
| `v` | Pick a saved view |
| `p` | Pick a project |
| `r` | Refresh saved-view settings and items from GitHub; retry readback when a save outcome is unknown |
| `o` | Open the selected item or project in a browser |
| `?` | Toggle help |
| `q` or `ctrl+c` | Quit |

In item details, `j/k` scrolls, `f` toggles all fields (including unset/unavailable
values), and `esc` closes the panel. For GitHub issues on boards and tables, `c` opens a comment
composer (`enter` submits, `esc` cancels), and `x` prompts to close or reopen the
issue (`enter` confirms, `esc` cancels). Pull requests and draft issues do not
offer these issue actions.

## Pre-alpha limitations

- Roadmap views are unsupported in this release and remain in the picker with
  an explanation. The TUI does not guess which project fields GitHub uses for
  the roadmap date axis.
- Saved filters are sent unchanged to GitHub's `ProjectV2.items(query:)` on
  every page and refresh, including custom fields such as `has:otc-sprint`.
  GitHub determines syntax and item membership; the TUI has no local filter
  allowlist or field-name/type restrictions. API errors are displayed without
  retrying as an unfiltered view. `/` searches only already-loaded cards and
  does not change the saved GitHub query.
  See [the filter contract and evidence](docs/api-contract.md#saved-filter-grammar-and-evidence)
  for observed server behavior and web/API differences. Grouping and sorting
  have separate compatibility checks because the TUI implements them locally.
- If a project has no compatible saved board, unsupported views stay in the
  picker with an explanation; the TUI never substitutes an unfiltered board.
- Card mutations require a writable, fully loaded board with
  single-select or iteration grouping. Combined boards require supported fields
  on both axes; `H/L` moves only between columns in the current swimlane.
  Clear local search before moving cards.
- Manual reordering requires project-position sorting and is disabled for
  combined-axis boards. Moving cards between swimlanes remains read-only.
  Supported saved-filtered boards use unfiltered project state to resolve writes;
  moves can affect hidden items and other views because position is project-wide.
- Writes are serialized and re-resolved against the latest project order.
  Rapid unsent moves of the same card collapse to the final placement.
  If a timeout leaves a save's outcome unknown, dependent writes pause until
  `r` confirms the state; `r` retries readback only and never resubmits the write.
  Switching views does not cancel a submitted write.
- Board loads use one paginated stream with only grouping, sort, and card
  fields; recent views/metadata are cached briefly and `r` forces a refresh.
  API cooldowns pace writes; `--debug` shows their countdown and request telemetry.
- Table support is an early preview; full parity with GitHub's table UI is not
  implemented. Table actions require update access, a complete item load, and
  no local search. Field editing uses the saved visible columns; empty text,
  number, or date input clears the field. Dates must be `YYYY-MM-DD`, and
  numbers must be finite. Issue-backed fields require issue field permissions
  and are saved on the issue; project-only fields use project permissions.
  System metadata columns remain read-only. Group moves require one writable single-select or iteration
  grouping field; reordering requires project-position sorting. Removing a
  project-only draft deletes that draft, while removing an issue or PR leaves
  its repository content intact. Archiving retains an item for restoration on
  GitHub. Unknown outcomes keep a write gate across navigation; `r` checks
  the originating project's unfiltered active items before unlocking further writes.

Owner, project, and view selections are not saved between runs. Use the explicit
flags above when you want to open a specific board directly.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

The default tests use local fixtures. Opt-in live API tests and the verified
GitHub API behavior are documented in [docs/api-contract.md](docs/api-contract.md).

The live smoke test is read-only and opt-in. Supply an owner and project/view
that your authenticated `gh` account can read; set owner kind to `user` for a
personal project (organization is the default):

```sh
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
GH_PROJECTS_TUI_LIVE_VIEW=VIEW_NUMBER \
go test -tags live ./internal/github
```

It runs discovery, opens the selected view, pages items, and loads one item's
detail. It does not submit mutations. Mutation verification belongs only on the
disposable sandbox project documented in `docs/api-contract.md`.

The saved-filter contract test is also read-only. It requires an existing project
with populated closure dates and parent-linked child issues, checks membership
against a complete unfiltered metadata baseline, and forces two-item pages to
exercise cursor pagination. It also checks created/updated date forms and Status
OR, repeated-qualifier AND, negation, and cross-field AND when two populated
Status values exist. Missing optional samples are reported. The
[2026-09-30 filter audit](docs/filter-audit-2026-09-30.md) records the results and the
distinction between project-item and issue update timestamps:

```sh
GH_PROJECTS_TUI_LIVE_FILTERS=1 \
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_OWNER_KIND=user \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
go test -tags live ./internal/github -run '^TestLiveSavedFilterContract$' -count=1
```

The read-only [roadmap investigation probe](docs/api-contract.md#reproducible-read-only-probe)
can inspect an existing saved roadmap's schema and aggregate date/iteration data.
It does not enable roadmap rendering or change project data.

### Saved-view compatibility probes

Compatibility work targets **github.com**. Enterprise schema negotiation is not
implemented. The read-only parity probe compares GraphQL saved-filter membership
with REST saved-view membership, including cursor pagination, and repeats the
GraphQL read to detect a changing sample. It reports aggregate counts only:

```sh
GH_PROJECTS_TUI_LIVE_PARITY=1 \
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
GH_PROJECTS_TUI_LIVE_VIEW=VIEW_NUMBER \
go test -tags live ./internal/github -run '^TestLive(SavedViewMembershipParity|EnhancedRowReads)$' -count=1 -v
```

Set `GH_PROJECTS_TUI_LIVE_OWNER_KIND=user` for personal projects. REST comparison
uses API version `2026-03-10`; it does not replace the production GraphQL loader.
The enhanced row probe checks visible standard metadata and reads sub-issues
when a parent exists in the first sampled page. Neither probe submits writes.
Sort fixtures cover both directions, unset values, stable position ties,
secondary sorts, and completed/unknown iterations. They pin the current client
policy; they do not establish universal agreement with GitHub's web renderer.
