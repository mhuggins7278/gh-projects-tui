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

Resume #29 with an existing roadmap using explicitly observed DATE start/target
roles and one single-select grouping field. Compare populated and unset sections,
option order, membership, and supported sorting within each section; record which
collapsed/custom display state is unavailable. Then build the grouped renderer
and row-window/selection regressions for the verified subset. Multiple/vertical/
iteration/multi-valued grouping, broader filters/sorts, and mutations stay outside
that initial grouping slice.
