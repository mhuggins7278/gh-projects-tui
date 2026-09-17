# Plan: `gh-projects-tui`

## Goal
Build a private, precompiled GitHub CLI extension invoked as `gh projects-tui` that reliably discovers personal and organization-owned Projects v2, mirrors saved board views, shows rich item detail, and supports safe card movement and reordering.

## Confirmed Decisions
- Stack: Go, `github.com/cli/go-gh/v2`, and Bubble Tea v2.
- Repository: new private `mhuggins7278/gh-projects-tui` repository (working name).
- Source of truth: GitHub saved board views, not a locally invented Status board.
- MVP detail: every project field plus the issue, pull request, or draft body.
- MVP mutations: move cards between lanes and reorder cards.
- No implementation or external mutation is authorized by this plan alone.

## Evidence Guiding the Design
- The current token can read organization projects when `glg` is queried explicitly, while `viewer.projectsV2` only represents user-owned projects. Discovery must enumerate owners rather than assume “viewer” means “all accessible projects.”
- Projects v2 GraphQL exposes saved-view layout, filter, visible fields, grouping, vertical grouping, and sorting metadata.
- A saved-view filter can be passed to `ProjectV2.items(query:)`, so filtering can remain server-side.
- Lane changes use `updateProjectV2ItemFieldValue`; manual ordering uses `updateProjectV2ItemPosition`.

## Product Shape
1. **Startup and discovery**
   - Use `go-gh` so authentication, `GH_HOST`, environment tokens, and existing `gh auth` configuration behave like native `gh` commands.
   - Query the viewer plus every visible organization, then paginate each owner’s open projects explicitly.
   - Present a searchable owner/project picker, remember only the last owner/project/view identifiers, and support direct `--owner`, `--project`, and `--view` flags.
   - Never persist item titles, bodies, field values, or tokens.

2. **Saved-view selection**
   - List the selected project’s saved views and support `BOARD_LAYOUT` in the MVP.
   - Apply the view’s filter, visible fields, column grouping, optional swimlane grouping, and sort rules.
   - Preserve field-option order and include a “No value” lane where applicable.
   - If no compatible board view exists, offer an explicit Status-board fallback rather than silently changing semantics.
   - Show table and roadmap views as unsupported for now, with a clear explanation.

3. **Board experience**
   - Use a three-part layout: project/view header, scrollable Kanban board, and item detail pane.
   - Wide terminals show details beside the board; narrow terminals open details as a full-screen overlay.
   - Cards display the saved view’s visible fields compactly. The detail pane shows all project field values and rendered Markdown body.
   - Core keys: `h/l` lanes, `j/k` cards, `/` local search, `enter` details, `p` project picker, `v` view picker, `r` refresh, `o` browser, `?` help, and `q` quit.

4. **Responsive data loading**
   - Load project/view/field metadata first, then paginate board items and render pages progressively.
   - Fetch only core metadata plus fields required for grouping, sorting, and cards during board load.
   - Lazy-load the selected item’s full field set and body after a short selection debounce; cache details in memory only.
   - Cancel obsolete requests when the user switches projects or views, avoid N+1 requests, and surface rate-limit/auth failures inside the TUI.

5. **Movement and ordering safety**
   - Enable writes only when `viewerCanUpdate` and the selected grouping field has a supported mutation.
   - Support writable single-select and iteration lanes first; render other grouping types read-only until their mutation semantics are deliberately added.
   - Use deliberate shifted keys for movement: `H/L` across lanes and `J/K` within a lane.
   - Apply optimistic UI updates, serialize mutations per item, and roll back or refetch canonical state on failure.
   - For a cross-lane move, update the grouping field and then position; if only one mutation succeeds, reload and report the partial result clearly.
   - Disable manual reorder when the saved view has an explicit non-position sort. Document that GitHub item position is project-wide and can affect other views.

## Module Seams
- `internal/github`: one deep Projects v2 module hiding owner branching, GraphQL unions, pagination, schema translation, auth errors, and mutations. Its small interface covers discovery, opening a view, paging items, loading detail, moving, and reordering.
- `internal/board`: pure domain model for fields, lane/swimlane projection, saved-view filtering metadata, ordering, and mutation rollback state.
- `internal/ui`: Bubble Tea screens and state transitions; network work runs as cancellable commands and returns typed messages.
- `internal/config`: minimal XDG-compatible preference storage for non-sensitive identifiers only.
- Tests use a fake Projects adapter at the same seam; no production-only abstraction layers or pass-through wrappers.

## Implementation Slices
1. Scaffold the private precompiled Go extension, dependency pinning, CI, and sanitized fixture strategy.
2. Implement authenticated owner/project discovery with complete cursor pagination and actionable scope/SSO errors.
3. Load saved board metadata and render the project/view picker.
4. Deliver a read-only vertical slice: filtered progressive item loading, columns/swimlanes, visible card fields, search, and refresh.
5. Add lazy item detail loading and Markdown rendering for issues, pull requests, and drafts.
6. Add lane movement and ordering with permission checks, optimistic state, partial-failure recovery, and refresh verification.
7. Package tagged binaries with `cli/gh-extension-precompile`, install from the private release, and document keybindings, scopes, and limitations.

## Verification
- Unit tests: owner discovery, cursor pagination, GraphQL union decoding, view projection, lane ordering, unsupported grouping, and auth/error classification.
- Model tests: fixed-size Bubble Tea navigation, loading, resize, search, detail overlay, read-only mode, and error states.
- Mutation tests: same-lane reorder, cross-lane move, denied permission, stale item, first/second mutation failure, rollback, and refetch behavior.
- Performance fixture: at least 1,000 sanitized items to prove progressive rendering and responsive navigation.
- Opt-in live smoke test: discovery and view loading only against real work projects; automated mutation tests use a disposable sandbox project, never a work board.
- Release check: install the tagged private extension on macOS arm64 and run `gh projects-tui`; CI also builds Linux amd64.

## Acceptance Criteria
- Work organization projects appear without manually typing the organization, and direct owner/project selection also works.
- A saved board view reproduces its filter, columns, swimlanes, visible fields, and sort behavior within documented API limits.
- Selecting a card exposes all project fields and its body without eagerly downloading every body.
- Authorized moves/reorders persist after refresh; unsupported or read-only views never offer misleading write controls.
- Large projects remain interactive while loading, and failures explain the corrective action without leaking tokens or private item data.

## Deferred Beyond MVP
Editing arbitrary fields; creating/adding/archiving items; comments and history; PR checks/reviews; issue/PR actions; exact table/roadmap rendering; offline item caching; public distribution; and local custom views.
