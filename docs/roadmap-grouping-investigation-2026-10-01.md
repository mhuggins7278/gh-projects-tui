# Grouped DATE roadmap sample search — 2026-10-01

The initial read-only timeline integration and its large-project checks are
complete. #29 no longer lacks the initial timeline or the approved local DATE
mapping source. Its remaining prerequisite is an existing populated single-select
grouped DATE roadmap for web/API comparison.

A bounded search inspected six additional published project references. Each
project's public flag was checked before reading configuration. Five projects
were confirmed public; the sixth was unavailable or not confirmed public, and its
metadata was not read or retained. Of the five inspected public projects, four
had no roadmap layout and one had an ungrouped roadmap with no DATE fields. None
provided the required grouped DATE sample. No project settings/items, credentials,
or permissions were changed. Names, IDs, item content, and exact dates are omitted.

Therefore horizontal/vertical grouped roadmaps remain explicitly unsupported.
Generic single-select option order and the table grouping implementation do not
establish roadmap section order, unset placement, sorting within sections, or
collapsed/custom group-order behavior. The fixture's ungrouped DATE sorting
contract must not silently flatten a grouped view.

## Fixture renderer checkpoint

`internal/ui/grouped_date_timeline.go` now renders caller-supplied sections with
loaded counts and explicit section/row order. It shares DATE row formatting with
the ungrouped renderer, rejects missing/duplicate identities, and preserves
selection by item ID. Heading lines consume the row-window budget; windows that
start within a section carry its heading when terminal height permits.

Synthetic tests cover populated, empty and unset sections, paging/search/section
movement, date classifications and clipping, Unicode/control text, and narrow or
short terminals. The preview labels groups synthetic and discloses unavailable
collapsed/custom ordering. This renderer has no live adapter or compatibility
change; fixtures do not establish GitHub grouping semantics.

`go test -race ./...`, `go vet ./...`, `go vet -tags live ./...`, and the opt-in
synthetic preview passed. A 100-section/10,000-row render benchmark at 120×30
measured approximately 4.57 ms/op, 1,600,057 B/op and 301 allocs/op on Linux amd64
(Intel Core Ultra 9 185H, 200 ms benchmark duration). This is renderer throughput,
not live API latency or an end-to-end responsiveness measurement.

```sh
GH_PROJECTS_TUI_TIMELINE_PREVIEW=1 go test ./internal/ui -run '^TestGroupedTimelineSyntheticPreview$' -count=1 -v
go test ./internal/ui -run '^$' -bench '^BenchmarkGroupedTimelineLargeFixture$' -benchmem
```

Resume #29 with an existing roadmap using explicitly observed DATE start/target
roles and one single-select grouping field. Compare populated and unset sections,
option order, membership, and supported sorting within each section; record which
collapsed/custom display state is unavailable. Then connect the fixture renderer
to the verified grouping projection and live selection lifecycle. Multiple/vertical/
iteration/multi-valued grouping, broader filters/sorts, and mutations stay outside
that initial grouping slice.

## Superseding enablement decision — 2026-10-01

The user requested general timeline enablement and explained that the work board
can only be authenticated on their work computer. The earlier populated-sample
prerequisite is therefore retained for claims of web parity, rather than blocking
all live grouping. Mapped DATE timelines now admit one single-select field with
no vertical grouping using an explicitly disclosed local display policy: complete
API option order, unset last, and all groups expanded. Unsupported group values
remain visible as unavailable; empty options retain loaded counts. Saved custom
order/collapse and web unset placement are not claimed to be restored.

The renderer is connected to the full item-loading, selection, search, lazy-detail,
refresh and read-only lifecycle. Membership uses field/option IDs; complete unique
field definitions and option identities are required. Multiple/vertical/iteration
and multi-valued grouping remain rejected. Model tests cover both owner kinds,
picker/direct startup, group-order navigation, paging/search/refresh identity,
fallback membership, and mutation guards. No work board access or data changes
were needed to implement the general local policy.
