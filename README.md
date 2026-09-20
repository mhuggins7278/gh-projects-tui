# gh-projects-tui

Private, precompiled GitHub CLI extension for GitHub Projects v2.

The current binary is a read-only board preview. It discovers owners and
projects, opens saved views, progressively loads issues/pull requests/drafts,
and groups cards by saved single-select or iteration metadata. Press `enter` on
a card to load its project fields and accessible issue, pull request, or draft
body in the detail panel. Project mutations remain disabled until sandbox
semantics are verified.

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

`--project` requires `--owner`; `--view` requires both. The current binary
validates and loads the selected owner/project/view but remains read-only.

On a loaded board, `h/l` changes lanes, `j/k` changes cards, and `enter` opens
detail. Use `r` to refresh, `v` to pick a view, `p` to pick a project, and `?`
for help.
