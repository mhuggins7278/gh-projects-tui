# Saved-filter audit — 2026-09-30

Issue #14's bounded grammar is implemented. The authoritative supported/unsupported
matrix, quoting rules, and earlier live evidence remain in
[API contract: Saved Filter Grammar and Evidence](api-contract.md#saved-filter-grammar-and-evidence).
This audit rechecks representative expressions; it does not enable every documented
GitHub qualifier or turn previously gated forms into supported syntax.

## Parser boundary correction

Unquoted parentheses previously passed the general-word and simple-value validators,
despite the documented exclusion of Boolean grouping. They now keep the saved view
blocked. Examples include `(bug)`, `bug (fix)`, `status:Todo)`, and `title:*(fix)*`.
Double-quoted literal punctuation remains accepted, such as `title:"Fix (bug)"`.
The picker explains that Boolean operators and grouping parentheses are unsupported.
Accepted expressions still pass unchanged to `ProjectV2.items(query:)` on every page;
there is no local membership implementation.

Targeted parser tests passed. A two-second fuzz run completed 32,877 executions with
two workers and no failures. Existing adapter fixtures verify byte-for-byte filter
preservation across cursor pages for both user and organization owners.

## Read-only evidence

The existing user-owned sandbox contained 26 active items in the unfiltered baseline.
All membership comparisons used IDs only in memory; this note retains counts only.
No project data was created, edited, or copied into fixtures.

The expanded `TestLiveSavedFilterContract` reads a complete baseline, derives sample
values from existing metadata, checks complete filtered ID sets, rejects duplicate
IDs/cursor failures, and forces two-item pages. Success logs use static probe names
and aggregate counts, without dates, issue references, titles, or IDs.

| Representative expression family | Matching items / cursor pages |
| --- | --- |
| Closed exact date, equal-bound range, relative exact date, relative equal-bound range | 5 / 3 each |
| Negated closed exact date | 21 / 11 |
| Closed comparison, `has:closed`, `-no:closed` | 17 / 9 each |
| `no:closed` | 9 / 5 |
| Quoted/unquoted parent reference, `has:parent-issue`, `-no:parent-issue` | 2 / 1 each |
| Negated quoted/unquoted parent reference, `no:parent-issue` | 24 / 12 each |
| Status comma-separated OR | 26 / 13 |
| Negated Status OR, distinct repeated Status qualifiers (AND) | 0 / 1 each |
| Status OR combined with `is:closed` (AND) | 17 / 9 |
| Created exact date and equal-bound range | 13 / 7 each |
| Created comparison | 26 / 13 |
| Updated exact date, equal-bound range, relative exact date, relative equal-bound range | 5 / 3 each |
| Negated updated exact date | 21 / 11 |
| Updated comparison | 26 / 13 |

The complete expanded live contract passed all 29 expression cases in 100.97 seconds.
The opt-in `live` test package also compiles with the probe disabled.

The `updated:` investigation exposed an incorrect expectation in the initial expanded
probe: its exact/equal-bound queries returned five items while `Issue.updatedAt`
predicted six. A separate read-only comparison tested every populated update-date
group. All four query ID sets matched **`ProjectV2Item.updatedAt`** exactly, with
counts 5/1/10/10. Issue timestamp groups predicted 6/1/10/9. A baseline reread found
zero changed items during that comparison.

This supports project-item update dates for the sampled Projects queries. It does not
establish that repository issue timestamps alone can predict Projects `updated:`
membership, or establish semantics for every host or item kind. The harness now makes
the project-item source explicit for updated comparisons; created comparisons continue
to use the separately observed issue/PR creation timestamps. Closure dates remain
issue/PR closure timestamps. GitHub evaluates all production filters server-side.

## Reproduction and remaining boundaries

Use an existing accessible project with populated closure dates and parent-linked child
issues. Status combination probes require two populated Status values; absent optional
samples are reported instead of manufactured. Relative dates are calculated at request
time and require rerunning around calendar boundaries.

```sh
GH_PROJECTS_TUI_LIVE_FILTERS=1 \
GH_PROJECTS_TUI_LIVE_OWNER=OWNER \
GH_PROJECTS_TUI_LIVE_OWNER_KIND=user \
GH_PROJECTS_TUI_LIVE_PROJECT=PROJECT_NUMBER \
go test -tags live ./internal/github -run '^TestLiveSavedFilterContract$' -count=1 -v
```

The [official Projects filtering documentation](https://docs.github.com/en/issues/planning-and-tracking-with-projects/customizing-views-in-your-project/filtering-projects)
was rechecked on this date. It continues to describe whitespace/repeated qualifier AND,
comma-separated values as within-field OR, and cross-field OR as unsupported.
The API matrix explicitly retains custom DATE/TEXT/MULTI_SELECT fields, unsampled
iteration offsets/next forms, reviewer `@me`/teams, milestones, reopened reasons,
additional multi-value forms, escaped quotes, and single quotes behind the compatibility
gate. Those cases need matching populated read-only samples before support expands.
