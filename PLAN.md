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
- Server-side saved-view filtering remains behaviorally unverified: the target GitHub.com schema exposes `query` alongside pagination and `orderBy` on `ProjectV2.items`, but representative saved-view behavior still needs validation. A schema mirror may lag the target host.
- Lane changes use `updateProjectV2ItemFieldValue`; moving to “No value” uses `clearProjectV2ItemFieldValue`; manual ordering uses `updateProjectV2ItemPosition`. Its `afterId` is a project-wide anchor, and omitting it moves the item to the top of the project.

## API Contract Gate (Slice 0)
- Record the target host, schema capabilities, required authentication/membership, and sanitized working queries for discovery, view metadata, item filtering, field loading, and position order.
- Compare representative saved views with GitHub, including compound/relative filters, empty values, grouping axes, and sort ties. Metadata availability alone does not prove rendering parity.
- Prefer server-side filtering only after verifying it on the target host. If unavailable, explicitly define and validate a bounded local-filter subset before implementation; arbitrary GitHub filter-language emulation is beyond MVP. Unrecognized filters make a view unsupported, never silently unfiltered.
- Finalize the compatibility matrix below with exact supported field/filter/sort types and observed API limitations before building the board. Prove mutation and insertion-anchor semantics only in a disposable sandbox.
- Exit condition: sanitized examples and the completed matrix demonstrate the intended work-board read path and supported sandbox writes. Unproven capabilities remain unsupported.

## Saved-View Compatibility Contract
Current implementation coverage and empirical limits are tracked in [docs/api-contract.md](docs/api-contract.md#current-compatibility-matrix). This table describes the intended contract; fixture-only behavior is identified separately in that record.

| Feature | MVP contract | Unsupported behavior |
| --- | --- | --- |
| Layout | Saved `BOARD_LAYOUT` views. | Explain why table/roadmap views cannot open as boards. |
| Filter | Verified server-side behavior, or the explicit locally evaluated subset established in Slice 0. | Block the saved view with an explanation; do not substitute an unfiltered board. |
| Column/swimlane grouping | Single-select and iteration projection first; preserve option order, distinguish active/completed iterations, and include unset values. Validate which metadata axis maps to each board axis. | Other types require validated projection before rendering. Multi-valued groups must define duplicate-card identity/navigation; otherwise mark the view unsupported. |
| Sorting | Project position plus an explicit Slice 0 list of supported field sorts, null placement, comparison rules, and tie-breaking. | Unsupported sort semantics make the view unsupported; supported explicit field sorts disable manual reorder. |
| Visible fields/detail | Validated card-field renderers; detail combines project field definitions with values so unset fields appear too. | Mark unavailable values/content explicitly, distinct from empty values/body. |
| Display settings | Reproduce settings exposed and verified through the API. Record unexposed settings, including any hidden/collapsed group or custom group-order differences. | Display known fidelity limitations when opening an otherwise compatible view. |
| Writes | Separate capabilities for rendering, lane movement, and reorder. | A renderable grouping without validated mutation semantics is read-only, not an unsupported view. |

## Product Shape
1. **Startup and discovery**
   - Use `go-gh` so authentication, `GH_HOST`, environment tokens, and existing `gh auth` configuration behave like native `gh` commands.
   - Query the viewer plus organizations returned by the authenticated organization-membership API, then paginate each owner’s open projects explicitly. This is not an exhaustive enumeration of all accessible owners; token visibility and outside-collaborator access can differ.
   - Keep successful discovery results usable when an individual organization fails, with an owner-specific explanation. Direct owner lookup operates independently of membership discovery.
   - Present a searchable owner/project picker. Do not persist or automatically restore the previous selection; direct startup uses explicit flags.
   - `--owner` accepts a login; `--project` and `--view` accept owner-scoped project and project-scoped view numbers respectively. A project number requires an explicit owner, and a view number requires explicit owner and project flags. Missing selections open the corresponding picker.
   - Never persist item titles, bodies, field values, or tokens.

2. **Saved-view selection**
   - List the selected project’s saved views and support `BOARD_LAYOUT` in the MVP.
   - Apply the view’s filter, visible fields, column grouping, optional swimlane grouping, and sort rules according to the compatibility contract; explain incompatibility before rendering.
   - Preserve field-option order and include a “No value” lane where applicable.
   - If no compatible board view exists, explain incompatibility; never silently switch to an unfiltered Status board.
   - Show table and roadmap views as unsupported for now, with a clear explanation.

3. **Board experience**
   - Use a three-part layout: project/view header, scrollable Kanban board, and item detail pane.
   - Wide terminals show details beside the board; narrow terminals open details as a full-screen overlay.
   - Cards display the saved view’s visible fields compactly. The detail pane shows all project fields, including unset values, and rendered Markdown body. Inaccessible/deleted content has an explicit placeholder rather than an empty-body presentation.
   - Core keys: `h/l` lanes, `j/k` cards, `/` local search, `enter` details, `p` project picker, `v` view picker, `r` refresh, `o` browser, `?` help, and `q` quit.

4. **Responsive data loading**
   - Load project/view/field metadata first, then paginate compatible Status lanes concurrently behind per-lane loading states. Reveal each lane when its grouping and saved sorting are stable; use whole-board pagination for unsupported grouping configurations.
   - Fetch only core metadata plus fields required for grouping, sorting, and cards during board load.
   - Lazy-load the selected item’s full field set and body after a short selection debounce; cache details in memory only. Paginate nested connections as needed, including field values and multi-valued fields, and combine them with project field definitions.
   - Cancel obsolete reads when the user switches projects or views, avoid N+1 requests, and surface rate-limit/auth failures inside the TUI. Tag asynchronous messages with host/project/view generation and item identity where applicable; discard stale read results even if cancellation arrives too late.
   - Treat write completion separately from read cancellation: changing views does not establish that a submitted mutation was cancelled. Reconcile its outcome against the originating project without applying stale UI updates to the current view.

5. **Movement and ordering safety**
   - Enable writes only when `viewerCanUpdate` and the selected grouping field has a supported mutation.
   - Support writable single-select and iteration lanes first; render other grouping types read-only until their mutation semantics are deliberately added.
   - Use deliberate shifted keys for movement: `H/L` across lanes and `J/K` within a lane.
   - Apply optimistic UI updates and serialize mutations through a project-wide queue. Resolve each queued lane/reorder intent against a complete, unfiltered project-position readback; never retain a queued anchor or rollback snapshot across writes.
   - Reconcile submitted writes against canonical project state. A timeout/disconnection or failed readback leaves the outcome unknown and blocks dependent writes; `r` retries readback only. View changes do not cancel submitted writes or let their results overwrite another project's UI.
   - Moving into “No value” clears the grouping field; moving out sets an option/iteration ID. Cross-lane moves preserve the swimlane value; moving between swimlanes is outside MVP.
   - In position-sorted views, cross-lane moves update/clear the grouping field and then append after the last visible item in the destination lane/swimlane. An empty destination preserves the card's existing project position and needs no position mutation. If only one mutation succeeds, reload and report the partial result clearly.
   - Disable position-changing controls until the view is fully loaded and whenever local search is active. For same-lane reorder, insert after the desired visible predecessor. For insertion before the first visible card, resolve that card's immediate project-wide predecessor from canonical position order, excluding the moving item; omit `afterId` only for the actual project top. Fetch any missing anchor context before submitting.
   - Filtered-out items are not movement targets. Preserve their relative order, but explain that the moved card's relation to hidden cards and other views can change because GitHub position is project-wide. Refresh/reconcile after writes; concurrent external edits can invalidate anchors.
   - Disable manual reorder when the saved view has an explicit non-position sort; cross-lane moves in those views update/clear the field only and let the sort determine placement.
   - Re-evaluate membership after a successful move. If the card no longer matches the saved filter, explain its disappearance and focus the next remaining card, then the previous card, or the empty lane if neither exists.

## Module Seams
- `internal/github`: one deep Projects v2 module hiding owner branching, GraphQL unions, pagination, schema translation, auth errors, and mutations. Its small interface covers discovery, opening a view, paging items, loading detail, moving, and reordering.
- `internal/board`: pure domain model for fields, lane/swimlane projection, saved-view filtering metadata, ordering, and mutation rollback state.
- `internal/ui`: Bubble Tea screens and state transitions; network work runs as commands and returns generation-tagged typed messages, with cancellable reads and separately reconciled write outcomes.
- `internal/config`: GitHub host resolution only; user selections are not persisted.
- Tests use a fake Projects adapter at the same seam; no production-only abstraction layers or pass-through wrappers.

## Implementation Slices
0. Validate the API contract and finalize the compatibility matrix, sanitized query examples, and sandbox position/mutation semantics before scaffolding.
1. Scaffold the private precompiled Go extension, dependency pinning, CI, and sanitized fixture strategy.
2. Implement authenticated owner/project discovery with complete cursor pagination and actionable scope/SSO errors.
3. Load saved board metadata and render the project/view picker.
4. Deliver a read-only vertical slice: filtered progressive item loading, columns/swimlanes, visible card fields, search, and refresh.
5. Add lazy item detail loading and Markdown rendering for issues, pull requests, and drafts.
6. Add lane movement and ordering with permission checks, optimistic state, partial-failure recovery, and refresh verification.
7. Package tagged binaries with `cli/gh-extension-precompile`, install from the private release, and document keybindings, scopes, and limitations.

## Verification
- Unit tests: owner discovery with partial failures and independent direct lookup, explicit flag validation, nested cursor pagination, GraphQL union decoding, unset/inaccessible fields, supported filters/sorts, null/tie handling, lane/iteration ordering, unsupported grouping, and auth/error classification.
- Model tests: fixed-size Bubble Tea navigation, loading, resize, search, detail overlay, read-only mode, and error states; identity-stable selection during progressive sorting; obsolete responses after selection/view/project changes; and control gating during loading/search.
- Mutation tests: same-lane reorder, first/empty destination, hidden-item anchors, cross-lane move preserving swimlane, clear/set “No value,” sorted-view field-only moves, filter-driven disappearance/focus, denied permission, stale item/anchor, first/second mutation failure, rollback/refetch, interleaved moves on different items, and ambiguous completion after timeout or view switch.
- Performance fixture: at least 1,000 sanitized items with deliberately delayed pages. On a documented reference machine and terminal size, target p95 input-to-model/view-update latency under 100 ms during loading; show the first page before the final page arrives and keep navigation responsive during detail fetching. Record timings and request counts.
- Opt-in live smoke test: discovery and view loading only against real work projects; automated mutation tests use a disposable sandbox project, never a work board.
- Release check: install the tagged private extension on macOS arm64 and run `gh projects-tui`; CI also builds Linux amd64.

## Acceptance Criteria
- Work organization projects appear automatically when the authenticated membership API includes that organization and the token has the required project access/SSO authorization. Direct owner/project selection works independently of membership discovery; one owner's failure does not hide successful results.
- A supported saved board view reproduces its filter, columns, swimlanes, visible fields, and sort behavior according to the completed compatibility matrix. Unsupported semantics are explained before rendering, and known display-only differences are visible.
- Selecting a card exposes all project fields, including unset values, and its accessible body without eagerly downloading every body; unavailable content is clearly distinguished.
- Authorized moves/reorders persist after refresh according to the documented project-wide anchor policy; unsupported or read-only views never offer misleading write controls. Ambiguous or partial results are reconciled and explained.
- Large projects meet the documented responsiveness target while loading, preserve selection as pages arrive, and explain corrective actions without leaking tokens or private item data.

## Deferred Beyond MVP
Editing arbitrary fields; moving cards between swimlanes; arbitrary GitHub filter-language emulation; creating/adding/archiving items; comments and history; PR checks/reviews; issue/PR actions; exact table/roadmap rendering; offline item caching; public distribution; and local custom views.
