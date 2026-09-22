# gh-projects-tui

Private, precompiled GitHub CLI extension for GitHub Projects v2.

The current binary is a board and table preview with guarded mutation support. It discovers owners and
projects, opens saved views, and progressively fetches issues, pull requests,
and drafts behind per-lane loading states. Compatible Status lanes load in
parallel and appear independently once their grouping and saved sorting are
stable. Board cards show populated fields from the saved view alongside content
state/type, repository and number when available, sub-issue progress when
available, and a wrapped title. Table views render saved fields as read-only rows. Press `enter`
on a card to load its project fields and accessible issue, pull request, or
draft body in the detail panel. Writable single-axis boards support guarded
card moves, with same-lane reordering on position-sorted views; combined,
filtered, incomplete, and unsupported views remain read-only.

## Development

```sh
go test ./...
go run ./cmd/gh-projects-tui
```

The installed extension command will be `gh projects-tui`.

Direct selection is available without membership discovery:

```sh
gh projects-tui --owner OWNER --project NUMBER --view NUMBER
```

`--project` requires `--owner`; `--view` requires both. Unsupported or
incomplete views remain read-only.

On a loaded board, `h/l` changes lanes, `j/k` changes cards, `/` searches
loaded cards, and `enter` opens detail. Use `r` to refresh, `v` to pick a view,
`p` to pick a project, and `?` for help.

Board views currently open `BOARD_LAYOUT` views with no saved filter or
the verified `iteration:@current` filter, one single-select or iteration axis,
including one column axis plus one vertical swimlane axis, and supported sort
fields. Table views are read-only and render their saved fields in rows.
Unsupported roadmap and view semantics remain in the picker with an explanation
instead of being rendered with a misleading fallback.
